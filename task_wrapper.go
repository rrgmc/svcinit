package svcinit

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

type taskWrapper struct {
	task    Task
	options taskOptions

	mu           sync.Mutex
	executeSteps []Step
	startCancel  context.CancelCauseFunc
	finishCtx    context.Context
}

func newTaskWrapper(task Task, options ...TaskOption) *taskWrapper {
	ret := &taskWrapper{
		task: task,
	}
	for _, option := range options {
		option.applyTaskOpt(&ret.options)
	}
	if to, ok := task.(TaskWithOptions); ok {
		for _, option := range to.TaskOptions() {
			option.applyTaskInstanceOpt(&ret.options)
		}
	}
	return ret
}

// checkStartStep checks if the step can be started for this task.
// Returns any logic error found.
func (t *taskWrapper) checkStartStep(step Step) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	canStartStep := t.internalCanStartStep(step)
	prevStepIsDone, err := t.internalPrevStepIsDone(step)
	if err != nil {
		return false, err
	}
	if !canStartStep && prevStepIsDone {
		t.internalAddStepDone(step)
	}
	return canStartStep && prevStepIsDone, nil
}

// checkRunStep checks if the step can be run, after checkStartStep allowed it to start.
func (t *taskWrapper) checkRunStep(step Step) bool {
	return taskHasStep(t.task, step)
}

// run runs the task.
// checkStartStep and checkRunStep must be called prior to calling this.
func (t *taskWrapper) run(ctx context.Context, stage string, step Step, callbacks []TaskCallback,
	taskErrorHandler TaskErrorHandler) (err error) {
	t.runCallbacks(ctx, stage, step, CallbackStepBefore, nil, callbacks)
	if step != StepSetup {
		// setup is only added if no run error, so start and stop are not called in that case.
		t.addStepDone(step)
	}
	if t.options.handler != nil {
		err = t.options.handler(ctx, t.task, step)
	} else {
		err = t.task.Run(ctx, step)
	}
	if taskErrorHandler != nil {
		err = taskErrorHandler(ctx, t.task, step, err)
	}
	if step == StepSetup && err == nil {
		t.addStepDone(step)
	}
	t.runCallbacks(ctx, stage, step, CallbackStepAfter, err, callbacks)
	return err
}

func (t *taskWrapper) runCallbacks(ctx context.Context, stage string, step Step, callbackStep CallbackStep, err error,
	callbacks []TaskCallback) {
	for _, callback := range slices.Concat(callbacks, t.options.callbacks) {
		callback.Callback(ctx, t.task, stage, step, callbackStep, err)
	}
}

func (t *taskWrapper) addStepDone(step Step) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.internalAddStepDone(step)
}

func (t *taskWrapper) internalAddStepDone(step Step) {
	t.executeSteps = append(t.executeSteps, step)
}

func (t *taskWrapper) internalCanStartStep(step Step) bool {
	if taskHasStep(t.task, step) {
		return true
	}
	// if SSM is enabled, a stop step must exist for it to work. If it don't exist, create an internal one.
	if step == StepStop {
		return t.options.startStepManager
	}
	return false
}

func (t *taskWrapper) internalStepIsDone(step Step) bool {
	return slices.Contains(t.executeSteps, step)
}

func (t *taskWrapper) internalStepsAreDoneAny(steps ...Step) bool {
	for _, step := range steps {
		if t.internalStepIsDone(step) {
			return true
		}
	}
	return false
}

func (t *taskWrapper) internalPrevStepIsDone(step Step) (bool, error) {
	switch step {
	case StepSetup:
		if !t.internalStepsAreDoneAny(StepSetup, StepStart, StepStop, StepTeardown) {
			return true, nil
		}
	case StepStart:
		if !t.internalStepsAreDoneAny(StepStart, StepStop, StepTeardown) {
			return t.internalStepsAreDoneAny(StepSetup), nil
		}
	case StepStop:
		if !t.internalStepsAreDoneAny(StepStop, StepTeardown) {
			return t.internalStepsAreDoneAny(StepStart), nil
		}
	case StepTeardown:
		if !t.internalStepsAreDoneAny(StepTeardown) {
			return t.internalStepsAreDoneAny(StepSetup), nil
		}
	default:
		return false, fmt.Errorf("%w: step '%s' is not a valid step", ErrInvalidTaskStep, step.String())
	}
	return false, fmt.Errorf("%w: invalid order for step '%s': already done '%s'", ErrInvalidStepOrder,
		step, stringerString(t.executeSteps))
}
