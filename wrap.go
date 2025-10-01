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

// WithWrapName sets the task name.
func WithWrapName(name string) WrapTaskOption {
	return func(task *wrappedTask) {
		task.name = name
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

// BaseOverloadedTask wraps a task and forwards all task interface methods.
// It doesn't implement TaskWithWrapped.
type BaseOverloadedTask[T Task] struct {
	Task T
}

var _ TaskName = (*BaseOverloadedTask[Task])(nil)
var _ TaskSteps = (*BaseOverloadedTask[Task])(nil)
var _ TaskWithOptions = (*BaseOverloadedTask[Task])(nil)

func (t *BaseOverloadedTask[T]) TaskOptions() []TaskInstanceOption {
	if tt, ok := any(t.Task).(TaskWithOptions); ok {
		return tt.TaskOptions()
	}
	return nil
}

func (t *BaseOverloadedTask[T]) TaskSteps() []Step {
	if tt, ok := any(t.Task).(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return DefaultTaskSteps()
}

func (t *BaseOverloadedTask[T]) TaskName() string {
	return GetTaskName(t.Task)
}

func (t *BaseOverloadedTask[T]) String() string {
	if tn := t.TaskName(); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t.Task)
}

// BaseWrappedTask wraps a task and forwards all task interface methods.
// It implements TaskWithWrapped.
type BaseWrappedTask[T Task] struct {
	*BaseOverloadedTask[T]
}

func NewBaseWrappedTask[T Task](task T) *BaseWrappedTask[T] {
	return &BaseWrappedTask[T]{
		&BaseOverloadedTask[T]{
			Task: task,
		},
	}
}

var _ TaskName = (*BaseWrappedTask[Task])(nil)
var _ TaskSteps = (*BaseWrappedTask[Task])(nil)
var _ TaskWithOptions = (*BaseWrappedTask[Task])(nil)
var _ TaskWithWrapped = (*BaseWrappedTask[Task])(nil)

func (t *BaseWrappedTask[T]) Run(ctx context.Context, step Step) error {
	return t.Task.Run(ctx, step)
}

func (t *BaseWrappedTask[T]) WrappedTask() Task {
	return t.Task
}

// internal

type wrappedTask struct {
	*baseWrappedTaskPrivate[Task]
	handler TaskHandler
	name    string
}

var _ Task = (*wrappedTask)(nil)
var _ TaskName = (*wrappedTask)(nil)
var _ TaskSteps = (*wrappedTask)(nil)
var _ TaskWithOptions = (*wrappedTask)(nil)
var _ TaskWithWrapped = (*wrappedTask)(nil)

func newWrappedTask(task Task, options ...WrapTaskOption) *wrappedTask {
	ret := &wrappedTask{
		baseWrappedTaskPrivate: NewBaseWrappedTask[Task](task),
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
	if t.name != "" {
		return t.name
	}
	return GetTaskName(t.Task)
}

func (t *wrappedTask) String() string {
	if tn := t.TaskName(); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t.Task)
}

type baseOverloadedTaskPrivate[T Task] = BaseOverloadedTask[T]

type baseWrappedTaskPrivate[T Task] = BaseWrappedTask[T]
