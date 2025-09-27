package svcinit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
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

	ctx = loggerToContext(ctx, m.logger)

	// create the context to be used during initialization.
	// this ensures that any task start step returning early don't cancel other start steps.
	m.startupCtx, m.startupCancel = context.WithCancelCause(ctx)
	// create the context to be sent to start steps with cancelContext = true.
	// It may only be cancelled after the full initialization finishes.
	m.taskDoneCtx, m.taskDoneCancel = context.WithCancelCause(ctx)

	defer m.taskDoneCancel(nil) // must cancel all contexts to avoid resource leak
	defer m.startupCancel(nil)  // must cancel all contexts to avoid resource leak

	// run setup and start steps.
	setupErr := m.start(ctx)
	if setupErr != nil {
		m.logger.ErrorContext(ctx, "setup error",
			slog2.ErrorKey, setupErr)
	}

	if setupErr == nil {
		m.logger.InfoContext(ctx, "waiting for first task to return")
	}
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
	} else {
		roptns.shutdownCtx = loggerToContext(roptns.shutdownCtx, m.logger)
	}

	// run stop steps.
	stopErr = m.shutdown(contextWithCause(roptns.shutdownCtx, cause))
	if stopErr != nil {
		m.logger.ErrorContext(ctx, "shutdown error", slog2.ErrorKey, stopErr)
	}

	if errors.Is(cause, ErrExit) {
		cause = nil
	}

	// build errors to return
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

	return
}

// start runs the setup and start steps.
func (m *Manager) start(ctx context.Context) error {
	for stage := range stagesIter(m.stages, false) {
		loggerStage := m.logger.With("stage", stage)

		// run setup tasks
		setupErr := newMultiErrorBuilder()

		m.runStage(ctx, m.taskDoneCtx, loggerStage, stage, StepSetup, nil, false,
			func(serr error) {
				if serr != nil {
					ierr := newInitializationError(serr)
					setupErr.add(ierr)
					m.startupCancel(ierr)
				}
			})

		if setupErr.hasErrors() {
			return setupErr.build()
		}

		// run start tasks
		m.runStage(ctx, m.taskDoneCtx, loggerStage, stage, StepStart, &m.tasksRunning, false,
			func(serr error) {
				if serr != nil {
					m.startupCancel(serr)
				} else {
					m.startupCancel(ErrExit)
				}
			})
	}

	return nil
}

// shutdown runs the stop step.
func (m *Manager) shutdown(ctx context.Context) (err error) {
	startTime := time.Now()

	shutdownCtx := ctx
	var shutdownAttr []slog.Attr
	if m.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		shutdownCtx, cancel = context.WithTimeout(ctx, m.shutdownTimeout)
		defer cancel()
		shutdownAttr = append(shutdownAttr,
			slog.Duration("timeout", m.shutdownTimeout))
	}
	m.logger.LogAttrs(shutdownCtx, slog.LevelInfo, "shutting down", shutdownAttr...)

	eb := newMultiErrorBuilder()

	// run stop tasks in reverse stage order
	for stage := range stagesIter(m.stages, true) {
		loggerStage := m.logger.With("stage", stage)

		// run stop tasks
		m.runStage(shutdownCtx, shutdownCtx, loggerStage, stage, StepStop, nil, m.enforceShutdownTimeout,
			func(serr error) {
				eb.add(serr)
			})
	}

	// wait for all goroutines to finish
	m.logger.LogAttrs(shutdownCtx, slog.LevelInfo, "waiting for tasks to shutdown", shutdownAttr...)
	if m.enforceShutdownTimeout {
		_ = waitGroupWaitWithContext(shutdownCtx, &m.tasksRunning)
	} else {
		m.tasksRunning.Wait()
	}
	m.logger.
		With("duration", time.Since(startTime).String()).
		LogAttrs(shutdownCtx, slog.LevelDebug, "(finished) waiting for tasks to shutdown", shutdownAttr...)

	isTeardownTimeout := m.teardownTimeout > 0

	if isTeardownTimeout && shutdownCtx.Err() != nil {
		ctxCause := context.Cause(shutdownCtx)
		if errors.Is(ctxCause, context.DeadlineExceeded) {
			ctxCause = ErrShutdownTimeout
		}
		eb.add(ctxCause)
		m.logger.ErrorContext(shutdownCtx, "shutdown context error", slog2.ErrorKey, ctxCause)
	}

	teardownCtx := shutdownCtx

	if isTeardownTimeout {
		var cancel context.CancelFunc
		teardownCtx, cancel = context.WithTimeout(ctx, m.teardownTimeout)
		defer cancel()
		shutdownAttr = append(shutdownAttr,
			slog.Duration("teardown_timeout", m.teardownTimeout))
	}

	// run teardown tasks in reverse stage order
	for stage := range stagesIter(m.stages, true) {
		loggerStage := m.logger.With("stage", stage)

		// run teardown tasks
		m.runStage(teardownCtx, teardownCtx, loggerStage, stage, StepTeardown, nil, false,
			func(serr error) {
				eb.add(serr)
			})
	}

	if teardownCtx.Err() != nil {
		ctxCause := context.Cause(teardownCtx)
		if errors.Is(ctxCause, context.DeadlineExceeded) {
			ctxCause = ErrShutdownTimeout
		}
		eb.add(ctxCause)
		if !isTeardownTimeout {
			m.logger.ErrorContext(teardownCtx, "shutdown context error", slog2.ErrorKey, ctxCause)
		} else {
			m.logger.ErrorContext(teardownCtx, "teardown context error", slog2.ErrorKey, ctxCause)
		}
	}

	m.logger.
		With("duration", time.Since(startTime).String()).
		LogAttrs(ctx, slog.LevelInfo, "(finished) shutting down", shutdownAttr...)

	return eb.build()
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

	var cbStepOnce sync.Once
	var isStepCallback bool
	cbStepFn := func() {
		isStepCallback = true
		m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepBefore)
	}

	isWait := false
	if waitWG == nil {
		isWait = true
		waitWG = &sync.WaitGroup{}
	}

	taskCount := m.runStageStep(ctx, cancelCtx, stage, step, waitWG, !isWait, func(logOnly bool) {
		loggerStepOnce.Do(loggerStepFn)
		if !logOnly {
			cbStepOnce.Do(cbStepFn)
		}
	}, onError)

	if isWait {
		if taskCount > 0 {
			loggerStep.Log(ctx, slog2.LevelTrace, "running step (waiting)")
		}
		if enforceWaitTimeout {
			_ = waitGroupWaitWithContext(ctx, waitWG)
		} else {
			waitWG.Wait()
		}
		if taskCount > 0 {
			loggerStep.InfoContext(ctx, "(finished) running step")
		}
	}

	if isStepCallback {
		m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepAfter)
	}
}

