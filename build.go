package svcinit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
)

type TaskBuildFunc func(ctx context.Context) error

// BuildTask creates a task from callback functions.
func BuildTask(options ...TaskBuildOption) Task {
	return newTaskBuild(options...)
}

type TaskBuildOption func(*taskBuild)

// WithName sets the task name.
func WithName(name string) TaskBuildOption {
	return func(build *taskBuild) {
		build.name = name
	}
}

// WithStep sets the callback for a step.
func WithStep(step Step, f TaskBuildFunc) TaskBuildOption {
	return func(build *taskBuild) {
		build.stepFunc[step] = f
	}
}

// WithSetup sets a callback for the "setup" step.
func WithSetup(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepSetup, f)
}

// WithStart sets a callback for the "start" step.
func WithStart(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStart, f)
}

// WithStop sets a callback for the "stop" step.
func WithStop(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStop, f)
}

// WithTeardown sets a callback for the "teardown" step.
func WithTeardown(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepTeardown, f)
}

// WithParent sets a parent task. Any step not set in the built task will be forwarded to it.
func WithParent(parent Task) TaskBuildOption {
	return func(build *taskBuild) {
		build.parent.Store(&parent)
	}
}

// WithTaskOptions sets default task options for the TaskOption interface.
func WithTaskOptions(options ...TaskInstanceOption) TaskBuildOption {
	return func(build *taskBuild) {
		build.options = append(build.options, options...)
	}
}

// internal

type taskBuild struct {
	stepFunc  map[Step]TaskBuildFunc
	parent    atomic.Pointer[Task]
	steps     []Step
	options   []TaskInstanceOption
	initError error
	name      string
}

var _ Task = (*taskBuild)(nil)
var _ TaskName = (*taskBuild)(nil)
var _ TaskSteps = (*taskBuild)(nil)
var _ TaskWithOptions = (*taskBuild)(nil)
var _ TaskWithInitError = (*taskBuild)(nil)

func newTaskBuild(options ...TaskBuildOption) *taskBuild {
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
		ret.initError = errors.Join(ret.initError, ErrNilTask)
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
	if parent := t.parent.Load(); parent != nil {
		parentHasStep = taskHasStep(*parent, step)
	}

	if fn, ok := t.stepFunc[step]; ok {
		if parentHasStep {
			return fmt.Errorf("%w: build task parent already has '%s' step", ErrDuplicateStep, step.String())
		}
		return fn(ctx)
	}
	if parentHasStep {
		if parent := t.parent.Load(); parent != nil {
			return (*parent).Run(ctx, step)
		}

	}
	return newInvalidTaskStep(step)
}

func (t *taskBuild) TaskName() string {
	if t.name != "" {
		return t.name
	}
	if parent := t.parent.Load(); parent != nil {
		return GetTaskName(*parent)
	}
	return ""
}

func (t *taskBuild) String() string {
	if tn := t.TaskName(); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t)
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

func (t *taskBuild) setParent(parent Task) error {
	if parent == nil {
		t.parent.Store(nil)
	} else {
		t.parent.Store(&parent)
	}
	return t.init()
}

func (t *taskBuild) init() error {
	var duplicatedSteps []Step

	t.steps = slices.Collect(maps.Keys(t.stepFunc))

	if parent := t.parent.Load(); parent != nil {
		for _, step := range taskSteps(*parent) {
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
