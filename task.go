package svcinit

import (
	"context"
	"fmt"
	"slices"
)

type Task interface {
	Run(ctx context.Context, step Step) error
}

type TaskWithData[T any] interface {
	Task
	TaskData() (T, error)
}

type TaskFunc func(ctx context.Context, step Step) error

func (t TaskFunc) Run(ctx context.Context, step Step) error {
	return t(ctx, step)
}

func (t TaskFunc) String() string {
	return getDefaultTaskDescription(t)
}

// TaskHandler should call task.Run(ctx, step) and return its error. It can do any processing it needs before and
// after the call.
// WARNING: not calling the method, or calling it for any other step, will break the promise of never calling
// the steps out of order or multiple times.
type TaskHandler func(ctx context.Context, task Task, step Step) error

// TaskInfo describes the optional metadata and behavior of a task. All fields are optional, and the zero value
// is the default for a task.
// It is the single extension point of a task: a task decorating another one should merge the inner task's
// TaskInfo instead of forwarding a set of interfaces. [BuildTask] with [WithParent] does this.
type TaskInfo struct {
	// Name is the task name.
	Name string
	// Steps are the steps that the task implements. They will be the only ones called.
	// If nil, all steps are called.
	Steps []Step
	// Options are task options set by the task itself. They have priority over options set via [Manager.AddTask].
	Options []TaskInstanceOption
	// InitError is a task initialization error. If not nil, [Manager.AddTask] won't add the task, and
	// [Manager.Run] will return the error.
	InitError error
	// Skipped is called when [Manager.Run] returns without having run any of the task steps, for example because
	// a setup step of a previous stage failed. cause is the error returned from Run, and may be nil.
	// It can be used to release anything waiting on the task, like an unresolved [Future].
	Skipped func(ctx context.Context, cause error)
}

// TaskWithInfo allows a task to describe its optional metadata and behavior.
type TaskWithInfo interface {
	TaskInfo() TaskInfo
}

// GetTaskInfo returns the task info, or the zero value if the task don't implement [TaskWithInfo].
func GetTaskInfo(task Task) TaskInfo {
	if ti, ok := task.(TaskWithInfo); ok {
		return ti.TaskInfo()
	}
	return TaskInfo{}
}

// DefaultTaskSteps returns the list of all steps, which is the default for [TaskInfo.Steps].
func DefaultTaskSteps() []Step {
	return slices.Clone(allSteps)
}

// GetTaskName gets the name of task, or blank if it don't have one.
func GetTaskName(task Task) string {
	return GetTaskInfo(task).Name
}

// GetTaskDescription returns the task description, be it the String method, a task name, or its variable type.
func GetTaskDescription(task Task) string {
	if ts, ok := task.(fmt.Stringer); ok {
		return ts.String()
	}
	if tn := GetTaskName(task); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(task)
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

func getDefaultTaskDescription(task any) string {
	return fmt.Sprintf("%T", task)
}
