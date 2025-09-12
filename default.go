package svcinit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

// TimeoutTask stops the task after the specified timeout, or the context is done.
// timeoutErr can be nil.
func TimeoutTask(timeout time.Duration, timeoutErr error) *TaskTimeoutTask {
	return &TaskTimeoutTask{
		timeout:    timeout,
		timeoutErr: timeoutErr,
	}
}

// SignalTask returns a task that returns when one of the passed OS signals is received.
func SignalTask(signals ...os.Signal) *TaskSignalTask {
	return &TaskSignalTask{signals}
}

type TaskTimeoutTask struct {
	timeout    time.Duration
	timeoutErr error
}

func (t *TaskTimeoutTask) Timeout() time.Duration {
	return t.timeout
}

func (t *TaskTimeoutTask) TimeoutErr() error {
	return t.timeoutErr
}

func (t *TaskTimeoutTask) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
	case <-time.After(t.timeout):
		return t.timeoutErr
	}
	return nil
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

type SignalError struct {
	Signal os.Signal
}

// Error implements the error interface.
func (e SignalError) Error() string {
	return fmt.Sprintf("received signal %s", e.Signal)
}
