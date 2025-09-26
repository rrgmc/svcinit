package svcinit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"

	slog2 "github.com/rrgmc/svcinit/v2/slog"
)

func (m *Manager) init() error {
	if len(m.stages) == 0 {
		return ErrNoStage
	}

	if slices.Contains(m.stages, "") {
		return fmt.Errorf("%w: blank stage not allowed", ErrInvalidStage)
	}

	return nil
}

func (m *Manager) runWithStopErrors(ctx context.Context, options ...RunOption) (cause error, stopErr error) {
	if !m.isRunning.CompareAndSwap(false, true) {
		return ErrAlreadyRunning, nil
	}

	if len(m.initErrors) > 0 {
		return buildMultiErrors(m.initErrors), nil
	}

	if m.tasks.stepTaskCount(StepStart) == 0 {
		return ErrNoStartTask, nil
	}

	var roptns runOptions
	for _, option := range options {
		option(&roptns)
	}

	ctx = slog2.LoggerToContext(ctx, m.logger)

	stopErrBuilder := newMultiErrorBuilder()

	// create the context to be used during initialization.
	// this ensures that any task start step returning early don't cancel other start steps.
	m.startupCtx, m.startupCancel = context.WithCancelCause(ctx)
	// create the context to be sent to start steps with cancelContext = true.
	// It may only be cancelled after the full initialization finishes.
	m.taskDoneCtx, m.taskDoneCancel = context.WithCancelCause(ctx)

	defer m.taskDoneCancel(nil) // must cancel all contexts to avoid resource leak
	defer m.startupCancel(nil)  // must cancel all contexts to avoid resource leak

	defer func() {
		m.teardown(ctx, stopErrBuilder)
		stopErr = stopErrBuilder.build()
		var ferr fatalError
		if errors.As(stopErr, &ferr) {
			if cause == nil {
				cause = ferr.err
			} else {
				cause = errors.Join(cause, ferr.err)
			}
		}
		cause = unwrapInternalErrors(cause)

		if cause == nil {
			m.logger.InfoContext(ctx, "execution finished")
		} else {
			m.logger.WarnContext(ctx, "execution finished with cause", slog2.ErrorKey, cause)
		}

		if stopErr != nil {
			m.logger.WarnContext(ctx, "execution finished with stop error", slog2.ErrorKey, stopErr)
		}
	}()

	// run setup and start steps.
	setupErr := m.start(ctx)
	if setupErr != nil {
		m.logger.ErrorContext(ctx, "setup error",
			slog2.ErrorKey, setupErr)
	}

	m.logger.InfoContext(ctx, "waiting for first task to return")
	<-m.startupCtx.Done()
	// get the error returned by the first exiting task. It will be the cause of exit.
	if setupErr == nil {
		cause = context.Cause(m.startupCtx)
	} else {
		cause = setupErr
	}
	m.logger.WarnContext(ctx, "first task returned", slog.String("cause", cause.Error()))
	m.logger.Log(ctx, slog2.LevelTrace, "cancelling start task context")
	// cancel the context of all tasks with cancelContext = true
	m.taskDoneCancel(cause)

	if roptns.shutdownCtx == nil {
		roptns.shutdownCtx = context.WithoutCancel(ctx)
	}

	// run stop steps.
	var shutdownErr error
	shutdownErr = m.shutdown(contextWithCause(roptns.shutdownCtx, cause), stopErrBuilder)
	if shutdownErr != nil {
		m.logger.ErrorContext(ctx, "shutdown error", slog2.ErrorKey, shutdownErr)
		cause = shutdownErr
		return
	}

	if errors.Is(cause, ErrExit) {
		cause = nil
	}

	return
}

// start runs the setup and start steps.
func (m *Manager) start(ctx context.Context) error {
	setupErr := newMultiErrorBuilder()

	for stage := range stagesIter(m.stages, false) {
		loggerStage := m.logger.With("stage", stage)
		loggerStage.InfoContext(ctx, "starting stage")

		// run setup tasks
		m.runStage(ctx, m.taskDoneCtx, loggerStage, stage, StepSetup, nil, false,
			func(serr error) {
				if serr != nil {
					ierr := newInitializationError(serr)
					setupErr.add(ierr)
					m.startupCancel(ierr)
				}
			})

		// if setupErr.hasErrors() {
		// 	return setupErr.build()
		// }

		// run start tasks
		m.runStage(ctx, m.taskDoneCtx, loggerStage, stage, StepStart, &m.tasksRunning, false,
			func(serr error) {
				if serr != nil {
					m.startupCancel(serr)
				} else {
					m.startupCancel(ErrExit)
				}
			})

		loggerStage.InfoContext(ctx, "starting stage: finished")
	}

	return setupErr.build()
}

