package svcinit

import (
	"context"
	"sync"
)

func (s *SvcInit) start() {
	var runWg sync.WaitGroup

	// start all tasks in separate goroutines.
	for _, task := range s.tasks {
		s.wg.Add(1)
		runWg.Add(1)
		go func() {
			defer s.wg.Done()
			runWg.Done()
			err := task.run(task.ctx, s.startTaskCallback)
			if err != nil {
				s.cancel(err)
			} else {
				s.cancel(ErrExit)
			}
		}()
	}
	runWg.Wait()
	if s.startedCallback != nil {
		if serr := s.startedCallback(s.ctx); serr != nil {
			s.cancel(serr)
		}
	}
}

func (s *SvcInit) shutdown() error {
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
		wg.Add(len(s.autoCleanup))
		for _, task := range s.autoCleanup {
			go func() {
				defer wg.Done()
				err := task.run(ctx, s.stopTaskCallback)
				errorBuilder.add(err)
			}()
		}
	}

	// execute ordered cleanups synchronously
	if len(s.cleanup) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, task := range s.cleanup {
				err := task.run(ctx, s.stopTaskCallback)
				errorBuilder.add(err)
			}
		}()
	}

	// wait for auto cleanups, if any
	if s.enforceShutdownTimeout {
		if !waitGroupWaitWithContext(ctx, &wg) {
			errorBuilder.add(ErrShutdownTimeout)
		}
	} else {
		wg.Wait()
	}

	if s.stoppedCallback != nil {
		if serr := s.stoppedCallback(s.shutdownCtx); serr != nil {
			errorBuilder.add(serr)
		}
	}

	return errorBuilder.build()
}

func (s *SvcInit) checkPending() error {
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

func (s *SvcInit) addPendingStart(p pendingItem) {
	s.pendingStarts = append(s.pendingStarts, p)
}

func (s *SvcInit) addPendingStop(p pendingItem) {
	s.pendingStops = append(s.pendingStops, p)
}

func (s *SvcInit) addPendingStopTask(task Task, options ...TaskOption) StopTask {
	st := newPendingStopTask(task, options...)
	s.pendingStops = append(s.pendingStops, st)
	return st
}

func runTask(ctx context.Context, task Task, callbacks ...TaskCallback) error {
	if tcb, ok := task.(taskRunCallback); ok {
		return tcb.runWithCallbacks(ctx, callbacks...)
	}
	for _, callback := range callbacks {
		if callback != nil {
			callback.BeforeRun(ctx, task)
		}
	}
	err := task.Run(ctx)
	for _, callback := range callbacks {
		if callback != nil {
			callback.AfterRun(ctx, task, err)
		}
	}
	return err
}

type taskWrapper struct {
	ctx     context.Context
	task    Task
	options taskOptions
}

func (w *taskWrapper) run(ctx context.Context, callbacks ...TaskCallback) error {
	if w.ctx != nil {
		ctx = w.ctx
	}
	return runTask(ctx, w.task, joinTaskCallbacks(callbacks, []TaskCallback{w.options.callback})...)
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
