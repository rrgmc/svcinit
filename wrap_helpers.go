package svcinit

import (
	"context"
)

type BaseOverloadedTask struct {
	Task Task
}

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

type baseOverloadedTaskPrivate = BaseOverloadedTask

type baseWrappedTaskPrivate = BaseWrappedTask
