package svcinit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

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

	stopErrBuilder := newMultiErrorBuilder()

	defer func() {
		m.teardown(ctx, stopErrBuilder)
		stopErr = stopErrBuilder.build()
		m.logger.InfoContext(ctx, "execution finished", "cause", cause)
	}()

	// create the context to be used during initialization.
	// this ensures that any task start step returning early don't cancel other start steps.
	m.startupCtx, m.startupCancel = context.WithCancelCause(ctx)
	// create the context to be sent to start steps with cancelContext = true.
	// It may only be cancelled after the full initialization finishes.
	m.taskDoneCtx, m.taskDoneCancel = context.WithCancelCause(ctx)

	// run setup and start steps.
	err := m.start(ctx)
	if err != nil {
		cause = err
		return
	}

	m.logger.InfoContext(ctx, "waiting for one start task to return")
	<-m.startupCtx.Done()
	// get the error returned by the first exiting task. It will be the cause of exit.
	cause = context.Cause(m.startupCtx)
	m.logger.InfoContext(ctx, "first start task returned", "cause", cause)
	m.logger.Log(ctx, slog2.LevelTrace, "cancelling start task context")
	// cancel the context of all tasks with cancelContext = true
	m.taskDoneCancel(cause)

	if roptns.shutdownCtx == nil {
		roptns.shutdownCtx = context.WithoutCancel(ctx)
	}

	// run pre-stop and stop steps.
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

	// m.logger.InfoContext(ctx, "execution finished", "cause", cause)

	return
}

// start runs the setup and start steps.
func (m *Manager) start(ctx context.Context) error {
	// run setup tasks
	err := m.runStep(ctx, m.taskDoneCtx, StepSetup,
		func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error {
			var setupWG sync.WaitGroup

			m.runStage(ctx, cancelCtx, stage, step, &setupWG, func(serr error) {
				if serr != nil {
					m.startupCancel(newInitializationError(serr))
				}
			})

			logger.Log(ctx, slog.LevelDebug, "waiting for step stage tasks to finish")
			setupWG.Wait()
			return nil
		})
	if err != nil {
		return err
	}

	if m.startupCtx.Err() != nil {
		return nil
	}

	// run start tasks
	err = m.runStep(ctx, m.taskDoneCtx, StepStart,
		func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error {
			m.runStage(ctx, cancelCtx, stage, step, &m.tasksRunning, func(serr error) {
				if serr != nil {
					m.startupCancel(serr)
				} else {
					m.startupCancel(ErrExit)
				}
			})
			return nil
		})
	if err != nil {
		return err
	}

	return nil
}

// shutdown runs the pre-stop and stop steps.
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

	// run pre-stop tasks in reverse stage order
	err = m.runStep(ctx, ctx, StepPreStop,
		func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error {
			var preStopWG sync.WaitGroup

			m.runStage(ctx, cancelCtx, stage, step, &m.tasksRunning, func(serr error) {
				logger.ErrorContext(ctx, "task error", slog2.ErrorKey, serr)
				eb.add(serr)
			})

			logger.Log(ctx, slog2.LevelTrace, "waiting for step stage tasks to finish")
			if m.enforceShutdownTimeout {
				_ = waitGroupWaitWithContext(ctx, &preStopWG)
			} else {
				preStopWG.Wait()
			}

			return nil
		})
	if err != nil {
		return err
	}

	// run stop tasks in reverse stage order
	err = m.runStep(ctx, ctx, StepStop,
		func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error {
			var stopWG sync.WaitGroup

			m.runStage(ctx, cancelCtx, stage, step, &m.tasksRunning, func(serr error) {
				logger.ErrorContext(ctx, "task error", slog2.ErrorKey, serr)
				eb.add(serr)
			})

			logger.Log(ctx, slog2.LevelTrace, "waiting for step stage tasks to finish")
			if m.enforceShutdownTimeout {
				_ = waitGroupWaitWithContext(ctx, &stopWG)
			} else {
				stopWG.Wait()
			}
			return nil
		})
	if err != nil {
		return err
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
	// run teardown tasks
	err := m.runStep(ctx, ctx, StepTeardown,
		func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error {
			var teardownWG sync.WaitGroup

			m.runStage(ctx, cancelCtx, stage, step, &teardownWG, func(serr error) {
				eb.add(serr)
			})

			logger.Log(ctx, slog.LevelDebug, "waiting for step stage tasks to finish")
			teardownWG.Wait()
			return nil
		})
	if err != nil {
		eb.add(err)
	}
}

func (m *Manager) AddInitError(err error) {
	if m.isRunning.Load() {
		return
	}
	m.initErrors = append(m.initErrors, err)
}

func (m *Manager) runStep(ctx, cancelCtx context.Context, step Step,
	onStage func(ctx, cancelCtx context.Context, logger *slog.Logger, stage string, step Step) error) error {
	// run start tasks
	loggerStep := m.logger.With("step", step.String())
	loggerStep.InfoContext(ctx, "running step tasks")

	m.runManagerCallbacks(cancelCtx, "", step, CallbackStepBefore)

	for stage := range stepStagesIter(step, m.stages) {
		loggerStage := loggerStep.With("stage", stage)
		loggerStage.InfoContext(ctx, "running step stage tasks")

		m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepBefore)

		err := onStage(ctx, cancelCtx, loggerStep, stage, step)
		if err != nil {
			return err
		}

		m.runManagerCallbacks(cancelCtx, stage, step, CallbackStepAfter)

		loggerStage.InfoContext(ctx, "finished running step stage tasks")
	}

	m.runManagerCallbacks(cancelCtx, "", step, CallbackStepAfter)

	loggerStep.InfoContext(ctx, "finished running step tasks")

	return nil
}

func (m *Manager) runStage(ctx, cancelCtx context.Context, stage string, step Step, wg *sync.WaitGroup, onError func(err error)) {
	loggerStage := m.logger.With(
		"stage", stage,
		"step", step.String())

	var startWg sync.WaitGroup

	for tw := range m.tasks.stageTasks(stage) {
		taskDesc := taskDescription(tw.task)

		if !tw.hasStep(step) {
			loggerStage.Log(ctx, slog2.LevelTrace, "task don't have step, skipping",
				"task", taskDesc)
			continue
		}

		loggerTask := loggerStage.With("task", taskDesc)

		var logAttrs []any

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
			if taskHasStep(step, tw.task) {
				if loggerTask.Enabled(ctx, slog.LevelInfo) {
					loggerTask.InfoContext(ctx, "running task step", logAttrs...)
				}
				err := tw.run(taskCtx, stage, step, m.taskCallbacks)
				if loggerTask.Enabled(ctx, slog.LevelInfo) {
					if err != nil {
						loggerStage.With(logAttrs...).InfoContext(ctx, "task step finished with error",
							slog2.ErrorKey, err)
					} else {
						loggerStage.With(logAttrs...).InfoContext(ctx, "task step finished")
					}
				}
				onError(err)
			}
		}()
	}

	loggerStage.Log(ctx, slog2.LevelTrace, "waiting for task step goroutines to start")
	startWg.Wait()
	loggerStage.Log(ctx, slog2.LevelTrace, "all task step goroutines started")
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
