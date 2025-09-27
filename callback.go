package svcinit

import (
	"context"
)

// TaskCallback is a callback for task events.
// err is only set for CallbackStepAfter.
type TaskCallback interface {
	Callback(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error)
}

type TaskCallbackFunc func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error)

func (f TaskCallbackFunc) Callback(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
	f(ctx, task, stage, step, callbackStep, err)
}

// ManagerCallback is a callback for manager events.
// A cause may be set in the context. Use CauseFromContext to check.
type ManagerCallback interface {
	Callback(ctx context.Context, stage string, step Step, callbackStep CallbackStep)
}

type ManagerCallbackFunc func(ctx context.Context, stage string, step Step, callbackStep CallbackStep)

func (f ManagerCallbackFunc) Callback(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
	f(ctx, stage, step, callbackStep)
}
