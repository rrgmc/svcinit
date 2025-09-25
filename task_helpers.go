package svcinit

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
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

func taskNextStep(task Task, doneSteps []Step) (Step, error) {
	return nextStep(taskSteps(task), doneSteps)
}

func nextStep(taskSteps []Step, doneSteps []Step) (Step, error) {
	nextStep := StepInvalid
	for step := range taskOrderedSteps(taskSteps) {
		if nextStep == StepInvalid {
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

func checkTaskStepOrder(ctx context.Context, logger *slog.Logger, task Task, doneSteps []Step, currentStep Step) error {
	tSteps := taskSteps(task)

	next, err := nextStep(tSteps, doneSteps)
	if err != nil {
		// if logger.Enabled(ctx, slog2.LevelTrace) {
		// 	logger.Log(ctx, slog2.LevelTrace, "task next step error",
		// 		"taskSteps", stringerIter(taskOrderedSteps(tSteps)),
		// 		"doneSteps", stringerList(doneSteps),
		// 		slog2.ErrorKey, err)
		// }
		return err
	}
	// if logger.Enabled(ctx, slog2.LevelTrace) {
	// 	logger.Log(ctx, slog2.LevelTrace, "task next step",
	// 		"taskSteps", stringerIter(taskOrderedSteps(tSteps)),
	// 		"doneSteps", stringerList(doneSteps),
	// 		"next", next.String())
	// }
	// teardown can be executed out of order.
	if next != currentStep && (currentStep != StepTeardown || slices.Contains(doneSteps, StepTeardown)) {
		if next == StepInvalid {
			return fmt.Errorf("%w: task has no next step but trying to run '%s'",
				ErrInvalidStepOrder, currentStep)
		}
		return fmt.Errorf("%w: task next step should be '%s' but trying to run '%s'",
			ErrInvalidStepOrder, next, currentStep)
	}
	return nil
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
