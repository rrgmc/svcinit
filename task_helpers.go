package svcinit

import (
	"context"
	"fmt"
	"iter"
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

func taskNextStep(task Task, doneSteps []Step) (Step, error) {
	return nextStep(taskSteps(task), doneSteps)
}

func nextStep(taskSteps []Step, doneSteps []Step) (Step, error) {
	nextStep := invalidStep
	for step := range taskOrderedSteps(taskSteps) {
		if nextStep == invalidStep {
			if !slices.Contains(doneSteps, step) {
				nextStep = step
			}
		} else if slices.Contains(doneSteps, step) {
			return nextStep, fmt.Errorf("%w: task next step should be '%s' but following step '%s' already done",
				ErrInvalidStepOrder, nextStep.String(), step.String())
		}
	}
	return nextStep, nil
}

func checkTaskStepOrder(task Task, doneSteps []Step, currentStep Step) error {
	next, err := taskNextStep(task, doneSteps)
	if err != nil {
		return err
	}
	if next != currentStep && currentStep != StepTeardown {
		if next == invalidStep {
			return fmt.Errorf("%w: task has no next step but trying to run '%s'",
				ErrInvalidStepOrder, currentStep)
		}
		return fmt.Errorf("%w: task next step should be '%s' but trying to run '%s'",
			ErrInvalidStepOrder, next, currentStep)
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