// shutdown runs the stop step.
func (m *Manager) shutdown(ctx context.Context, eb *multiErrorBuilder) (err error) {
	var shutdownAttr []slog.Attr
	if m.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.shutdownTimeout)
		defer cancel()
		shutdownAttr = append(shutdownAttr,
			slog.Duration("timeout", m.shutdownTimeout))
	}
	m.logger.LogAttrs(ctx, slog.LevelInfo, "shutting down", shutdownAttr...)

	// run stop tasks in reverse stage order
	for stage := range stagesIter(m.stages, true) {
		loggerStage := m.logger.With("stage", stage)
		loggerStage.InfoContext(ctx, "stopping stage")

		// run stop tasks
		m.runStage(ctx, ctx, loggerStage, stage, StepStop, nil, m.enforceShutdownTimeout,
			func(serr error) {
				if serr != nil {
					loggerStage.ErrorContext(ctx, "step error",
						slog2.ErrorKey, serr)
				}
				eb.add(serr)
			})

		loggerStage.InfoContext(ctx, "stopping stage: finished")
	}

	// wait for all goroutines to finish
	m.logger.LogAttrs(ctx, slog.LevelInfo, "waiting for tasks to finish", shutdownAttr...)
	if m.enforceShutdownTimeout {
		_ = waitGroupWaitWithContext(ctx, &m.tasksRunning)
	} else {
		m.tasksRunning.Wait()
	}
	m.logger.LogAttrs(ctx, slog.LevelDebug, "finished waiting for tasks to finish", shutdownAttr...)

	if ctx.Err() != nil {
		ctxCause := context.Cause(ctx)
		if errors.Is(ctxCause, context.DeadlineExceeded) {
			ctxCause = ErrShutdownTimeout
		}
		eb.add(ctxCause)
		m.logger.ErrorContext(ctx, "shutdown context error", slog2.ErrorKey, ctxCause)
	}

	return nil
}

// start runs the setup and start steps.
func (m *Manager) teardown(ctx context.Context, eb *multiErrorBuilder) {
	for stage := range stagesIter(m.stages, true) {
		loggerStage := m.logger.With("stage", stage)
		loggerStage.InfoContext(ctx, "running stage")

		// run teardown tasks
		m.runStage(ctx, ctx, loggerStage, stage, StepTeardown, nil, false,
			func(serr error) {
				eb.add(serr)
			})
	}
}

func (m *Manager) AddInitError(err error) {
	if m.isRunning.Load() {
		return
	}
	m.initErrors = append(m.initErrors, err)
}

// runStage runs all tasks for one step / stage.
func (m *Manager) runStage(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step,
	waitWG *sync.WaitGroup, enforceWaitTimeout bool, onError func(serr error)) {
	// run start tasks
	loggerStep := logger.With("step", step.String())
	var loggerStepOnce sync.Once
	loggerStepFn := func() {
		loggerStep.InfoContext(ctx, "running step")
	}

	m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepBefore)

	isWait := false
	if waitWG == nil {
		isWait = true
		waitWG = &sync.WaitGroup{}
	}

	taskCount := m.runStageStep(ctx, cancelCtx, stage, step, waitWG, func() {
		loggerStepOnce.Do(loggerStepFn)
	}, onError)

	m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepAfter)

	if taskCount > 0 {
		logger.Log(ctx, slog.LevelDebug, "waiting for step to finish")
	}
	if isWait {
		if enforceWaitTimeout {
			_ = waitGroupWaitWithContext(ctx, waitWG)
		} else {
			waitWG.Wait()
		}
	}
	if taskCount > 0 {
		loggerStep.InfoContext(ctx, "running step: finished")
	}
}

