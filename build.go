package svcinit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type TaskBuildFunc func(ctx context.Context) error

func BuildTask(options ...TaskBuildOption) Task {
	return newTaskBuild(options...)
}

type TaskBuildOption func(*taskBuild)

func WithParent(parent Task) TaskBuildOption {
	return func(build *taskBuild) {
		build.parent = parent
	}
}

func WithDescription(description string) TaskBuildOption {
	return func(build *taskBuild) {
		build.description = description
	}
}

func WithStep(step Step, f TaskBuildFunc) TaskBuildOption {
	return func(build *taskBuild) {
		build.stepFunc[step] = f
	}
}

func WithSetup(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepSetup, f)
}

func WithStart(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStart, f)
}

func WithPreStop(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepPreStop, f)
}

func WithStop(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStop, f)
}

func WithTeardown(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepTeardown, f)
}

func WithTaskOptions(options ...TaskInstanceOption) TaskBuildOption {
	return func(build *taskBuild) {
		build.options = append(build.options, options...)
	}
}

type taskBuild struct {
	stepFunc    map[Step]TaskBuildFunc
	parent      Task
	steps       []Step
	options     []TaskInstanceOption
	initError   error
	description string
}

var _ Task = (*taskBuild)(nil)
var _ TaskSteps = (*taskBuild)(nil)
var _ TaskWithOptions = (*taskBuild)(nil)
var _ TaskWithInitError = (*taskBuild)(nil)

func newTaskBuild(options ...TaskBuildOption) Task {
	ret := &taskBuild{
		stepFunc: make(map[Step]TaskBuildFunc),
	}
	for _, opt := range options {
		opt(ret)
	}
	err := ret.init()
	if err != nil {
		ret.initError = err
	}
	if ret.isEmpty() {
		return nil
	}
	return ret
}

func (t *taskBuild) TaskSteps() []Step {
	return t.steps
}

func (t *taskBuild) TaskOptions() []TaskInstanceOption {
	return t.options
}

func (t *taskBuild) TaskInitError() error {
	return t.initError
}

func (t *taskBuild) Run(ctx context.Context, step Step) error {
	var parentHasStep bool
	if t.parent != nil {
		parentHasStep = taskHasStep(t.parent, step)
	}

	if fn, ok := t.stepFunc[step]; ok {
		if parentHasStep {
			return fmt.Errorf("%w: build task parent already has '%s' step", ErrDuplicateStep, step.String())
		}
		return fn(ctx)
	}
	if parentHasStep {
		return t.parent.Run(ctx, step)
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuild) String() string {
	if t.description != "" {
		return t.description
	}
	if t.parent != nil {
		return taskDescription(t.parent)
	}
	return fmt.Sprintf("%T", t)
}

func (t *taskBuild) isEmpty() bool {
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

func (t *taskBuild) init() error {
	var duplicatedSteps []Step

	t.steps = slices.Collect(maps.Keys(t.stepFunc))

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
		return fmt.Errorf("%w: build task parent already has '%s' step(s)", ErrDuplicateStep,
			strings.Join(sliceMap(duplicatedSteps, func(i int, e Step) string {
				return e.String()
			}), ","))
	}

	return nil
}
