package svcinit

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

type TaskBuildDataFunc[T any] func(ctx context.Context, data T) error

type TaskBuildDataSetupFunc[T any] func(ctx context.Context) (T, error)

func BuildDataTask[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	return newTaskBuildData[T](setupFunc, options...)
}

type TaskBuildDataOption[T any] func(*taskBuildData[T])

func WithDataDescription[T any](description string) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.description = description
	}
}

func WithDataStart[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStart, f)
}

func WithDataPreStop[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepPreStop, f)
}

func WithDataStop[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStop, f)
}

func WithDataTeardown[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepTeardown, f)
}

func WithDataTaskOptions[T any](options ...TaskInstanceOption) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.options = append(build.options, options...)
	}
}

type taskBuildData[T any] struct {
	data        *T
	setupFunc   TaskBuildDataSetupFunc[T]
	stepFunc    map[Step]TaskBuildDataFunc[T]
	steps       []Step
	options     []TaskInstanceOption
	description string
}

var _ Task = (*taskBuildData[int])(nil)
var _ TaskSteps = (*taskBuildData[int])(nil)
var _ TaskWithOptions = (*taskBuildData[int])(nil)

func newTaskBuildData[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	ret := &taskBuildData[T]{
		setupFunc: setupFunc,
		stepFunc:  make(map[Step]TaskBuildDataFunc[T]),
		steps:     []Step{StepSetup},
	}
	for _, opt := range options {
		opt(ret)
	}
	ret.init()
	if ret.isEmpty() {
		return nil
	}
	return ret
}

func (t *taskBuildData[T]) TaskSteps() []Step {
	return t.steps
}

func (t *taskBuildData[T]) TaskOptions() []TaskInstanceOption {
	return t.options
}

func (t *taskBuildData[T]) Run(ctx context.Context, step Step) error {
	switch step {
	case StepSetup:
		if t.data != nil {
			return ErrAlreadyInitialized
		}
		data, err := t.setupFunc(ctx)
		if err != nil {
			return err
		}
		t.data = &data
		return nil
	default:
		if t.data == nil {
			return ErrNotInitialized
		}
		if fn, ok := t.stepFunc[step]; ok {
			return fn(ctx, *t.data)
		}
		return newInvalidTaskStep(step)
	}
}

func (t *taskBuildData[T]) String() string {
	if t.description != "" {
		return t.description
	}
	return fmt.Sprintf("%T", t)
}

func (t *taskBuildData[T]) isEmpty() bool {
	if len(t.stepFunc) == 0 {
		return true
	}
	for _, sf := range t.stepFunc {
		if sf == nil {
			return true
		}
	}
	return false
}

func (t *taskBuildData[T]) init() {
	t.steps = slices.Concat(t.steps, slices.Collect(maps.Keys(t.stepFunc)))
}

func withDataStep[T any](step Step, f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.stepFunc[step] = f
	}
}