func (m *Manager) runStageStep(ctx, cancelCtx context.Context, stage string, step Step, wg *sync.WaitGroup,
	onTask func(), onError func(err error)) int {
	loggerStage := m.logger.With(
		"stage", stage,
		"step", step.String())

	var startWg sync.WaitGroup
	var taskCount atomic.Int64

	for tw := range m.tasks.stageTasks(stage) {
		taskDesc := TaskDescription(tw.task)

		if startStep, err := tw.checkStartStep(step); err != nil {
			onError(err)
			continue
		} else if !startStep {
			// taskCount.Add(1)
			// onTask()
			// loggerStage.Log(ctx, slog2.LevelTrace, "task don't have step, skipping",
			// 	"task", taskDesc)
			continue
		}

		// if !tw.canRunStep(step) {
		// 	err := tw.stepDone(step)
		// 	if err != nil {
		// 		onError(err)
		// 	}
		// 	// taskCount.Add(1)
		// 	// onTask()
		// 	// loggerStage.Log(ctx, slog2.LevelTrace, "task don't have step, skipping",
		// 	// 	"task", taskDesc)
		// 	continue
		// }

		loggerTask := loggerStage.With("task", taskDesc)

		var logAttrs []any

		taskCount.Add(1)
		onTask()

		wg.Add(1)
		startWg.Add(1)
		go func() {
			defer wg.Done()
			startWg.Done()
			taskCtx := cancelCtx
			var taskCancelOnStop context.CancelFunc
			switch step {
			case StepStart:
				logAttrs = append(logAttrs,
					slog.Bool("cancelContext", tw.options.cancelContext),
				)
				if !tw.options.cancelContext {
					taskCtx = ctx // don't cancel context automatically using the global task done context
				}
				if tw.options.startStepManager {
					logAttrs = append(logAttrs, slog.Bool("ssm", true))
					// create cancellable context for the start step.
					tw.mu.Lock()
					taskCtx, tw.startCancel = context.WithCancelCause(taskCtx)
					defer tw.startCancel(context.Canceled) // ensure context is always cancelled
					// create context to be cancelled when the start task ends.
					tw.finishCtx, taskCancelOnStop = context.WithCancel(context.WithoutCancel(taskCtx))
					defer taskCancelOnStop() // ensure context is always cancelled
					tw.mu.Unlock()
				}
			case StepStop:
				if tw.options.startStepManager {
					logAttrs = append(logAttrs, slog.Bool("ssm", true))
					startStepMan := &startStepManager{
						logger: loggerTask,
					}

					tw.mu.Lock()
					if tw.options.startStepManager {
						startStepMan.cancel = tw.startCancel
						startStepMan.finished = tw.finishCtx
					}
					tw.mu.Unlock()

					if startStepMan.cancel != nil || startStepMan.finished != nil {
						taskCtx = contextWithStartStepManager(taskCtx, startStepMan)
					}
				}
			default:
			}
			if tw.checkRunStep(step) {
				if loggerTask.Enabled(ctx, slog.LevelInfo) {
					loggerTask.InfoContext(ctx, "running task", logAttrs...)
				}
				err := tw.run(taskCtx, loggerTask, stage, step, m.taskCallbacks)
				if loggerTask.Enabled(ctx, slog.LevelInfo) {
					if err != nil {
						loggerTask.With(logAttrs...).WarnContext(ctx, "task finished with error",
							slog2.ErrorKey, err)
					} else {
						loggerTask.With(logAttrs...).InfoContext(ctx, "task finished")
					}
				}
				onError(err)
			}
		}()
	}

	if taskCount.Load() > 0 {
		loggerStage.Log(ctx, slog2.LevelTrace, "waiting for task goroutines to start")
	}
	startWg.Wait() // even if taskCount is 0, some startup might have been done, must still wait.
	if taskCount.Load() > 0 {
		loggerStage.Log(ctx, slog2.LevelTrace, "waiting for task goroutines to start: finished")
	}

	return int(taskCount.Load())
}

func (m *Manager) runManagerCallbacks(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
	ctx = context.WithoutCancel(ctx)

	if m.managerCallbacks != nil {
		for _, scallback := range m.managerCallbacks {
			scallback.Callback(ctx, stage, step, callbackStep)
		}
	}
}

type runOptions struct {
	shutdownCtx context.Context
}
