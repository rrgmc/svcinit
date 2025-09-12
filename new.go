package svcinit

import (
	"context"
	"time"
)

func New(ctx context.Context, options ...Option) *SvcInit {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	unorderedCancelCtx, unorderedCancel := context.WithCancelCause(ctx)
	s := &SvcInit{
		ctx:                ctx,
		cancelCtx:          cancelCtx,
		cancel:             cancel,
		unorderedCancelCtx: unorderedCancelCtx,
		unorderedCancel:    unorderedCancel,
		shutdownTimeout:    10 * time.Second,
	}
	for _, opt := range options {
		opt(s)
	}
	if s.shutdownCtx == nil {
		s.shutdownCtx = context.WithoutCancel(ctx)
	}
	return s
}

// WithShutdownContext sets a separate context to use for shutdown.
// If the main context can be cancelled, it can't be used for shutdown as the shutdown tasks won't run.
// The default is context.WithoutCancel(baseContext).
func WithShutdownContext(shutdownCtx context.Context) Option {
	return func(s *SvcInit) {
		s.shutdownCtx = shutdownCtx
	}
}

// WithShutdownTimeout sets a shutdown timeout. The default is 10 seconds.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *SvcInit) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithStartedCallback sets a callback to be called after all tasks were initialized.
func WithStartedCallback(startedCallback Task) Option {
	return func(s *SvcInit) {
		s.startedCallback = startedCallback
	}
}

// WithStoppedCallback sets a callback to be called after all tasks were stopped.
func WithStoppedCallback(stoppedCallback Task) Option {
	return func(s *SvcInit) {
		s.stoppedCallback = stoppedCallback
	}
}

type Option func(*SvcInit)
