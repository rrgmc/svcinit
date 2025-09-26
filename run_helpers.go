package svcinit

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"time"
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

// run runs the task.
// checkStartStep and checkRunStep must be called prior to calling this.
func (t *taskWrapper) run(ctx context.Context, logger *slog.Logger, stage string, step Step, callbacks []TaskCallback) (err error) {
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

type stageTasks struct {
	tasks map[string][]*taskWrapper
}

func newStageTasks() *stageTasks {
	return &stageTasks{
		tasks: make(map[string][]*taskWrapper),
	}
}

func (s *stageTasks) add(stage string, tw *taskWrapper) {
	s.tasks[stage] = append(s.tasks[stage], tw)
}

func (s *stageTasks) stageTasks(stage string) iter.Seq[*taskWrapper] {
	return func(yield func(*taskWrapper) bool) {
		for _, t := range s.tasks[stage] {
			if !yield(t) {
				return
			}
		}
	}
}

func (s *stageTasks) stepTaskCount(step Step) (ct int) {
	for _, tasks := range s.tasks {
		for _, task := range tasks {
			if slices.Contains(taskSteps(task.task), step) {
				ct++
			}
		}
	}
	return ct
}

// waitGroupWaitWithContext waits for the WaitGroup or the context to be done.
// Returns false if waiting timed out.
func waitGroupWaitWithContext(ctx context.Context, wg *sync.WaitGroup) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return true // completed normally
	case <-ctx.Done():
		return false // timed out
	}
}

// stagesIter returns an interator to a list of stages.
func stagesIter(stages []string, reversed bool) iter.Seq[string] {
	if reversed {
		return reversedSlice(stages)
	} else {
		return slices.Values(stages)
	}
}

// reversedSlice returns a reversed iterator to a slice.
func reversedSlice[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(s) - 1; i >= 0; i-- {
			if !yield(s[i]) {
				return
			}
		}
	}
}

// multiError is an error containing a list of errors.
type multiError struct {
	errors []error
}

func (e *multiError) Error() string {
	if len(e.errors) == 0 {
		return "empty errors"
	}
	return e.errors[0].Error()
}

func (e *multiError) JoinedError() error {
	return errors.Join(e.errors...)
}

func (e *multiError) Unwrap() []error {
	return e.errors
}

// multiErrorBuilder is a thread-safe error builder. It is used to avoid a mutex being return in the final error.
// Returns a multiError error.
type multiErrorBuilder struct {
	m    sync.Mutex
	errs []error
}

func newMultiErrorBuilder() *multiErrorBuilder {
	return &multiErrorBuilder{}
}

func (b *multiErrorBuilder) hasErrors() bool {
	b.m.Lock()
	defer b.m.Unlock()
	return len(b.errs) > 0
}

func (b *multiErrorBuilder) add(err error) {
	if err == nil {
		return
	}
	b.m.Lock()
	defer b.m.Unlock()
	if me, ok := err.(*multiError); ok {
		b.errs = append(b.errs, me.errors...)
	} else {
		b.errs = append(b.errs, err)
	}
}

func (b *multiErrorBuilder) build() error {
	b.m.Lock()
	defer b.m.Unlock()
	if len(b.errs) == 0 {
		return nil
	} else if len(b.errs) == 1 {
		return b.errs[0]
	}
	return &multiError{
		errors: slices.Clone(b.errs),
	}
}

func buildMultiErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	} else if len(errs) == 1 {
		return errs[0]
	}
	return &multiError{
		errors: slices.Clone(errs),
	}
}

// sleepContext sleeps while checking for context cancellation.
// Returns nil for any option by default. These can be changed by options.
func sleepContext(ctx context.Context, duration time.Duration, options ...sleepContextOption) error {
	var optns sleepContextOptions
	for _, opt := range options {
		opt(&optns)
	}
	select {
	case <-ctx.Done():
		if optns.contextError {
			return context.Cause(ctx)
		}
		return nil
	case <-time.After(duration):
		return optns.timeoutErr
	}
}

type sleepContextOption func(*sleepContextOptions)

func withSleepContextError(contextError bool) sleepContextOption {
	return func(opts *sleepContextOptions) {
		opts.contextError = contextError
	}
}

func withSleepContextTimeoutError(timeoutErr error) sleepContextOption {
	return func(o *sleepContextOptions) {
		o.timeoutErr = timeoutErr
	}
}

type sleepContextOptions struct {
	contextError bool
	timeoutErr   error
}
