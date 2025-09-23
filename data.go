package svcinit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type ResolvedData interface {
	IsResolved() bool
	DoneResolved() <-chan struct{}
}

type TaskWithResolved interface {
	Task
	ResolvedData
}

type TaskAndResolved struct {
	*baseWrappedTaskPrivate
	*DataWithResolved
}

func NewTaskAndResolved(task Task) TaskWithResolved {
	return &TaskAndResolved{
		baseWrappedTaskPrivate: NewBaseWrappedTask(task),
		DataWithResolved:       NewDataWithResolved(),
	}
}

var _ TaskWithResolved = (*TaskAndResolved)(nil)
var _ TaskSteps = (*TaskAndResolved)(nil)
var _ TaskWithOptions = (*TaskAndResolved)(nil)

func (t *TaskAndResolved) Run(ctx context.Context, step Step) error {
	err := t.Task.Run(ctx, step)
	switch step {
	case StepSetup:
		if err != nil {
			t.SetResolved(nil)
		}
	default:
	}
	return err
}

type TaskWithResolvedData[T any] struct {
	*baseOverloadedTaskPrivate
	*DataIWithResolved[T]
}

func NewTaskAndResolvedData[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) *TaskWithResolvedData[T] {
	dr := NewDataIWithResolved[T]()
	return &TaskWithResolvedData[T]{
		baseOverloadedTaskPrivate: &baseOverloadedTaskPrivate{BuildDataTask[T](func(ctx context.Context) (T, error) {
			data, err := setupFunc(ctx)
			if err != nil {
				var empty T
				return empty, err
			}
			dr.SetResolved(data)
			return data, nil
		}, options...)},
		DataIWithResolved: dr,
	}
}

var _ TaskWithResolved = (*TaskWithResolvedData[int])(nil)
var _ TaskSteps = (*TaskWithResolvedData[int])(nil)
var _ TaskWithOptions = (*TaskWithResolvedData[int])(nil)

func (t *TaskWithResolvedData[T]) Run(ctx context.Context, step Step) error {
	return t.Task.Run(ctx, step)
}

type DataWithResolved struct {
	isResolved   atomic.Bool
	resolvedChan chan struct{}
}

var _ ResolvedData = (*DataWithResolved)(nil)

func NewDataWithResolved() *DataWithResolved {
	return &DataWithResolved{
		resolvedChan: make(chan struct{}),
	}
}

func (d *DataWithResolved) IsResolved() bool {
	return d.isResolved.Load()
}

func (d *DataWithResolved) DoneResolved() <-chan struct{} {
	return d.resolvedChan
}

func (d *DataWithResolved) SetResolved(options ...DataWithResolvedSetOption) {
	var optns dataWithResolvedSetOptions
	for _, opt := range options {
		opt(&optns)
	}
	if d.isResolved.CompareAndSwap(false, true) {
		if optns.resolvedFunc != nil {
			optns.resolvedFunc()
		}
		close(d.resolvedChan)
	}
}

type DataWithResolvedSetOption func(*dataWithResolvedSetOptions)

func WithResolvedFunc(resolvedFunc func()) DataWithResolvedSetOption {
	return func(opts *dataWithResolvedSetOptions) {
		opts.resolvedFunc = resolvedFunc
	}
}

type DataIWithResolved[T any] struct {
	Data T

	*dataWithResolvedPrivate
}

var _ ResolvedData = (*DataIWithResolved[int])(nil)

func NewDataIWithResolved[T any]() *DataIWithResolved[T] {
	return &DataIWithResolved[T]{
		dataWithResolvedPrivate: NewDataWithResolved(),
	}
}

func (d *DataIWithResolved[T]) SetResolved(data T) {
	d.dataWithResolvedPrivate.SetResolved(
		WithResolvedFunc(func() {
			d.Data = data
		}))
}

func InitDataFromContext(ctx context.Context, name string) (any, error) {
	id, err := initDataFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	dt, ok := id.data[name]
	if !ok || !dt.isSet {
		return nil, fmt.Errorf("%w: '%s' not set", ErrInitData, name)
	}
	return dt.value, nil
}

func InitDataTypeFromContext[T any](ctx context.Context, name string) (T, error) {
	id, err := InitDataFromContext(ctx, name)
	if err == nil {
		if v, ok := id.(T); ok {
			return v, nil
		} else {
			err = fmt.Errorf("%w: unexpected type %T", ErrInitData, id)
		}
	}
	var empty T
	return empty, err
}

func InitDataSet[T any](ctx context.Context, name string, data T) error {
	id, err := initDataFromContext(ctx)
	if err != nil {
		return err
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	dt, ok := id.data[name]
	if ok {
		if dt.isSet {
			return fmt.Errorf("%w: '%s' already set", ErrInitData, name)
		}
	}
	id.data[name] = &initDataItem{
		isSet: true,
		value: data,
	}
	return nil
}

func InitDataClear(ctx context.Context, name string) error {
	id, err := initDataFromContext(ctx)
	if err != nil {
		return err
	}
	id.mu.Lock()
	defer id.mu.Unlock()
	if _, ok := id.data[name]; ok {
		delete(id.data, name)
	}
	return nil
}

func initDataFromContext(ctx context.Context) (*initData, error) {
	if val := ctx.Value(initDataKey{}); val != nil {
		if id, ok := val.(*initData); ok {
			return id, nil
		}
	}
	return nil, fmt.Errorf("%w: %w", ErrInitData, ErrNotInitialized)
}

type dataWithResolvedSetOptions struct {
	resolvedFunc func()
}

type initDataKey struct{}

type initData struct {
	mu   sync.Mutex
	data map[string]*initDataItem
}

type initDataItem struct {
	isSet bool
	value any
}

func contextWithInitData(ctx context.Context) context.Context {
	id := initData{
		data: make(map[string]*initDataItem),
	}
	return context.WithValue(ctx, initDataKey{}, &id)
}

type dataWithResolvedPrivate = DataWithResolved
