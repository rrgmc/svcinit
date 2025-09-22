package svcinit

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync"
	"time"
)

type taskWrapper struct {
	task    Task
	options taskOptions

	mu          sync.Mutex
	startCancel context.CancelCauseFunc
	finishCtx   context.Context
}

func (m *Manager) newTaskWrapper(task Task, options ...TaskOption) *taskWrapper {
	ret := &taskWrapper{
		task: task,
		options: taskOptions{
			stage: m.defaultStage,
		},
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

func (t *taskWrapper) run(ctx context.Context, stage string, step Step, callbacks []TaskCallback) (err error) {
	t.runCallbacks(ctx, stage, step, CallbackStepBefore, nil, callbacks)
	if t.options.handler != nil {
		err = t.options.handler(ctx, t.task, step)
	} else {
		err = t.task.Run(ctx, step)
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

func (t *taskWrapper) hasStep(step Step) bool {
	if taskHasStep(step, t.task) {
		return true
	}
	if step == StepStop {
		return t.options.startStepManager
	}
	return false
}

type stageTasks struct {
	tasks map[string][]*taskWrapper
}

func newStageTasks() *stageTasks {
	return &stageTasks{
		tasks: make(map[string][]*taskWrapper),
	}
}

func (s *stageTasks) add(tw *taskWrapper) {
	s.tasks[tw.options.stage] = append(s.tasks[tw.options.stage], tw)
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

// stepStagesIter returns an interator to a list of stages.
// For the pre-stop and stop steps, it returns a reverse iterator.
func stepStagesIter(step Step, stages []string) iter.Seq[string] {
	if step == StepPreStop || step == StepStop {
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
	Errors []error
}

func (e *multiError) Error() string {
	err := e.Err()
	if err == nil {
		return "empty errors"
	}
	return err.Error()
}

func (e *multiError) Err() error {
	return errors.Join(e.Errors...)
}

func (e *multiError) Unwrap() []error {
	return e.Errors
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

func (b *multiErrorBuilder) add(err error) {
	if err == nil {
		return
	}
	b.m.Lock()
	defer b.m.Unlock()
	if me, ok := err.(*multiError); ok {
		b.errs = append(b.errs, me.Errors...)
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
		Errors: slices.Clone(b.errs),
	}
}

func buildMultiErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	} else if len(errs) == 1 {
		return errs[0]
	}
	return &multiError{
		Errors: slices.Clone(errs),
	}
}

// sleepContext sleeps while checking for context cancellation.
// Returns [context.Cause] of the context error, or nil if timed out.
func sleepContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(duration):
		return nil
	}
}
