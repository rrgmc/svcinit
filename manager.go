package svcinit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrExit            = errors.New("normal exit")
	ErrShutdownTimeout = errors.New("shutdown timeout")
	ErrPending         = errors.New("pending start or stop command")
	ErrNoTask          = errors.New("no tasks available")
	ErrAlreadyRunning  = errors.New("already running")
	ErrNilTasks        = errors.New("nil tasks are not allowed")
)

// Manager schedules tasks to be run and stopped on service initialization.
// The first task to return, with an error or nil, will cause all the other tasks to stop and return the error
// from that one.
type Manager struct {
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
	// list of pre-stop tasks.
	preStopTasks []taskWrapper
	// list of unordered stop tasks.
	stopTasks []taskWrapper
	// list of ordered stop tasks.
	stopTasksOrdered []taskWrapper
	// list of pending starts and stops.
	pendingStarts []pendingItem
	pendingStops  []pendingItem
	// task finish wait group.
	wg        sync.WaitGroup
	isRunning atomic.Bool
	isNilTask atomic.Bool
	// options
	managerCallbacks       []ManagerCallback
	globalTaskCallbacks    []TaskCallback
	shutdownTimeout        time.Duration
	enforceShutdownTimeout bool
}

// RunWithErrors runs all tasks and returns the error of the first task to finish, which can be nil,
// and a list of stop errors, if any.
func (s *Manager) RunWithErrors() (cause error, stopErr error) {
	if !s.isRunning.CompareAndSwap(false, true) {
		return ErrAlreadyRunning, nil
	}

	if s.isNilTask.Load() {
		return ErrNilTasks, nil
	}

	if err := s.checkPending(); err != nil {
		return err, nil
	}
	if len(s.tasks) == 0 {
		return ErrNoTask, nil
	}
	err := s.start()
	if err != nil {
		return err, nil
	}
	<-s.cancelCtx.Done()
	s.unorderedCancel(context.Cause(s.cancelCtx))
	cause = context.Cause(s.cancelCtx)
	err, stopErr = s.shutdown(cause)
	if err != nil {
		return err, nil
	}
	s.wg.Wait()
	if errors.Is(cause, ErrExit) || errors.Is(cause, context.Canceled) {
		cause = nil
	}
	return cause, stopErr
}

// Run runs all tasks and returns the error of the first task to finish, which can be nil.
func (s *Manager) Run() error {
	err, _ := s.RunWithErrors()
	return err
}

// Shutdown starts the shutdown process as if a task finished.
func (s *Manager) Shutdown() {
	s.cancel(ErrExit)
}
