package svcinit

import (
	"context"
	"fmt"
)

type Task interface {
	Run(ctx context.Context, step Step) error
}

type TaskFunc func(ctx context.Context, step Step) error

func (t TaskFunc) Run(ctx context.Context, step Step) error {
	return t(ctx, step)
}

func (t TaskFunc) String() string {
	return fmt.Sprintf("%T", t)
}

type TaskHandler func(ctx context.Context, task Task, step Step) error

// TaskSteps sets the steps that the task implements. They will be the only ones called.
type TaskSteps interface {
	TaskSteps() []Step
}

// DefaultTaskSteps returns the default value for [TaskSteps.TaskSteps], which is the list of all steps.
func DefaultTaskSteps() []Step {
	return allSteps
}

// TaskWithOptions allows the task to set some of the task options. They have priority over options set via
// [Manager.AddTask].
type TaskWithOptions interface {
	TaskOptions() []TaskInstanceOption
}

// TaskWithInitError allows a task to report an initialization error. The error might be nil.
type TaskWithInitError interface {
	TaskInitError() error
}

// WithCancelContext sets whether to automatically cancel the task start step context when the first task finishes.
// The default is false, meaning that the stop step should handle to stop the task.
func WithCancelContext(cancelContext bool) TaskAndInstanceOption {
	return taskGlobalOptionFunc(func(options *taskOptions) {
		options.cancelContext = cancelContext
	})
}

// WithStartStepManager sets whether to add a StartStepManager to the stop step context.
// This allows the stop step to cancel the start step context and/or wait for its completion.
func WithStartStepManager() TaskAndInstanceOption {
	return taskGlobalOptionFunc(func(options *taskOptions) {
		options.startStepManager = true
	})
}

// WithHandler adds a task handler.
func WithHandler(handler TaskHandler) TaskOption {
	return taskOptionFunc(func(options *taskOptions) {
		options.handler = handler
	})
}

// WithCallback adds callbacks for the task.
func WithCallback(callbacks ...TaskCallback) TaskOption {
	return taskOptionFunc(func(options *taskOptions) {
		options.callbacks = append(options.callbacks, callbacks...)
	})
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
