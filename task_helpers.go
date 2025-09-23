package svcinit

import (
	"context"
	"fmt"
	"slices"
)

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

var allSteps = []Step{StepSetup, StepStart, StepPreStop, StepStop, StepTeardown}

func taskSteps(task Task) []Step {
	if ts, ok := task.(TaskSteps); ok {
		return ts.TaskSteps()
	}
	return allSteps
}

func taskHasStep(step Step, task Task) bool {
	return slices.Contains(taskSteps(task), step)
}

func taskDescription(task Task) string {
	if ts, ok := task.(fmt.Stringer); ok {
		return ts.String()
	}
	return fmt.Sprintf("%T", task)
}

// task options

type taskOptions struct {
	stage            string
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
