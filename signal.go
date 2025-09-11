package svcinit_poc1

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

type SignalError struct {
	Signal os.Signal
}

// Error implements the error interface.
func (e SignalError) Error() string {
	return fmt.Sprintf("received signal %s", e.Signal)
}

// SignalTask returns a task that returns when one of the passed OS signals is received.
func SignalTask(signals ...os.Signal) Task {
	return func(ctx context.Context) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, signals...)
		select {
		case sig := <-c:
			return SignalError{Signal: sig}
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}
