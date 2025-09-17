package svcinit

import (
	"context"
	"sync"
)

func (s *Manager) start() error {
	if s.managerCallback != nil {
		for _, scallback := range s.managerCallback {
			if serr := scallback.Callback(s.ctx, TaskStageStart, StepBefore, nil); serr != nil {
				s.cancel(serr)
				return serr
			}
		}
	}

	var runWg sync.WaitGroup

	// start all tasks in separate goroutines.
	for _, task := range s.tasks {
		s.wg.Add(1)
		runWg.Add(1)
		go func() {
			defer s.wg.Done()
			runWg.Done()
			err := task.run(task.ctx, TaskStageStart, s.taskCallback...)
			if err != nil {
				s.cancel(err)
			} else {
				s.cancel(ErrExit)
			}
		}()
	}
	runWg.Wait()
	if s.managerCallback != nil {
		for _, scallback := range s.managerCallback {
			if serr := scallback.Callback(s.ctx, TaskStageStart, StepAfter, nil); serr != nil {
				s.cancel(serr)
			}
		}
	}

	return nil
}

func (s *Manager) shutdown(cause error) (err error, cleanupErr error) {
	if s.managerCallback != nil {
		for _, scallback := range s.managerCallback {
			if serr := scallback.Callback(s.shutdownCtx, TaskStageStop, StepBefore, cause); serr != nil {
				return serr, nil
			}
		}
	}

	var (
		wg sync.WaitGroup
	)

	errorBuilder := newMultiErrorBuilder()

	ctx := s.shutdownCtx
	if s.shutdownTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(s.shutdownCtx, s.shutdownTimeout)
		defer cancel()
	}

	if len(s.autoCleanup) > 0 {
		// cleanups where order don't matter are done in parallel
		for _, task := range s.autoCleanup {
			wg.Go(func() {
				err := task.run(ctx, TaskStageStop, s.taskCallback...)
				errorBuilder.add(err)
			})
		}
	}

	// execute ordered cleanups synchronously
	if len(s.cleanup) > 0 {
		wg.Go(func() {
			for _, task := range s.cleanup {
				err := task.run(ctx, TaskStageStop, s.taskCallback...)
				errorBuilder.add(err)
			}
		})
	}

	// wait for auto cleanups, if any
	if s.enforceShutdownTimeout {
		if !waitGroupWaitWithContext(ctx, &wg) {
			errorBuilder.add(ErrShutdownTimeout)
		}
	} else {
		wg.Wait()
		if ctx.Err() != nil {
			errorBuilder.add(context.Cause(ctx))
		}
	}

	if s.managerCallback != nil {
		for _, scallback := range s.managerCallback {
			if serr := scallback.Callback(s.shutdownCtx, TaskStageStop, StepAfter, cause); serr != nil {
				errorBuilder.add(serr)
			}
		}
	}

	return nil, errorBuilder.build()
}

func (s *Manager) checkPending() error {
	for _, task := range s.pendingStarts {
		if !task.isResolved() {
			return ErrPending
		}
	}
	for _, task := range s.pendingStops {
		if !task.isResolved() {
			return ErrPending
		}
	}
	return nil
}

func (s *Manager) addPendingStart(p pendingItem) {
	s.pendingStarts = append(s.pendingStarts, p)
}

func (s *Manager) addPendingStop(p pendingItem) {
	s.pendingStops = append(s.pendingStops, p)
}

func (s *Manager) addPendingStopTask(task Task, options ...TaskOption) StopFuture {
	st := newPendingStopFuture(task, options...)
	s.pendingStops = append(s.pendingStops, st)
	return st
}

func runTask(ctx context.Context, task Task, stage TaskStage, callbacks ...TaskCallback) error {
	if tcb, ok := task.(taskRunCallback); ok {
		return tcb.runWithCallbacks(ctx, stage, callbacks...)
	}
	for _, callback := range callbacks {
		if callback != nil {
			callback.BeforeRun(ctx, UnwrapTask(task), stage)
		}
	}
	err := task.Run(ctx)
	for _, callback := range callbacks {
		if callback != nil {
			callback.AfterRun(ctx, UnwrapTask(task), stage, err)
		}
	}
	return err
}

type taskWrapper struct {
	ctx     context.Context
	task    Task
	options taskOptions
}

func (w *taskWrapper) run(ctx context.Context, stage TaskStage, callbacks ...TaskCallback) error {
	if w.ctx != nil {
		ctx = w.ctx
	}
	return runTask(ctx, w.task, stage, joinTaskCallbacks(callbacks, []TaskCallback{w.options.callback})...)
}

func newTaskWrapper(ctx context.Context, task Task, options ...TaskOption) taskWrapper {
	ret := taskWrapper{
		ctx:  ctx,
		task: task,
	}
	for _, option := range options {
		option(&ret.options)
	}
	return ret
}

func newStopTaskWrapper(task Task, options ...TaskOption) taskWrapper {
	return newTaskWrapper(nil, task, options...)
}
