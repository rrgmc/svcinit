package svcinit

import (
	"context"
	"log/slog"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
)

// StartStepManager allows the "stop" step to cancel the start step and/or wait for it to finish.
type StartStepManager interface {
	ContextCancel(cause error) bool // cancel the "start" step context. Returns whether the cancellation was possible.
	Finished() <-chan struct{}      // channel that will be closed once the "start" step finishes.
	CanContextCancel() bool         // returns whether the "start" step context can be called.
	CanFinished() bool              // returns whether the Finished channel can be checked. If false, Finished will return a nil channel.
}

// StartStepManagerFromContext returns a StartStepManager from the stop step's context.
// If not available returns a noop instance.
func StartStepManagerFromContext(ctx context.Context) StartStepManager {
	if val := ctx.Value(startStepManagerContextKey{}); val != nil {
		if sc, ok := val.(*startStepManager); ok {
			return sc
		}
	}
	return &startStepManager{}
}

// CauseFromContext gets the stop cause from the context, if available.
func CauseFromContext(ctx context.Context) (error, bool) {
	if val := ctx.Value(causeKey{}); val != nil {
		if err, ok := val.(error); ok {
			return err, true
		}
	}
	return nil, false
}

// internal

type startStepManagerContextKey struct{}

type causeKey struct{}

func contextWithCause(ctx context.Context, cause error) context.Context {
	return context.WithValue(ctx, causeKey{}, cause)
}

func contextWithStartStepManager(ctx context.Context, stopContext *startStepManager) context.Context {
	return context.WithValue(ctx, startStepManagerContextKey{}, stopContext)
}

type startStepManager struct {
	logger   *slog.Logger
	cancel   context.CancelCauseFunc
	finished context.Context
}

var _ StartStepManager = (*startStepManager)(nil)

func (s *startStepManager) ContextCancel(cause error) bool {
	if s.cancel != nil {
		s.logger.Log(context.Background(), slog2.LevelTrace, "ssm: canceling context",
			"cause", cause)
		s.cancel(cause)
		return true
	}
	return false
}

func (s *startStepManager) Finished() <-chan struct{} {
	if s.finished != nil {
		return s.finished.Done()
	}
	return nil
}

func (s *startStepManager) CanContextCancel() bool {
	return s.cancel != nil
}

func (s *startStepManager) CanFinished() bool {
	return s.finished != nil
}
