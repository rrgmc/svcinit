package svcinit

import (
	"context"
	"sync"
)

func (s *Manager) start() error {
	if serr := s.runCallbacks(s.shutdownCtx, StageStart, StepBefore, nil); serr != nil {
		s.cancel(serr)
		return serr
	}

	var runWg sync.WaitGroup

	// start all tasks in separate goroutines.
	for _, task := range s.tasks {
		s.wg.Add(1)
		runWg.Add(1)
		go func() {
			defer s.wg.Done()
			runWg.Done()
			err := task.run(task.ctx, StageStart, s.globalTaskCallbacks)
			if err != nil {
				s.cancel(err)
			} else {
				s.cancel(ErrExit)
			}
		}()
	}
	runWg.Wait()

	if serr := s.runCallbacks(s.shutdownCtx, StageStart, StepAfter, nil); serr != nil {
		s.cancel(serr)
	}

	return nil
}

func (s *Manager) shutdown(cause error) (err error, cleanupErr error) {
	if serr := s.runCallbacks(s.shutdownCtx, StageStop, StepBefore, cause); serr != nil {
		return serr, nil
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

	if len(s.preStopTasks) > 0 {
		if serr := s.runCallbacks(s.shutdownCtx, StagePreStop, StepBefore, cause); serr != nil {
			return serr, nil
		}

		var wgPreStop sync.WaitGroup
		// stop tasks where order don't matter are done in parallel
		for _, task := range s.preStopTasks {
			wgPreStop.Go(func() {
				err := task.run(ctx, StagePreStop, s.globalTaskCallbacks)
				errorBuilder.add(err)
			})
		}

		if s.enforceShutdownTimeout {
			_ = waitGroupWaitWithContext(ctx, &wgPreStop)
		} else {
			wgPreStop.Wait()
		}

		if serr := s.runCallbacks(s.shutdownCtx, StagePreStop, StepAfter, cause); serr != nil {
			return serr, nil
		}
	}

	if len(s.stopTasks) > 0 {
		// stop tasks where order don't matter are done in parallel
		for _, task := range s.stopTasks {
			wg.Go(func() {
				err := task.run(ctx, StageStop, s.globalTaskCallbacks)
				errorBuilder.add(err)
			})
		}
	}

	// execute ordered stop tasks synchronously
	if len(s.stopTasksOrdered) > 0 {
		wg.Go(func() {
			for _, task := range s.stopTasksOrdered {
				err := task.run(ctx, StageStop, s.globalTaskCallbacks)
				errorBuilder.add(err)
			}
		})
	}

	// wait for auto stop tasks, if any
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

	if serr := s.runCallbacks(s.shutdownCtx, StageStop, StepAfter, cause); serr != nil {
		errorBuilder.add(serr)
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

func (s *Manager) runCallbacks(ctx context.Context, stage Stage, step Step, cause error) error {
	eb := newMultiErrorBuilder()
	if s.managerCallbacks != nil {
		for _, scallback := range s.managerCallbacks {
			if serr := scallback.Callback(ctx, stage, step, cause); serr != nil {
				eb.add(serr)
			}
		}
	}
	return eb.build()
}

func (s *Manager) newTaskWrapper(ctx context.Context, task Task, options ...TaskOption) taskWrapper {
	if task == nil {
		s.isNilTask.Store(true)
	}
	return newTaskWrapper(ctx, task, options...)
}

func (s *Manager) newStopTaskWrapper(task Task, options ...TaskOption) taskWrapper {
	return s.newTaskWrapper(nil, task, options...)
}

func runTask(ctx context.Context, task Task, stage Stage, callbacks []TaskCallback) error {
	if tcb, ok := task.(taskRunCallback); ok {
		return tcb.runWithCallbacks(ctx, stage, callbacks...)
	}
	for _, callback := range callbacks {
		if callback != nil {
			callback.Callback(ctx, UnwrapTask(task), stage, StepBefore, nil)
		}
	}
	err := task.Run(ctx)
	for _, callback := range callbacks {
		if callback != nil {
			callback.Callback(ctx, UnwrapTask(task), stage, StepAfter, err)
		}
	}
	return err
}

type taskWrapper struct {
	ctx     context.Context
	task    Task
	options taskOptions
}

func (w *taskWrapper) run(ctx context.Context, stage Stage, callbacks []TaskCallback) error {
	if w.ctx != nil {
		ctx = w.ctx
	}
	// run global callbacks first.
	return runTask(ctx, w.task, stage, joinTaskCallbacks(callbacks, w.options.callbacks))
}

func newTaskWrapper(ctx context.Context, task Task, options ...TaskOption) taskWrapper {
	ret := taskWrapper{
		ctx:  ctx,
		task: checkNilTask(task),
	}
	for _, option := range options {
		option(&ret.options)
	}
	return ret
}
