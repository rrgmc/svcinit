package svcinit

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

type TaskBuildDataFunc func(ctx context.Context) error

func BuildDataTask(options ...TaskBuildDataOption) Task {
	return newTaskBuildData(options...)
}

type TaskBuildDataOption func(*taskBuildData)

func WithDataDescription(description string) TaskBuildDataOption {
	return func(build *taskBuildData) {
		build.description = description
	}
}

func WithDataStep(step Step, f TaskBuildDataFunc) TaskBuildDataOption {
	return func(build *taskBuildData) {
		build.stepFunc[step] = f
	}
}

func WithDataSetup(f TaskBuildDataFunc) TaskBuildDataOption {
	return WithDataStep(StepSetup, f)
}

func WithDataStart(f TaskBuildDataFunc) TaskBuildDataOption {
	return WithDataStep(StepStart, f)
}

func WithDataPreStop(f TaskBuildDataFunc) TaskBuildDataOption {
	return WithDataStep(StepPreStop, f)
}

func WithDataStop(f TaskBuildDataFunc) TaskBuildDataOption {
	return WithDataStep(StepStop, f)
}

func WithDataTeardown(f TaskBuildDataFunc) TaskBuildDataOption {
	return WithDataStep(StepTeardown, f)
}

func WithDataTaskOptions(options ...TaskInstanceOption) TaskBuildDataOption {
	return func(build *taskBuildData) {
		build.options = append(build.options, options...)
	}
}

type taskBuildData struct {
	stepFunc    map[Step]TaskBuildDataFunc
	steps       []Step
	options     []TaskInstanceOption
	description string
}

var _ Task = (*taskBuildData)(nil)
var _ TaskSteps = (*taskBuildData)(nil)
var _ TaskWithOptions = (*taskBuildData)(nil)

func newTaskBuildData(options ...TaskBuildDataOption) Task {
	ret := &taskBuildData{
		stepFunc: make(map[Step]TaskBuildDataFunc),
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

func (t *taskBuildData) TaskSteps() []Step {
	return t.steps
}

func (t *taskBuildData) TaskOptions() []TaskInstanceOption {
	return t.options
}

func (t *taskBuildData) Run(ctx context.Context, step Step) error {
	if fn, ok := t.stepFunc[step]; ok {
		return fn(ctx)
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuildData) String() string {
	if t.description != "" {
		return t.description
	}
	return fmt.Sprintf("%T", t)
}

func (t *taskBuildData) isEmpty() bool {
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

func (t *taskBuildData) init() {
	t.steps = slices.Collect(maps.Keys(t.stepFunc))
}
