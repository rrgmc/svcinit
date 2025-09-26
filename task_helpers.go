package svcinit

import (
	"context"
	"iter"
	"slices"
)

var allSteps = []Step{StepSetup, StepStart, StepStop, StepTeardown} // order matters

func taskSteps(task Task) []Step {
	if ts, ok := task.(TaskSteps); ok {
		return ts.TaskSteps()
	}
	return allSteps
}

func taskOrderedSteps(steps []Step) iter.Seq[Step] {
	return func(yield func(Step) bool) {
		if len(steps) == 0 {
			return
		}
		for _, step := range allSteps {
			if slices.Contains(steps, step) {
				if !yield(step) {
					return
				}
			}
		}
	}
}

func taskHasStep(task Task, step Step) bool {
	return slices.Contains(taskSteps(task), step)
}

type errorTask struct {
	err error
}

var _ Task = (*errorTask)(nil)

func (n *errorTask) Run(_ context.Context, step Step) error {
	return n.err
}

// checkNilTask returns errorTask if task is nil, otherwise return the task itself.
func checkNilTask(task Task) Task {
	if task == nil {
		return &errorTask{err: ErrNilTask}
	}
	return task
}

// task options

type taskOptions struct {
	cancelContext    bool
	startStepManager bool
	callbacks        []TaskCallback
	handler          TaskHandler
}

type taskOptionFunc func(*taskOptions)

func (f taskOptionFunc) applyTaskOpt(opts *taskOptions) {
	f(opts)
}

type taskInstanceOptionFunc func(*taskOptions)

func (f taskInstanceOptionFunc) applyTaskInstanceOpt(opts *taskOptions) {
	f(opts)
}

type taskGlobalOptionFunc func(*taskOptions)

func (f taskGlobalOptionFunc) applyTaskOpt(options *taskOptions) {
	f(options)
}

func (f taskGlobalOptionFunc) applyTaskInstanceOpt(opts *taskOptions) {
	f(opts)
}
