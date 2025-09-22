package svcinit

import "context"

type Task interface {
	Run(ctx context.Context, step Step) error
}

type TaskFunc func(ctx context.Context, step Step) error

func (t TaskFunc) Run(ctx context.Context, step Step) error {
	return t(ctx, step)
}

type TaskSteps interface {
	TaskSteps() []Step
}

func DefaultTaskSteps() []Step {
	return allSteps
}

type TaskWithOptions interface {
	TaskOptions() []TaskInstanceOption
}

type TaskOption interface {
	applyTaskOpt(options *taskOptions)
}

type TaskInstanceOption interface {
	applyTaskInstanceOpt(options *taskOptions)
}

type TaskAndInstanceOption interface {
	TaskOption
	TaskInstanceOption
}

func WithStage(stage string) TaskOption {
	return taskOptionFunc(func(options *taskOptions) {
		options.stage = stage
	})
}

func WithCancelContext(cancelContext bool) TaskAndInstanceOption {
	return taskGlobalOptionFunc(func(options *taskOptions) {
		options.cancelContext = cancelContext
	})
}

func WithStartStepManager() TaskAndInstanceOption {
	return taskGlobalOptionFunc(func(options *taskOptions) {
		options.startStepManager = true
	})
}

func WithCallback(callbacks ...TaskCallback) TaskOption {
	return taskOptionFunc(func(options *taskOptions) {
		options.callbacks = append(options.callbacks, callbacks...)
	})
}
