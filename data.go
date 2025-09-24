package svcinit

import (
	"context"
)

type ResolvedData interface {
	IsResolved() bool
	DoneResolved() <-chan struct{}
}

type TaskWithResolved interface {
	Task
	ResolvedData
}

type ResolvedDataTask[T any] struct {
	*baseOverloadedTaskPrivate
	*ResolvedDataType[T]
}

func NewResolvedDataTask[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) *ResolvedDataTask[T] {
	dr := NewResolvedDataType[T]()
	return &ResolvedDataTask[T]{
		baseOverloadedTaskPrivate: &baseOverloadedTaskPrivate{BuildDataTask[T](func(ctx context.Context) (T, error) {
			data, err := setupFunc(ctx)
			if err != nil {
				var empty T
				return empty, err
			}
			dr.SetResolved(data)
			return data, nil
		}, options...)},
		ResolvedDataType: dr,
	}
}

var _ TaskWithResolved = (*ResolvedDataTask[int])(nil)
var _ TaskSteps = (*ResolvedDataTask[int])(nil)
var _ TaskWithOptions = (*ResolvedDataTask[int])(nil)

func (t *ResolvedDataTask[T]) Run(ctx context.Context, step Step) error {
	return t.Task.Run(ctx, step)
}

type ResolvedDataType[T any] struct {
	Data T

	*baseResolvedDataPrivate
}

var _ ResolvedData = (*ResolvedDataType[int])(nil)

func NewResolvedDataType[T any]() *ResolvedDataType[T] {
	return &ResolvedDataType[T]{
		baseResolvedDataPrivate: NewBasedResolvedData(),
	}
}

func (d *ResolvedDataType[T]) SetResolved(data T) {
	d.baseResolvedDataPrivate.SetResolved(
		WithResolvedFunc(func() {
			d.Data = data
		}))
}
