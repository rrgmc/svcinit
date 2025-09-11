package svcinit

import (
	"context"
	"time"
)

func New(ctx context.Context, options ...Option) *SvcInit {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	serviceCancelCtx, serviceCancel := context.WithCancelCause(ctx)
	s := &SvcInit{
		ctx:              ctx,
		cancelCtx:        cancelCtx,
		cancel:           cancel,
		serviceCancelCtx: serviceCancelCtx,
		serviceCancel:    serviceCancel,
		shutdownTimeout:  10 * time.Second,
	}
	for _, opt := range options {
		opt(s)
	}
	return s
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

type Option func(*SvcInit)
