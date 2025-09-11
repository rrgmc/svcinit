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

func SignalTask(signals ...os.Signal) Task {
	return func(ctx context.Context) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, signals...)
		select {
		case sig := <-c:
			return SignalError{Signal: sig}
		case <-ctx.Done():
			fmt.Printf("signal task done: %v\n", context.Cause(ctx))
			return context.Cause(ctx)
		}
	}
}
