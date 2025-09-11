package svcinit_poc1

import (
	"context"
	"errors"
	"sync"
)

func (s *SvcInit) start() {
	// start all tasks in separate goroutines.
	for _, task := range s.tasks {
		s.wg.Add(1)
		go func(ctx context.Context, fn Task) {
			defer s.wg.Done()
			err := fn(ctx)
			if err != nil {
				s.cancel(err)
			} else {
				s.cancel(ErrExit)
			}
		}(task.ctx, task.task)
	}
}

func (s *SvcInit) shutdown() []error {
	var (
		wg   sync.WaitGroup
		lock sync.Mutex
		errs []error
	)

	ctx, cancel := context.WithTimeout(s.ctx, s.shutdownTimeout)
	defer cancel()

	if len(s.autoCleanup) > 0 {
		// cleanups where order don't matter are done in parallel
		wg.Add(len(s.autoCleanup))
		for _, fn := range s.autoCleanup {
			go func(fn Task) {
				defer wg.Done()
				err := fn(ctx)
				if err != nil {
					lock.Lock()
					errs = append(errs, err)
					lock.Unlock()
				}
			}(fn)
		}
	}

	// execute ordered cleanups synchronously
	for _, fn := range s.cleanup {
		err := fn(ctx)
		if err != nil {
			lock.Lock()
			errs = append(errs, err)
			lock.Unlock()
		}
	}

	// wait for auto cleanups, if any
	wg.Wait()

	return errs
}

func (s *SvcInit) checkPending() error {
	for _, task := range s.pendingStarts {
		if !task.isResolved() {
			return errors.New("all start commands must be resolved")
		}
	}
	for _, task := range s.pendingStops {
		if !task.isResolved() {
			return errors.New("all stop commands must be resolved")
		}
	}
	return nil
}

func (s *SvcInit) addPendingStart(p pendingTask) {
	s.pendingStarts = append(s.pendingStarts, p)
}

func (s *SvcInit) addPendingStop(p pendingTask) {
	s.pendingStops = append(s.pendingStops, p)
}

func (s *SvcInit) addPendingStopTask(task Task) StopTask {
	st := newPendingStopTask(task)
	s.pendingStops = append(s.pendingStops, st)
	return st
}
