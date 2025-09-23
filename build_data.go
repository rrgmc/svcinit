package svcinit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
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

func WithDataParent[T any](parent Task) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.parent = parent
	}
}

func WithDataParentFromSetup[T any](parentFromSetup bool) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.parentFromSetup = parentFromSetup
	}
}

// func WithInitDataSet[T any](name string) TaskBuildDataOption[T] {
// 	return func(build *taskBuildData[T]) {
// 		build.initDataSet = name
// 	}
// }

func WithDataTaskOptions[T any](options ...TaskInstanceOption) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.options = append(build.options, options...)
	}
}

type taskBuildData[T any] struct {
	data            *T
	setupFunc       TaskBuildDataSetupFunc[T]
	stepFunc        map[Step]TaskBuildDataFunc[T]
	parent          Task
	parentFromSetup bool
	steps           []Step
	options         []TaskInstanceOption
	initError       error
	description     string
	// initDataSet     string
}

var _ Task = (*taskBuildData[int])(nil)
var _ TaskSteps = (*taskBuildData[int])(nil)
var _ TaskWithOptions = (*taskBuildData[int])(nil)
var _ TaskWithInitError = (*taskBuildData[int])(nil)

func newTaskBuildData[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	ret := &taskBuildData[T]{
		setupFunc: setupFunc,
		stepFunc:  make(map[Step]TaskBuildDataFunc[T]),
		steps:     []Step{StepSetup},
	}
	for _, opt := range options {
		opt(ret)
	}
	err := ret.init()
	if err != nil {
		ret.initError = err
	}
	return ret
}

func (t *taskBuildData[T]) TaskSteps() []Step {
	return t.steps
}

func (t *taskBuildData[T]) TaskOptions() []TaskInstanceOption {
	return t.options
}

func (t *taskBuildData[T]) TaskInitError() error {
	return t.initError
}

func (t *taskBuildData[T]) Run(ctx context.Context, step Step) error {
	var parentHasStep bool
	if t.parent != nil {
		parentHasStep = taskHasStep(step, t.parent)
	}

	switch step {
	case StepSetup:
		if t.data != nil {
			return ErrAlreadyInitialized
		}
		if parentHasStep {
			return fmt.Errorf("%w: build data task parent already has 'setup' step", ErrDuplicateStep)
		}
		data, err := t.setupFunc(ctx)
		if err != nil {
			return err
		}
		t.data = &data
		// if t.initDataSet != "" {
		// 	return InitDataSet(ctx, t.initDataSet, data)
		// }
		if t.parentFromSetup {
			if tt, ok := any(data).(Task); ok {
				t.parent = tt
				return t.init()
			} else {
				return fmt.Errorf("%w: setup does not implement Task", ErrInitialization)
			}
		}
		return nil
	default:
		if t.data == nil {
			return ErrNotInitialized
		}
		if fn, ok := t.stepFunc[step]; ok {
			if parentHasStep {
				return fmt.Errorf("%w: build data task parent already has '%s' step", ErrDuplicateStep, step.String())
			}
			return fn(ctx, *t.data)
		}
		if parentHasStep {
			return t.parent.Run(ctx, step)
		}
		return newInvalidTaskStep(step)
	}
}

func (t *taskBuildData[T]) String() string {
	if t.description != "" {
		return t.description
	}
	if t.parent != nil {
		return taskDescription(t.parent)
	}
	return fmt.Sprintf("%T", t)
}

func (t *taskBuildData[T]) init() error {
	var duplicatedSteps []Step

	t.steps = slices.Concat([]Step{StepSetup}, slices.Collect(maps.Keys(t.stepFunc)))

	if t.parent != nil {
		for _, step := range taskSteps(t.parent) {
			if !slices.Contains(t.steps, step) {
				t.steps = append(t.steps, step)
			} else {
				duplicatedSteps = append(duplicatedSteps, step)
			}
		}
	}

	if len(duplicatedSteps) > 0 {
		return fmt.Errorf("%w: build data task parent already has '%s' step(s)", ErrDuplicateStep,
			strings.Join(sliceMap(duplicatedSteps, func(i int, e Step) string {
				return e.String()
			}), ","))
	}

	return nil
}

func withDataStep[T any](step Step, f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.stepFunc[step] = f
	}
}
