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
	ret := &wrappedTask{
		task: task,
	}
	for _, option := range options {
		option(ret)
	}
	return ret
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
	task        Task
	handler     TaskHandler
	description string
}

var _ Task = (*wrappedTask)(nil)
var _ TaskSteps = (*wrappedTask)(nil)
var _ TaskWithOptions = (*wrappedTask)(nil)
var _ TaskWithWrapped = (*wrappedTask)(nil)

func (t *wrappedTask) Run(ctx context.Context, step Step) error {
	if t.handler != nil {
		return t.handler(ctx, t.task, step)
	}
	return t.task.Run(ctx, step)
}

func (t *wrappedTask) TaskOptions() []TaskInstanceOption {
	if tt, ok := t.task.(TaskWithOptions); ok {
		return tt.TaskOptions()
	}
	return nil
}

func (t *wrappedTask) TaskSteps() []Step {
	if tt, ok := t.task.(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return DefaultTaskSteps()
}

func (t *wrappedTask) WrappedTask() Task {
	return t.task
}

func (t *wrappedTask) String() string {
	if t.description != "" {
		return t.description
	}
	return fmt.Sprintf("wrappedTask(%v)", t.task)
}
