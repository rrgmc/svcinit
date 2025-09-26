package svcinit

import (
	"context"
)

// TaskWithWrapped is a task which was wrapped from another Task.
type TaskWithWrapped interface {
	Task
	WrappedTask() Task
}

// WrapTask wraps a task in a TaskWithWrapped, allowing the handler to be customized.
func WrapTask(task Task, options ...WrapTaskOption) TaskWithWrapped {
	return newWrappedTask(task, options...)
}

type WrapTaskOption func(task *wrappedTask)

// WithWrapTaskHandler sets an optional handler for the task.
func WithWrapTaskHandler(handler TaskHandler) WrapTaskOption {
	return func(task *wrappedTask) {
		task.handler = handler
	}
}

// WithWrapDescription sets the task description.
func WithWrapDescription(description string) WrapTaskOption {
	return func(task *wrappedTask) {
		task.description = description
	}
}

// UnwrapTask unwraps TaskWithWrapped from tasks.
func UnwrapTask(task Task) Task {
	if task == nil {
		return nil
	}
	for {
		if tc, ok := task.(TaskWithWrapped); ok && tc.WrappedTask() != nil {
			task = tc.WrappedTask()
		} else {
			return task
		}
	}
}

// BaseOverloadedTask wraps and task and forwards TaskOptions and TaskSteps.
// It doesn't implement TaskWithWrapped.
type BaseOverloadedTask struct {
	Task Task
}

var _ TaskName = (*BaseOverloadedTask)(nil)
var _ TaskSteps = (*BaseOverloadedTask)(nil)
var _ TaskWithOptions = (*BaseOverloadedTask)(nil)

func (t *BaseOverloadedTask) TaskOptions() []TaskInstanceOption {
	if tt, ok := t.Task.(TaskWithOptions); ok {
		return tt.TaskOptions()
	}
	return nil
}

func (t *BaseOverloadedTask) TaskSteps() []Step {
	if tt, ok := t.Task.(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return DefaultTaskSteps()
}

func (t *BaseOverloadedTask) TaskName() string {
	return GetTaskName(t.Task)
}

func (t *BaseOverloadedTask) String() string {
	return GetTaskDescription(t.Task)
}

// BaseWrappedTask wraps and task and forwards TaskOptions and TaskSteps.
// It implements TaskWithWrapped.
type BaseWrappedTask struct {
	*BaseOverloadedTask
}

func NewBaseWrappedTask(task Task) *BaseWrappedTask {
	return &BaseWrappedTask{
		&BaseOverloadedTask{
			Task: task,
		},
	}
}

var _ TaskSteps = (*BaseWrappedTask)(nil)
var _ TaskWithOptions = (*BaseWrappedTask)(nil)
var _ TaskWithWrapped = (*BaseWrappedTask)(nil)

func (t *BaseWrappedTask) Run(ctx context.Context, step Step) error {
	return t.Task.Run(ctx, step)
}

func (t *BaseWrappedTask) WrappedTask() Task {
	return t.Task
}

// internal

type wrappedTask struct {
	*baseWrappedTaskPrivate
	handler     TaskHandler
	description string
}

var _ Task = (*wrappedTask)(nil)
var _ TaskName = (*wrappedTask)(nil)
var _ TaskSteps = (*wrappedTask)(nil)
var _ TaskWithOptions = (*wrappedTask)(nil)
var _ TaskWithWrapped = (*wrappedTask)(nil)

func newWrappedTask(task Task, options ...WrapTaskOption) *wrappedTask {
	ret := &wrappedTask{
		baseWrappedTaskPrivate: NewBaseWrappedTask(task),
	}
	for _, option := range options {
		option(ret)
	}
	return ret
}

func (t *wrappedTask) Run(ctx context.Context, step Step) error {
	if t.handler != nil {
		return t.handler(ctx, t.Task, step)
	}
	return t.Task.Run(ctx, step)
}

func (t *wrappedTask) TaskName() string {
	if t.description != "" {
		return t.description
	}
	return GetTaskName(t.Task)
}

func (t *wrappedTask) String() string {
	if tn := t.TaskName(); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t.Task)
}

type baseOverloadedTaskPrivate = BaseOverloadedTask

type baseWrappedTaskPrivate = BaseWrappedTask
