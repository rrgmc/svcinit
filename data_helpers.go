package svcinit

import (
	"context"
	"fmt"
)

type taskFuture[T any] struct {
	*BaseOverloadedTask
	future FutureResolver[T]
}

var _ Future[int] = (*taskFuture[int])(nil)
var _ Task = (*taskFuture[int])(nil)
var _ TaskSteps = (*taskFuture[int])(nil)
var _ TaskWithOptions = (*taskFuture[int])(nil)

func (t *taskFuture[T]) Run(ctx context.Context, step Step) error {
	return t.Task.Run(ctx, step)
}

func (t *taskFuture[T]) Value(options ...FutureValueOption) (T, error) {
	ret, err := t.future.Value(options...)
	if err != nil {
		return ret, fmt.Errorf("error resolving task data: %w", err)
	}
	return ret, nil
}

func (t *taskFuture[T]) Done() <-chan struct{} {
	return t.future.Done()
}
