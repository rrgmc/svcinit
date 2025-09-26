package svcinit

import (
	"context"
	"fmt"
	"sync/atomic"
)

type TaskBuildDataFunc[T any] func(ctx context.Context, data T) error

type TaskBuildDataSetupFunc[T any] func(ctx context.Context) (T, error)

// BuildDataTask creates a task from callback functions, where some data is created in the "setup" step and passed
// to all other steps.
func BuildDataTask[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	return newTaskBuildData[T](setupFunc, options...)
}

type TaskBuildDataOption[T any] func(*taskBuildData[T])

// WithDataName sets the task name.
func WithDataName[T any](name string) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.tbOptions = append(build.tbOptions, WithName(name))
	}
}

// WithDataStart sets a callback for the "start" step.
func WithDataStart[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStart, f)
}

// WithDataStop sets a callback for the "stop" step.
func WithDataStop[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStop, f)
}

// WithDataTeardown sets a callback for the "teardown" step.
func WithDataTeardown[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepTeardown, f)
}

// WithDataParent sets a parent task. Any step not set in the built task will be forwarded to it.
func WithDataParent[T any](parent Task) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.tbOptions = append(build.tbOptions, WithParent(parent))
	}
}

// WithDataParentFromSetup sets a parent task from the result of the "setup" task.
// If this value doesn't implement Task, an initialization error will be issued.
func WithDataParentFromSetup[T any](parentFromSetup bool) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.parentFromSetup = parentFromSetup
	}
}

// WithDataTaskOptions sets default task options for the TaskOption interface.
func WithDataTaskOptions[T any](options ...TaskInstanceOption) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.tbOptions = append(build.tbOptions, WithTaskOptions(options...))
	}
}

// internal

type taskBuildData[T any] struct {
	tb              *taskBuild
	data            atomic.Pointer[T]
	setupFunc       TaskBuildDataSetupFunc[T]
	stepFunc        map[Step]TaskBuildDataFunc[T]
	parentFromSetup bool
	tbOptions       []TaskBuildOption
}

var _ Task = (*taskBuildData[int])(nil)
var _ TaskName = (*taskBuildData[int])(nil)
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
					if data := ret.data.Load(); data == nil {
						return fmt.Errorf("%w: data not initialized", ErrNotInitialized)
					}
					return stepFn(ctx, *ret.data.Load())
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
	if t.data.Load() != nil {
		return ErrAlreadyInitialized
	}
	data, err := t.setupFunc(ctx)
	if err != nil {
		return err
	}
	t.data.Store(&data)
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
		return fn(ctx, *t.data.Load())
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuildData[T]) Run(ctx context.Context, step Step) error {
	return t.tb.Run(ctx, step)
}

func (t *taskBuildData[T]) TaskName() string {
	return t.tb.TaskName()
}

func (t *taskBuildData[T]) String() string {
	return t.tb.String()
}

func withDataStep[T any](step Step, f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.stepFunc[step] = f
	}
}
