package svcinit

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

type TaskBuildFunc func(ctx context.Context) error

func BuildTask(options ...TaskBuildOption) Task {
	return newTaskBuild(options...)
}

type TaskBuildOption func(*taskBuild)

func WithDescription(descxription string) TaskBuildOption {
	return func(build *taskBuild) {
		build.description = descxription
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

func WithTaskOptions(options ...TaskInstanceOption) TaskBuildOption {
	return func(build *taskBuild) {
		build.options = append(build.options, options...)
	}
}

type taskBuild struct {
	stepFunc    map[Step]TaskBuildFunc
	steps       []Step
	options     []TaskInstanceOption
	description string
}

var _ Task = (*taskBuild)(nil)
var _ TaskSteps = (*taskBuild)(nil)
var _ TaskWithOptions = (*taskBuild)(nil)

func newTaskBuild(options ...TaskBuildOption) Task {
	ret := &taskBuild{
		stepFunc: make(map[Step]TaskBuildFunc),
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

func (t *taskBuild) TaskSteps() []Step {
	return t.steps
}

func (t *taskBuild) TaskOptions() []TaskInstanceOption {
	return t.options
}

func (t *taskBuild) Run(ctx context.Context, step Step) error {
	if fn, ok := t.stepFunc[step]; ok {
		return fn(ctx)
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuild) String() string {
	if t.description != "" {
		return t.description
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

func (t *taskBuild) init() {
	t.steps = slices.Collect(maps.Keys(t.stepFunc))
}
