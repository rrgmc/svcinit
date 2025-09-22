package svcinit

import "context"

// TaskWithWrapped is a task which was wrapped from one Task.
type TaskWithWrapped interface {
	Task
	WrappedTask() Task
}

// NewWrappedTask wraps a task in a TaskWithWrapped, allowing the handler to be customized.
func NewWrappedTask(task Task, options ...WrapTaskOption) *WrappedTask {
	ret := &WrappedTask{
		task: task,
	}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type WrapTaskOption func(task *WrappedTask)

// WithWrapTaskHandler sets an optional handler for the task.
func WithWrapTaskHandler(handler func(ctx context.Context, task Task) error) WrapTaskOption {
	return func(task *WrappedTask) {
		task.handler = handler
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

type WrappedTask struct {
	task    Task
	handler func(ctx context.Context, task Task) error
}

var _ Task = (*WrappedTask)(nil)
var _ TaskSteps = (*WrappedTask)(nil)
var _ TaskWithOptions = (*WrappedTask)(nil)
var _ TaskWithWrapped = (*WrappedTask)(nil)

func (t *WrappedTask) Run(ctx context.Context, step Step) error {
	if t.handler == nil {
		return t.task.Run(ctx, step)
	}
	return t.handler(ctx, t.task)
}

func (t *WrappedTask) TaskOptions() []TaskInstanceOption {
	if tt, ok := t.task.(TaskWithOptions); ok {
		return tt.TaskOptions()
	}
	return nil
}

func (t *WrappedTask) TaskSteps() []Step {
	if tt, ok := t.task.(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return DefaultTaskSteps()
}

func (t *WrappedTask) WrappedTask() Task {
	return t.task
}
