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

// func InitDataFromContext(ctx context.Context, name string) (any, error) {
// 	id, err := initDataFromContext(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	id.mu.Lock()
// 	defer id.mu.Unlock()
// 	dt, ok := id.data[name]
// 	if !ok || !dt.isSet {
// 		return nil, fmt.Errorf("%w: '%s' not set", ErrInitData, name)
// 	}
// 	return dt.value, nil
// }
//
// func InitDataTypeFromContext[T any](ctx context.Context, name string) (T, error) {
// 	id, err := InitDataFromContext(ctx, name)
// 	if err == nil {
// 		if v, ok := id.(T); ok {
// 			return v, nil
// 		} else {
// 			err = fmt.Errorf("%w: unexpected type %T", ErrInitData, id)
// 		}
// 	}
// 	var empty T
// 	return empty, err
// }
//
// func InitDataSet[T any](ctx context.Context, name string, data T) error {
// 	id, err := initDataFromContext(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	id.mu.Lock()
// 	defer id.mu.Unlock()
// 	dt, ok := id.data[name]
// 	if ok {
// 		if dt.isSet {
// 			return fmt.Errorf("%w: '%s' already set", ErrInitData, name)
// 		}
// 	}
// 	id.data[name] = &initDataItem{
// 		isSet: true,
// 		value: data,
// 	}
// 	return nil
// }
//
// func InitDataClear(ctx context.Context, name string) error {
// 	id, err := initDataFromContext(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	id.mu.Lock()
// 	defer id.mu.Unlock()
// 	if _, ok := id.data[name]; ok {
// 		delete(id.data, name)
// 	}
// 	return nil
// }
//
// func initDataFromContext(ctx context.Context) (*initData, error) {
// 	if val := ctx.Value(initDataKey{}); val != nil {
// 		if id, ok := val.(*initData); ok {
// 			return id, nil
// 		}
// 	}
// 	return nil, fmt.Errorf("%w: %w", ErrInitData, ErrNotInitialized)
// }
//
// type initDataKey struct{}
//
// type initData struct {
// 	mu   sync.Mutex
// 	data map[string]*initDataItem
// }
//
// type initDataItem struct {
// 	isSet bool
// 	value any
// }
//
// func contextWithInitData(ctx context.Context) context.Context {
// 	id := initData{
// 		data: make(map[string]*initDataItem),
// 	}
// 	return context.WithValue(ctx, initDataKey{}, &id)
// }
