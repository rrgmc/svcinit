package testutils

import (
	"context"
	"time"
)

// SleepContext sleeps while checking for context cancellation.
// Returns nil for any option by default. These can be changed by options.
func SleepContext(ctx context.Context, duration time.Duration, options ...SleepContextOption) error {
	var optns sleepContextOptions
	for _, opt := range options {
		opt(&optns)
	}
	select {
	case <-ctx.Done():
		if optns.contextError {
			return context.Cause(ctx)
		}
		return nil
	case <-time.After(duration):
		return optns.timeoutErr
	}
}

type SleepContextOption func(*sleepContextOptions)

func WithSleepContextError(contextError bool) SleepContextOption {
	return func(opts *sleepContextOptions) {
		opts.contextError = contextError
	}
}

func WithSleepContextTimeoutError(timeoutErr error) SleepContextOption {
	return func(o *sleepContextOptions) {
		o.timeoutErr = timeoutErr
	}
}

type sleepContextOptions struct {
	contextError bool
	timeoutErr   error
}
