package svcinit

import (
	"context"
	"fmt"
)

type taskBuildData[T any] struct {
	tb              *taskBuild
	data            *T
	setupFunc       TaskBuildDataSetupFunc[T]
	stepFunc        map[Step]TaskBuildDataFunc[T]
	parentFromSetup bool
	tbOptions       []TaskBuildOption
}

var _ Task = (*taskBuildData[int])(nil)
var _ TaskSteps = (*taskBuildData[int])(nil)
var _ TaskWithOptions = (*taskBuildData[int])(nil)
var _ TaskWithInitError = (*taskBuildData[int])(nil)

func newTaskBuildData[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	ret := &taskBuildData[T]{
		setupFunc: setupFunc,
		stepFunc:  make(map[Step]TaskBuildDataFunc[T]),
	}
	for _, opt := range options {
		opt(ret)
	}

	if ret.setupFunc != nil {
		ret.tbOptions = append(ret.tbOptions,
			WithSetup(func(ctx context.Context) error {
				return ret.runSetup(ctx)
			}),
		)
	} else {
		ret.tbOptions = append(ret.tbOptions,
			WithSetup(nil),
		)
	}

	for step, stepFn := range ret.stepFunc {
		if stepFn != nil {
			ret.tbOptions = append(ret.tbOptions,
				WithStep(step, func(ctx context.Context) error {
					if ret.data == nil {
						return ErrNotInitialized
					}
					return stepFn(ctx, *ret.data)
				}))
		} else {
			ret.tbOptions = append(ret.tbOptions,
				WithStep(step, nil))
		}
	}

	ret.tb = newTaskBuild(ret.tbOptions...)

	return ret
}

func (t *taskBuildData[T]) TaskSteps() []Step {
	return t.tb.TaskSteps()
}

func (t *taskBuildData[T]) TaskOptions() []TaskInstanceOption {
	return t.tb.TaskOptions()
}

func (t *taskBuildData[T]) TaskInitError() error {
	return t.tb.TaskInitError()
}

func (t *taskBuildData[T]) runSetup(ctx context.Context) error {
	if t.data != nil {
		return ErrAlreadyInitialized
	}
	data, err := t.setupFunc(ctx)
	if err != nil {
		return err
	}
	t.data = &data
	if t.parentFromSetup {
		if tt, ok := any(data).(Task); ok {
			err := t.tb.setParent(tt)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: data returned from setup doesn't implement Task to be set as parent", ErrInitialization)
		}
	}
	return nil
}

func (t *taskBuildData[T]) runStep(ctx context.Context, step Step) error {
	if fn, ok := t.stepFunc[step]; ok {
		return fn(ctx, *t.data)
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuildData[T]) Run(ctx context.Context, step Step) error {
	return t.tb.Run(ctx, step)
}

func (t *taskBuildData[T]) String() string {
	return t.tb.String()
}

func withDataStep[T any](step Step, f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.stepFunc[step] = f
	}
}
