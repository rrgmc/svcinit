package svcinit

import (
	"context"
	"time"
)

func New(ctx context.Context, options ...Option) *Manager {
	cancelCtx, cancel := context.WithCancelCause(ctx)
	unorderedCancelCtx, unorderedCancel := context.WithCancelCause(ctx)
	s := &Manager{
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
func (s *Manager) SetOptions(options ...Option) {
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
	return func(s *Manager) {
		s.shutdownCtx = shutdownCtx
	}
}

// WithShutdownTimeout sets a shutdown timeout. The default is 10 seconds.
// If less then or equal to 0, no shutdown timeout will be set.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithEnforceShutdownTimeout don't wait for all shutdown tasks to complete if they are over the shutdown timeout.
// Usually the shutdown timeout only sets a timeout in the context, but it can't guarantee that all tasks will follow it.
// Default is true.
func WithEnforceShutdownTimeout(enforceShutdownTimeout bool) Option {
	return func(s *Manager) {
		s.enforceShutdownTimeout = enforceShutdownTimeout
	}
}

// WithManagerCallback appends a function that is called before and after each lifecycle event happens.
func WithManagerCallback(managerCallback ManagerCallback) Option {
	return func(s *Manager) {
		s.managerCallbacks = append(s.managerCallbacks, managerCallback)
	}
}

// WithGlobalTaskCallback appends a function that is called before and after each task runs.
func WithGlobalTaskCallback(taskCallback TaskCallback) Option {
	return func(s *Manager) {
		s.globalTaskCallbacks = append(s.globalTaskCallbacks, taskCallback)
	}
}

type Option func(*Manager)