func (m *Manager) runStageStep(ctx, cancelCtx context.Context, stage string, step Step, wg *sync.WaitGroup,
	waitStart bool, onTask func(logOnly bool), onError func(err error)) int {
	loggerStage := m.logger.With(
		"stage", stage,
		"step", step.String())

	var startWg sync.WaitGroup
	var taskCount atomic.Int64

	doInitLog := func(logOnly bool, f func()) {
		if !logOnly {
			taskCount.Add(1)
		}
		onTask(logOnly)
		if f != nil {
			f()
		}
	}

	for tw := range m.tasks.stageTasks(stage) {
		taskDesc := GetTaskDescription(tw.task)
		loggerTask := loggerStage.With("task", taskDesc)

		if startStep, err := tw.checkStartStep(step); err != nil {
			doInitLog(true, func() {
				loggerTask.ErrorContext(ctx, "error checking task start step",
					slog2.ErrorKey, err)
			})
			onError(fatalError{err})
			continue
		} else if !startStep {
			// doInitLog(func() {
			// 	loggerStage.Log(ctx, slog2.LevelTrace, "can't start step, skipping")
			// })
			continue
		}

		var logAttrs []any

		doInitLog(false, nil)

		wg.Add(1)
		if waitStart {
			startWg.Add(1)
		}
		go func() {
			defer wg.Done()
			if waitStart {
				startWg.Done()
			}
			taskCtx := cancelCtx
			var taskCancelOnStop context.CancelCauseFunc
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
					tw.finishCtx, taskCancelOnStop = context.WithCancelCause(context.WithoutCancel(taskCtx))
					defer taskCancelOnStop(startStepManagerNilError) // ensure context is always cancelled
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
					loggerTask.InfoContext(ctx, "running task step", logAttrs...)
				}
				err := tw.run(taskCtx, loggerTask, stage, step, m.taskCallbacks)
				if taskCancelOnStop != nil {
					if err != nil {
						taskCancelOnStop(err)
					} else {
						taskCancelOnStop(startStepManagerNilError)
					}
				}
				if loggerTask.Enabled(ctx, slog.LevelInfo) {
					if err != nil {
						level := slog.LevelDebug
						if step != StepStart && step != StepStop {
							level = slog.LevelWarn
						}
						loggerTask.With(logAttrs...).Log(ctx, level, "(finished with error) running task step",
							slog2.ErrorKey, err)
					} else {
						loggerTask.With(logAttrs...).DebugContext(ctx, "(finished) running task step")
					}
				}
				onError(err)
			}
		}()
	}

	if waitStart {
		if taskCount.Load() > 0 {
			loggerStage.Log(ctx, slog2.LevelTrace, "waiting for task goroutines to start")
		}
		startWg.Wait() // even if taskCount is 0, some startup might have been done, must still wait.
		if taskCount.Load() > 0 {
			loggerStage.Log(ctx, slog2.LevelTrace, "(finished) waiting for task goroutines to start")
		}
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
