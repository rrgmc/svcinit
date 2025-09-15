package svcinit

import (
	"context"
	"time"
)

func New(ctx context.Context, options ...Option) *SvcInit {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	unorderedCancelCtx, unorderedCancel := context.WithCancelCause(ctx)
	s := &SvcInit{
		ctx:                    ctx,
		cancelCtx:              cancelCtx,
		cancel:                 cancel,
		unorderedCancelCtx:     unorderedCancelCtx,
		unorderedCancel:        unorderedCancel,
		shutdownTimeout:        10 * time.Second,
		enforceShutdownTimeout: true,
	}
	s.SetOptions(options...)
	return s
}

// SetOptions allows overriding the options.
func (s *SvcInit) SetOptions(options ...Option) {
	for _, opt := range options {
		opt(s)
	}
	if s.shutdownCtx == nil {
		s.shutdownCtx = context.WithoutCancel(s.ctx)
	}
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
// If less then or equal to 0, no shutdown timeout will be set.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *SvcInit) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithEnforceShutdownTimeout don't wait for all shutdown tasks to complete if they are over the shutdown timeout.
// Usually the shutdown timeout only sets a timeout in the context, but it can't guarantee that all tasks will follow it.
// Default is true.
func WithEnforceShutdownTimeout(enforceShutdownTimeout bool) Option {
	return func(s *SvcInit) {
		s.enforceShutdownTimeout = enforceShutdownTimeout
	}
}

// WithStartingCallback appends a callback to be called before tasks are run.
// Returning an error will skip running all tasks and just returns the error from [SvcInit.Run].
func WithStartingCallback(startingCallback func(ctx context.Context) error) Option {
	return func(s *SvcInit) {
		s.startingCallback = append(s.startingCallback, startingCallback)
	}
}

// WithStartedCallback appends a callback to be called after all tasks were initialized.
// Returning an error will be the same as if one of the tasks returned that error.
func WithStartedCallback(startedCallback func(ctx context.Context) error) Option {
	return func(s *SvcInit) {
		s.startedCallback = append(s.startedCallback, startedCallback)
	}
}

// WithStoppingCallback appends a callback to be called before tasks area stopped.
// WARNING: returning an error from this callback WILL SKIP STOPPING TASKS and just returns the error from the
// [SvcInit.Run] function.
func WithStoppingCallback(stoppingCallback func(ctx context.Context, cause error) error) Option {
	return func(s *SvcInit) {
		s.stoppingCallback = append(s.stoppingCallback, stoppingCallback)
	}
}

// WithStoppedCallback appends a callback to be called after all tasks were stopped.
// Returning an error will be the same as if one stop task returned an error.
func WithStoppedCallback(stoppedCallback func(ctx context.Context, cause error) error) Option {
	return func(s *SvcInit) {
		s.stoppedCallback = append(s.stoppedCallback, stoppedCallback)
	}
}

// WithStartTaskCallback appends a function that is called before and after each start task runs.
func WithStartTaskCallback(startTaskCallback TaskCallback) Option {
	return func(s *SvcInit) {
		s.startTaskCallback = append(s.startTaskCallback, startTaskCallback)
	}
}

// WithStopTaskCallback adds a function that is called before and after each stop task runs.
func WithStopTaskCallback(stopTaskCallback TaskCallback) Option {
	return func(s *SvcInit) {
		s.stopTaskCallback = append(s.stopTaskCallback, stopTaskCallback)
	}
}

type Option func(*SvcInit)
