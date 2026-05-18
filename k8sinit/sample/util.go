package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
)

var (
	initLogTime time.Time
	initLogOnce sync.Once
)

// defaultLogger is the default logger to be used internally.
func defaultLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog2.LevelTrace,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			initLogOnce.Do(func() {
				initLogTime = time.Now()
			})

			if a.Key == slog.TimeKey {
				t := a.Value.Time()
				return slog.String(slog.TimeKey, formatDuration(t.Sub(initLogTime)))
			} else {
				return slog2.ReplaceAttr(groups, a)
			}
		},
	}))
}

func formatDuration(d time.Duration) string {
	minute := int(d.Minutes()) % 60
	second := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d.%03d", minute, second, ms)
}

// sleepContext sleeps while checking for context cancellation.
// Returns nil for any option by default. These can be changed by options.
func sleepContext(ctx context.Context, duration time.Duration, options ...sleepContextOption) error {
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

type sleepContextOption func(*sleepContextOptions)

func withSleepContextError(contextError bool) sleepContextOption {
	return func(opts *sleepContextOptions) {
		opts.contextError = contextError
	}
}

func withSleepContextTimeoutError(timeoutErr error) sleepContextOption {
	return func(o *sleepContextOptions) {
		o.timeoutErr = timeoutErr
	}
}

type sleepContextOptions struct {
	contextError bool
	timeoutErr   error
}
