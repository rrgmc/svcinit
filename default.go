package svcinit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

// SignalTask returns a task that returns when one of the passed OS signals is received.
func SignalTask(signals ...os.Signal) *TaskSignalTask {
	return &TaskSignalTask{signals}
}

// TimeoutTask stops the task after the specified timeout, or the context is done.
// By default, a TimeoutError is returned on timeout.
func TimeoutTask(timeout time.Duration, options ...TimeoutTaskOption) *TaskTimeoutTask {
	ret := &TaskTimeoutTask{
		timeout: timeout,
	}
	for _, opt := range options {
		opt(ret)
	}
	return ret
}

type TaskSignalTask struct {
	signals []os.Signal
}

var _ Task = (*TaskSignalTask)(nil)
var _ TaskSteps = (*TaskSignalTask)(nil)
var _ TaskWithOptions = (*TaskTimeoutTask)(nil)

func (t *TaskSignalTask) Signals() []os.Signal {
	return t.signals
}

func (t *TaskSignalTask) Run(ctx context.Context, step Step) error {
	switch step {
	case StepStart:
		c := make(chan os.Signal, 1)
		signal.Notify(c, t.signals...)
		select {
		case sig := <-c:
			return SignalError{Signal: sig}
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	default:
	}
	return nil
}

func (t *TaskSignalTask) TaskSteps() []Step {
	return []Step{StepStart}
}

func (t *TaskSignalTask) TaskOptions() []TaskInstanceOption {
	return []TaskInstanceOption{
		WithCancelContext(true),
	}
}

func (t *TaskSignalTask) String() string {
	return fmt.Sprintf("Signals %v", t.signals)
}

type TaskTimeoutTask struct {
	timeout        time.Duration
	returnNilError bool
	timeoutErr     error
}

var _ Task = (*TaskTimeoutTask)(nil)
var _ TaskSteps = (*TaskTimeoutTask)(nil)
var _ TaskWithOptions = (*TaskTimeoutTask)(nil)

func (t *TaskTimeoutTask) Timeout() time.Duration {
	return t.timeout
}

func (t *TaskTimeoutTask) Run(ctx context.Context, step Step) error {
	switch step {
	case StepStart:
		select {
		case <-ctx.Done():
		case <-time.After(t.timeout):
			if t.returnNilError {
				return nil
			} else if t.timeoutErr != nil {
				return t.timeoutErr
			}
			return &TimeoutError{
				Timeout: t.timeout,
			}
		}
	default:
	}
	return nil
}

func (t *TaskTimeoutTask) TaskSteps() []Step {
	return []Step{StepStart}
}

func (t *TaskTimeoutTask) TaskOptions() []TaskInstanceOption {
	return []TaskInstanceOption{
		WithCancelContext(true),
	}
}

func (t *TaskTimeoutTask) String() string {
	return fmt.Sprintf("Timeout %v", t.timeout)
}

type TimeoutTaskOption func(task *TaskTimeoutTask)

// WithTimeoutTaskError sets an error to be returned from the timeout task instead of TimeoutError.
func WithTimeoutTaskError(timeoutErr error) TimeoutTaskOption {
	return func(task *TaskTimeoutTask) {
		task.timeoutErr = timeoutErr
	}
}

// WithoutTimeoutTaskError returns a nil error in case of timeout.
func WithoutTimeoutTaskError() TimeoutTaskOption {
	return func(task *TaskTimeoutTask) {
		task.returnNilError = true
	}
}

// TimeoutError is returned by TimeoutTask by default.
type TimeoutError struct {
	Timeout time.Duration
}

func (e TimeoutError) Error() string {
	return fmt.Sprintf("timed out after %v", e.Timeout)
}

// SignalError is returned from SignalTask if the signal was received.
type SignalError struct {
	Signal os.Signal
}

func (e SignalError) Error() string {
	return fmt.Sprintf("received signal %s", e.Signal)
}
