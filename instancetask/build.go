package instancetask

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rrgmc/svcinit/v3"
)

type BuildFunc[T any] func(ctx context.Context, data T) error

type BuildSetupFunc[T any] func(ctx context.Context) (T, error)

// Build creates a task from callback functions, where some data is created in the "setup" step and passed
// to all other steps.
func Build[T any](setupFunc BuildSetupFunc[T], options ...BuildOption[T]) svcinit.TaskWithData[T] {
	return newTaskBuild[T](setupFunc, options...)
}

type BuildOption[T any] func(*taskBuild[T])

// WithName sets the task name.
func WithName[T any](name string) BuildOption[T] {
	return func(build *taskBuild[T]) {
		build.tbOptions = append(build.tbOptions, svcinit.WithName(name))
	}
}

// WithStart sets a callback for the "start" step.
func WithStart[T any](f BuildFunc[T]) BuildOption[T] {
	return withStep(svcinit.StepStart, f)
}

// WithStop sets a callback for the "stop" step.
func WithStop[T any](f BuildFunc[T]) BuildOption[T] {
	return withStep(svcinit.StepStop, f)
}

// WithTeardown sets a callback for the "teardown" step.
func WithTeardown[T any](f BuildFunc[T]) BuildOption[T] {
	return withStep(svcinit.StepTeardown, f)
}

// WithParent sets a parent task. Any step not set in the built task will be forwarded to it.
func WithParent[T any](parent svcinit.Task) BuildOption[T] {
	return func(build *taskBuild[T]) {
		build.tbOptions = append(build.tbOptions, svcinit.WithParent(parent))
	}
}

// WithParentFromSetup sets a parent task from the result of the "setup" task.
// If this value doesn't implement Task, an initialization error will be issued.
func WithParentFromSetup[T any](parentFromSetup bool) BuildOption[T] {
	return func(build *taskBuild[T]) {
		build.parentFromSetup = parentFromSetup
	}
}

// WithTaskOptions sets default task options for the TaskOption interface.
func WithTaskOptions[T any](options ...svcinit.TaskInstanceOption) BuildOption[T] {
	return func(build *taskBuild[T]) {
		build.tbOptions = append(build.tbOptions, svcinit.WithTaskOptions(options...))
	}
}

// internal

type taskBuild[T any] struct {
	tb              svcinit.TaskBuild
	data            atomic.Pointer[T]
	setupFunc       BuildSetupFunc[T]
	stepFunc        map[svcinit.Step]BuildFunc[T]
	parentFromSetup bool
	tbOptions       []svcinit.TaskBuildOption
}

var _ svcinit.TaskWithData[int] = (*taskBuild[int])(nil)
var _ svcinit.TaskName = (*taskBuild[int])(nil)
var _ svcinit.TaskSteps = (*taskBuild[int])(nil)
var _ svcinit.TaskWithOptions = (*taskBuild[int])(nil)
var _ svcinit.TaskWithInitError = (*taskBuild[int])(nil)

func newTaskBuild[T any](setupFunc BuildSetupFunc[T], options ...BuildOption[T]) svcinit.TaskWithData[T] {
	ret := &taskBuild[T]{
		setupFunc: setupFunc,
		stepFunc:  make(map[svcinit.Step]BuildFunc[T]),
	}
	for _, opt := range options {
		opt(ret)
	}

	if ret.setupFunc != nil {
		ret.tbOptions = append(ret.tbOptions,
			svcinit.WithSetup(func(ctx context.Context) error {
				return ret.runSetup(ctx)
			}),
		)
	} else {
		ret.tbOptions = append(ret.tbOptions,
			svcinit.WithSetup(nil),
		)
	}

	for step, stepFn := range ret.stepFunc {
		if stepFn != nil {
			ret.tbOptions = append(ret.tbOptions,
				svcinit.WithStep(step, func(ctx context.Context) error {
					if data := ret.data.Load(); data == nil {
						return fmt.Errorf("%w: data not initialized", svcinit.ErrNotInitialized)
					}
					return stepFn(ctx, *ret.data.Load())
				}))
		} else {
			ret.tbOptions = append(ret.tbOptions,
				svcinit.WithStep(step, nil))
		}
	}

	ret.tb = svcinit.BuildTask(ret.tbOptions...)

	return ret
}

func (t *taskBuild[T]) TaskData() (T, error) {
	if data := t.data.Load(); data == nil {
		var empty T
		return empty, fmt.Errorf("%w: data not initialized", svcinit.ErrNotInitialized)
	} else {
		return *data, nil
	}
}

func (t *taskBuild[T]) TaskSteps() []svcinit.Step {
	return t.tb.TaskSteps()
}

func (t *taskBuild[T]) TaskOptions() []svcinit.TaskInstanceOption {
	return t.tb.TaskOptions()
}

func (t *taskBuild[T]) TaskInitError() error {
	return t.tb.TaskInitError()
}

func (t *taskBuild[T]) runSetup(ctx context.Context) error {
	if t.data.Load() != nil {
		return svcinit.ErrAlreadyInitialized
	}
	data, err := t.setupFunc(ctx)
	if err != nil {
		return err
	}
	t.data.Store(&data)
	if t.parentFromSetup {
		if tt, ok := any(data).(svcinit.Task); ok {
			err := t.tb.SetParent(tt)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: data returned from setup doesn't implement Task to be set as parent", svcinit.ErrInitialization)
		}
	}
	return nil
}

func (t *taskBuild[T]) runStep(ctx context.Context, step svcinit.Step) error {
	if fn, ok := t.stepFunc[step]; ok {
		return fn(ctx, *t.data.Load())
	}
	return fmt.Errorf("%w: %s", svcinit.ErrInvalidTaskStep, step)
}

func (t *taskBuild[T]) Run(ctx context.Context, step svcinit.Step) error {
	return t.tb.Run(ctx, step)
}

func (t *taskBuild[T]) TaskName() string {
	return t.tb.TaskName()
}

func (t *taskBuild[T]) String() string {
	return t.tb.String()
}

func withStep[T any](step svcinit.Step, f BuildFunc[T]) BuildOption[T] {
	return func(build *taskBuild[T]) {
		build.stepFunc[step] = f
	}
}
