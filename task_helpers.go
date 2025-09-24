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

var allSteps = []Step{StepSetup, StepStart, StepPreStop, StepStop, StepTeardown} // order matters

func taskSteps(task Task) []Step {
	if ts, ok := task.(TaskSteps); ok {
		return ts.TaskSteps()
	}
	return allSteps
}

func taskHasStep(step Step, task Task) bool {
	return slices.Contains(taskSteps(task), step)
}

func checkTaskStepOrder(steps []Step, currentStep Step) error {
	if len(steps) == 0 {
		return nil
	}
	lastStepIdx := -1
	var lastStep Step
	currentStepIdx := -1
	for stepI, step := range allSteps {
		hasStep := slices.Contains(steps, step)
		if hasStep && stepI > lastStepIdx {
			lastStepIdx = stepI
			lastStep = step
		}
		if currentStep == step {
			if hasStep {
				return fmt.Errorf("%w: step '%s' already executed", ErrInvalidStepOrder, currentStep.String())
			}
			currentStepIdx = stepI
		}
	}
	if lastStepIdx == -1 || currentStepIdx == -1 {
		return fmt.Errorf("%w: unknown step(s), cannot check task execute order (%d -- %d)", ErrInvalidStepOrder,
			lastStepIdx, currentStepIdx)
	}
	if currentStepIdx <= lastStepIdx {
		return fmt.Errorf("%w: cannot execute step '%s' after '%s'", ErrInvalidStepOrder,
			currentStep.String(), lastStep.String())
	}
	return nil
}

func taskDescription(task Task) string {
	if ts, ok := task.(fmt.Stringer); ok {
		return ts.String()
	}
	return fmt.Sprintf("%T", task)
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
