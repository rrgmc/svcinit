package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync/atomic"
)

type TaskBuild interface {
	Task
	TaskWithInfo
	// SetParent sets the parent task, returning the new initialization error, which is also returned from
	// [TaskInfo.InitError].
	SetParent(parent Task) error
	String() string
}

type TaskBuildFunc func(ctx context.Context) error

// BuildTask creates a task from callback functions.
//
// It is also the way to decorate an existing task: use [WithParent] to forward all steps not set here to it,
// and [WithName], [WithTaskOptions] and [WithNotRun] to add to its [TaskInfo]. To customize how the steps are
// called, use [WithHandler] in [Manager.AddTask].
func BuildTask(options ...TaskBuildOption) TaskBuild {
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
// Its [TaskInfo] is merged into the built task one: its name is used if one is not set, its options are applied
// before the built task ones, its initialization error is returned, and its [TaskInfo.NotRun] is called.
func WithParent(parent Task) TaskBuildOption {
	return func(build *taskBuild) {
		if parent == nil {
			build.parent.Store(nil)
		} else {
			build.parent.Store(&parent)
		}
	}
}

// WithTaskOptions sets default task options for [TaskInfo.Options].
func WithTaskOptions(options ...TaskInstanceOption) TaskBuildOption {
	return func(build *taskBuild) {
		build.options = append(build.options, options...)
	}
}

// WithNotRun adds a callback for [TaskInfo.NotRun]. All callbacks are called in order, before the parent one.
func WithNotRun(f func(ctx context.Context, cause error)) TaskBuildOption {
	return func(build *taskBuild) {
		if f != nil {
			build.notRun = append(build.notRun, f)
		}
	}
}

// internal

type taskBuild struct {
	stepFunc map[Step]TaskBuildFunc
	parent   atomic.Pointer[Task]
	state    atomic.Pointer[taskBuildState]
	options  []TaskInstanceOption
	notRun   []func(ctx context.Context, cause error)
	name     string
}

// taskBuildState is the state computed from the step callbacks and the parent.
type taskBuildState struct {
	steps     []Step
	initError error
}

var _ TaskBuild = (*taskBuild)(nil)

func newTaskBuild(options ...TaskBuildOption) *taskBuild {
	ret := &taskBuild{
		stepFunc: make(map[Step]TaskBuildFunc),
	}
	for _, opt := range options {
		opt(ret)
	}
	ret.init()
	return ret
}

func (t *taskBuild) TaskInfo() TaskInfo {
	var parentInfo TaskInfo
	if parent := t.loadParent(); parent != nil {
		parentInfo = GetTaskInfo(parent)
	}
	state := t.state.Load()
	ret := TaskInfo{
		Name:      cmp.Or(t.name, parentInfo.Name),
		Steps:     slices.Clone(state.steps),
		Options:   slices.Concat(parentInfo.Options, t.options),
		InitError: state.initError,
	}
	if len(t.notRun) > 0 || parentInfo.NotRun != nil {
		ret.NotRun = func(ctx context.Context, cause error) {
			for _, f := range t.notRun {
				f(ctx, cause)
			}
			if parentInfo.NotRun != nil {
				parentInfo.NotRun(ctx, cause)
			}
		}
	}
	return ret
}

func (t *taskBuild) Run(ctx context.Context, step Step) error {
	var parent Task
	if p := t.loadParent(); p != nil && taskHasStep(p, step) {
		parent = p
	}

	if fn, ok := t.stepFunc[step]; ok {
		if parent != nil {
			return fmt.Errorf("%w: build task parent already has '%s' step", ErrDuplicateStep, step.String())
		}
		return fn(ctx)
	}
	if parent != nil {
		return parent.Run(ctx, step)
	}
	return newInvalidTaskStep(step)
}

func (t *taskBuild) String() string {
	if tn := GetTaskName(t); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t)
}

func (t *taskBuild) SetParent(parent Task) error {
	if parent == nil {
		t.parent.Store(nil)
	} else {
		t.parent.Store(&parent)
	}
	return t.init()
}

func (t *taskBuild) loadParent() Task {
	if p := t.parent.Load(); p != nil {
		return *p
	}
	return nil
}

// hasMissingStep returns whether any step callback is nil, or if there are no steps at all (neither callbacks
// nor a parent to forward to).
func (t *taskBuild) hasMissingStep() bool {
	if len(t.stepFunc) == 0 {
		return t.loadParent() == nil
	}
	for _, sf := range t.stepFunc {
		if sf == nil {
			return true
		}
	}
	return false
}

// init computes the task steps and initialization error, and returns the error.
func (t *taskBuild) init() error {
	var errs []error
	if t.hasMissingStep() {
		errs = append(errs, ErrNilTask)
	}

	// never nil, as it would mean "all steps".
	steps := slices.AppendSeq(make([]Step, 0, len(allSteps)), maps.Keys(t.stepFunc))

	if parent := t.loadParent(); parent != nil {
		var duplicatedSteps []Step
		for _, step := range taskSteps(parent) {
			if !slices.Contains(steps, step) {
				steps = append(steps, step)
			} else {
				duplicatedSteps = append(duplicatedSteps, step)
			}
		}
		if len(duplicatedSteps) > 0 {
			errs = append(errs, fmt.Errorf("%w: build task parent already has '%s' step(s)", ErrDuplicateStep,
				stringerString(duplicatedSteps)))
		}
		if err := GetTaskInfo(parent).InitError; err != nil {
			errs = append(errs, err)
		}
	}

	state := &taskBuildState{
		steps:     steps,
		initError: buildJoinedErrors(errs),
	}
	t.state.Store(state)
	return state.initError
}

// buildJoinedErrors returns nil for no errors, the error itself if only one, or [errors.Join] of all of them.
func buildJoinedErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}
