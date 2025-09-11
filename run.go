package svcinit_poc1

import (
	"context"
	"errors"
)

func (s *SvcInit) start() {
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
	var errs []error

	ctx, cancel := context.WithTimeout(s.ctx, s.shutdownTimeout)
	defer cancel()
	for _, fn := range s.cleanup {
		err := fn(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (s *SvcInit) checkPending() error {
	for _, task := range s.pendingStarts {
		if !task.isResolved() {
			return errors.New("all start commands must be resolved")
		}
	}
	return nil
}

func (s *SvcInit) addPending(p pendingStart) {
	s.pendingStarts = append(s.pendingStarts, p)
}
