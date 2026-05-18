package futuretask

import (
	"context"
	"fmt"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/instancetask"
)

func NewTaskFuture[T any](setupFunc instancetask.TaskBuildSetupFunc[T],
	options ...instancetask.TaskBuildOption[T]) svcinit.TaskFuture[T] {
	dr := svcinit.NewFuture[T]()
	return &taskFuture[T]{
		BaseOverloadedTask: &svcinit.BaseOverloadedTask[svcinit.TaskWithData[T]]{instancetask.BuildTask[T](func(ctx context.Context) (T, error) {
			data, err := setupFunc(ctx)
			if err != nil {
				dr.ResolveError(err)
				var empty T
				return empty, err
			}
			dr.Resolve(data)
			return data, nil
		}, options...)},
		future: dr,
	}
}

// internal

type taskFuture[T any] struct {
	*svcinit.BaseOverloadedTask[svcinit.TaskWithData[T]]
	future svcinit.FutureResolver[T]
}

var _ svcinit.Future[int] = (*taskFuture[int])(nil)
var _ svcinit.Task = (*taskFuture[int])(nil)
var _ svcinit.TaskSteps = (*taskFuture[int])(nil)
var _ svcinit.TaskWithOptions = (*taskFuture[int])(nil)

func (t *taskFuture[T]) Run(ctx context.Context, step svcinit.Step) error {
	return t.Task.Run(ctx, step)
}

func (t *taskFuture[T]) Value(options ...svcinit.FutureValueOption) (T, error) {
	ret, err := t.future.Value(options...)
	if err != nil {
		return ret, fmt.Errorf("error resolving task data: %w", err)
	}
	return ret, nil
}

func (t *taskFuture[T]) Done() <-chan struct{} {
	return t.future.Done()
}
