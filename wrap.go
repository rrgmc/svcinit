package svcinit

import (
	"context"
	"fmt"
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

type wrappedTask struct {
	*baseWrappedTaskPrivate
	handler     TaskHandler
	description string
}

var _ Task = (*wrappedTask)(nil)
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

func (t *wrappedTask) String() string {
	if t.description != "" {
		return t.description
	}
	return fmt.Sprintf("wrappedTask(%v)", t.Task)
}
