package svcinit

import "context"

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
	return t.future.Value(options...)
}

func (t *taskFuture[T]) Done() <-chan struct{} {
	return t.future.Done()
}
