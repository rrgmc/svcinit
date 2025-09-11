package svcinit_poc1

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrExit = errors.New("normal exit")
)

type SvcInit struct {
	ctx             context.Context
	cancelCtx       context.Context
	cancel          context.CancelCauseFunc
	tasks           []taskWrapper
	autoCleanup     []Task
	cleanup         []Task
	pendingStarts   []pendingStart
	wg              sync.WaitGroup
	shutdownTimeout time.Duration
}

func New(ctx context.Context, options ...Option) *SvcInit {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	s := &SvcInit{
		ctx:             ctx,
		cancelCtx:       cancelCtx,
		cancel:          cancel,
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range options {
		opt(s)
	}
	return s
}

func (s *SvcInit) RunWithErrors() (error, []error) {
	if err := s.checkPending(); err != nil {
		return err, nil
	}
	if len(s.tasks) == 0 {
		return nil, nil
	}
	s.start()
	<-s.cancelCtx.Done()
	cleanupErr := s.shutdown()
	s.wg.Wait()
	cause := context.Cause(s.cancelCtx)
	if errors.Is(cause, ErrExit) || errors.Is(cause, context.Canceled) {
		cause = nil
	}
	return cause, cleanupErr
}

func (s *SvcInit) Run() error {
	err, _ := s.RunWithErrors()
	return err
}

func (s *SvcInit) Shutdown() {
	s.cancel(ErrExit)
}

func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(g *SvcInit) {
		g.shutdownTimeout = shutdownTimeout
	}
}

type Option func(*SvcInit)

type taskWrapper struct {
	ctx  context.Context
	task Task
}
