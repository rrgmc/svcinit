package svcinit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

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

// SignalTask returns a task that returns when one of the passed OS signals is received.
func SignalTask(signals ...os.Signal) *TaskSignalTask {
	return &TaskSignalTask{signals}
}

type TaskTimeoutTask struct {
	timeout        time.Duration
	returnNilError bool
	timeoutErr     error
}

func (t *TaskTimeoutTask) Timeout() time.Duration {
	return t.timeout
}

func (t *TaskTimeoutTask) Run(ctx context.Context) error {
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
	return nil
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

type TaskSignalTask struct {
	signals []os.Signal
}

var _ Task = (*TaskSignalTask)(nil)

func (t *TaskSignalTask) Signals() []os.Signal {
	return t.signals
}

func (t *TaskSignalTask) Run(ctx context.Context) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, t.signals...)
	select {
	case sig := <-c:
		return SignalError{Signal: sig}
	case <-ctx.Done():
		return context.Cause(ctx)
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

// CheckNullTask returns NullTask if task is nil, otherwise return the task itself.
func CheckNullTask(task Task) Task {
	if task == nil {
		return &NullTask{}
	}
	return task
}

type NullTask struct {
}

func (n *NullTask) Run(_ context.Context) error {
	return nil
}
