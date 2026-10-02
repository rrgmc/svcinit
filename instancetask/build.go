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
func Build[T any](setupFunc BuildSetupFunc[T], options ...BuildOption[T]) *Task[T] {
	var optns buildOptions[T]
	for _, opt := range options {
		opt(&optns)
	}

	ret := &Task[T]{
		setupFunc:       setupFunc,
		parentFromSetup: optns.parentFromSetup,
	}

	tbOptions := optns.tbOptions
	if setupFunc != nil {
		tbOptions = append(tbOptions, svcinit.WithSetup(ret.runSetup))
	} else {
		tbOptions = append(tbOptions, svcinit.WithSetup(nil))
	}
	for step, stepFn := range optns.stepFunc {
		if stepFn == nil {
			tbOptions = append(tbOptions, svcinit.WithStep(step, nil))
			continue
		}
		tbOptions = append(tbOptions, svcinit.WithStep(step, func(ctx context.Context) error {
			data, err := ret.TaskData()
			if err != nil {
				return err
			}
			return stepFn(ctx, data)
		}))
	}

	ret.build = svcinit.BuildTask(tbOptions...)
	ret.buildTask = ret.build
	return ret
}

// Task is a task created by [Build].
type Task[T any] struct {
	buildTask
	build           svcinit.TaskBuild
	data            atomic.Pointer[T]
	setupFunc       BuildSetupFunc[T]
	parentFromSetup bool
}

var _ svcinit.TaskWithData[int] = (*Task[int])(nil)
var _ svcinit.TaskWithInfo = (*Task[int])(nil)

// buildTask is the part of [svcinit.TaskBuild] that [Task] exposes, by embedding.
type buildTask interface {
	svcinit.Task
	svcinit.TaskWithInfo
	fmt.Stringer
}

// TaskData returns the data returned from the "setup" step, or [svcinit.ErrNotInitialized] if it didn't run yet.
func (t *Task[T]) TaskData() (T, error) {
	data := t.data.Load()
	if data == nil {
		var empty T
		return empty, fmt.Errorf("%w: data not initialized", svcinit.ErrNotInitialized)
	}
	return *data, nil
}

func (t *Task[T]) runSetup(ctx context.Context) error {
	if t.data.Load() != nil {
		return svcinit.ErrAlreadyInitialized
	}
	data, err := t.setupFunc(ctx)
	if err != nil {
		return err
	}
	t.data.Store(&data)
	if t.parentFromSetup {
		tt, ok := any(data).(svcinit.Task)
		if !ok {
			return fmt.Errorf("%w: data returned from setup doesn't implement Task to be set as parent", svcinit.ErrInitialization)
		}
		return t.build.SetParent(tt)
	}
	return nil
}

// options

type BuildOption[T any] func(*buildOptions[T])

// WithName sets the task name.
func WithName[T any](name string) BuildOption[T] {
	return withBuildOption[T](svcinit.WithName(name))
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
// See [svcinit.WithParent].
func WithParent[T any](parent svcinit.Task) BuildOption[T] {
	return withBuildOption[T](svcinit.WithParent(parent))
}

// WithParentFromSetup sets a parent task from the result of the "setup" task.
// If this value doesn't implement Task, an initialization error will be issued.
func WithParentFromSetup[T any](parentFromSetup bool) BuildOption[T] {
	return func(o *buildOptions[T]) {
		o.parentFromSetup = parentFromSetup
	}
}

// WithTaskOptions sets default task options for [svcinit.TaskInfo.Options].
func WithTaskOptions[T any](options ...svcinit.TaskInstanceOption) BuildOption[T] {
	return withBuildOption[T](svcinit.WithTaskOptions(options...))
}

// WithSkipped adds a callback for [svcinit.TaskInfo.Skipped]. See [svcinit.WithSkipped].
func WithSkipped[T any](f func(ctx context.Context, cause error)) BuildOption[T] {
	return withBuildOption[T](svcinit.WithSkipped(f))
}

// internal

type buildOptions[T any] struct {
	stepFunc        map[svcinit.Step]BuildFunc[T]
	parentFromSetup bool
	tbOptions       []svcinit.TaskBuildOption
}

func withBuildOption[T any](option svcinit.TaskBuildOption) BuildOption[T] {
	return func(o *buildOptions[T]) {
		o.tbOptions = append(o.tbOptions, option)
	}
}

func withStep[T any](step svcinit.Step, f BuildFunc[T]) BuildOption[T] {
	return func(o *buildOptions[T]) {
		if o.stepFunc == nil {
			o.stepFunc = make(map[svcinit.Step]BuildFunc[T])
		}
		o.stepFunc[step] = f
	}
}
