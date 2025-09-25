package svcinit

import (
	"context"
)

// TaskFuture is a Task with data where the return of the setup step will resolve the Future.
type TaskFuture[T any] interface {
	Task
	Future[T]
}

func NewTaskFuture[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) TaskFuture[T] {
	dr := NewFuture[T]()
	return &taskFuture[T]{
		BaseOverloadedTask: &BaseOverloadedTask{BuildDataTask[T](func(ctx context.Context) (T, error) {
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
