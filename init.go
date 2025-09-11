package svcinit_poc1

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrExit    = errors.New("normal exit")
	ErrPending = errors.New("pending start or stop command")
)

// SvcInit schedules tasks to be run and stopped on service initialization.
// The first task to return, with an error or nil, will cause all the other tasks to stop and return the error
// from that one.
type SvcInit struct {
	ctx              context.Context
	cancelCtx        context.Context
	cancel           context.CancelCauseFunc
	serviceCancelCtx context.Context
	serviceCancel    context.CancelCauseFunc
	startedCallback  Task
	tasks            []taskWrapper
	autoCleanup      []Task
	cleanup          []Task
	pendingStarts    []pendingTask
	pendingStops     []pendingTask
	wg               sync.WaitGroup
	shutdownTimeout  time.Duration
}

func New(ctx context.Context, options ...Option) *SvcInit {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	serviceCancelCtx, serviceCancel := context.WithCancelCause(ctx)
	s := &SvcInit{
		ctx:              ctx,
		cancelCtx:        cancelCtx,
		cancel:           cancel,
		serviceCancelCtx: serviceCancelCtx,
		serviceCancel:    serviceCancel,
		shutdownTimeout:  10 * time.Second,
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
	s.serviceCancel(context.Cause(s.cancelCtx))
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

// Shutdown starts the shutdown process as if a task returned.
func (s *SvcInit) Shutdown() {
	s.cancel(ErrExit)
}

func (s *SvcInit) SetStartedCallback(startedCallback Task) {
	s.startedCallback = startedCallback
}

// WithShutdownTimeout sets a shutdown timeout. The default is 10 seconds.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *SvcInit) {
		s.shutdownTimeout = shutdownTimeout
	}
}

func WithStartedCallback(startedCallback Task) Option {
	return func(s *SvcInit) {
		s.startedCallback = startedCallback
	}
}

type Option func(*SvcInit)

type taskWrapper struct {
	ctx             context.Context
	task            Task
	taskFinishedCtx context.Context
	taskFinished    context.CancelFunc
}
