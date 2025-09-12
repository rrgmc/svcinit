package svcinit

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
	// ctx is the original context passed on New.
	ctx context.Context
	// shutdownCtx is the context to use for shutdown. The default is context.WithoutCancel(ctx).
	shutdownCtx context.Context
	// cancelCtx and cancel are used to return the error of the first task to finish, and signal that the service should
	// shut down.
	cancelCtx context.Context
	cancel    context.CancelCauseFunc
	// unorderedCancelCtx and unorderedCancel are used as the context for unordered tasks and to cancel them.
	unorderedCancelCtx context.Context
	unorderedCancel    context.CancelCauseFunc
	// list of tasks to start.
	tasks []taskWrapper
	// list of ordered cleanup tasks.
	cleanup []Task
	// list of unordered cleanup tasks.
	autoCleanup []Task
	// list of pending starts and stops.
	pendingStarts []pendingTask
	pendingStops  []pendingTask
	// task finish wait group.
	wg sync.WaitGroup
	// options
	startedCallback Task
	stoppedCallback Task
	shutdownTimeout time.Duration
}

// RunWithErrors runs all tasks and returns the error of the first task to finish, which can be nil,
// and a list of stop errors, if any.
func (s *SvcInit) RunWithErrors() (error, []error) {
	if err := s.checkPending(); err != nil {
		return err, nil
	}
	if len(s.tasks) == 0 {
		return nil, nil
	}
	s.start()
	<-s.cancelCtx.Done()
	s.unorderedCancel(context.Cause(s.cancelCtx))
	cleanupErr := s.shutdown()
	s.wg.Wait()
	cause := context.Cause(s.cancelCtx)
	if errors.Is(cause, ErrExit) || errors.Is(cause, context.Canceled) {
		cause = nil
	}
	return cause, cleanupErr
}

// Run runs all tasks and returns the error of the first task to finish, which can be nil.
func (s *SvcInit) Run() error {
	err, _ := s.RunWithErrors()
	return err
}

// Shutdown starts the shutdown process as if a task finished.
func (s *SvcInit) Shutdown() {
	s.cancel(ErrExit)
}

// SetStartedCallback sets a callback to be called after all tasks were initialized.
// It overrides the WithStartedCallback option.
func (s *SvcInit) SetStartedCallback(startedCallback Task) {
	s.startedCallback = startedCallback
}

// SetStoppedCallback sets a callback to be called after all tasks were stopped.
// It overrides the WithStoppedCallback option.
func (s *SvcInit) SetStoppedCallback(stoppedCallback Task) {
	s.stoppedCallback = stoppedCallback
}

type taskWrapper struct {
	ctx  context.Context
	task Task
}
