package futuretask

import (
	"context"
	"fmt"
	"slices"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/instancetask"
)

// New creates a task that resolves a [svcinit.Future] from the result of setupFunc's "setup" step.
// setupFunc must not be nil: unlike [instancetask.Build], there would be no data to resolve the future
// with.
// If the task never runs, for example because a previous stage failed, the future is resolved with
// [svcinit.ErrTaskNotRun], so waiters don't block forever.
func New[T any](setupFunc instancetask.BuildSetupFunc[T], options ...instancetask.BuildOption[T]) *Task[T] {
	future := svcinit.NewFuture[T]()
	if setupFunc == nil {
		setupFunc = func(context.Context) (T, error) {
			var empty T
			return empty, svcinit.ErrNilTask
		}
	}
	return &Task[T]{
		instanceTask: instancetask.Build[T](func(ctx context.Context) (T, error) {
			data, err := setupFunc(ctx)
			if err != nil {
				future.ResolveError(err)
				var empty T
				return empty, err
			}
			future.Resolve(data)
			return data, nil
		}, append(slices.Clip(options), instancetask.WithNotRun[T](func(_ context.Context, cause error) {
			select {
			case <-future.Done():
				return // resolving twice panics.
			default:
			}
			if cause != nil {
				future.ResolveError(fmt.Errorf("%w: %w", svcinit.ErrTaskNotRun, cause))
			} else {
				future.ResolveError(svcinit.ErrTaskNotRun)
			}
		}))...),
		future: future,
	}
}

// Task is a task created by [New], which is also the [svcinit.Future] resolved by its "setup" step.
type Task[T any] struct {
	*instanceTask[T]
	future svcinit.Future[T]
}

var _ svcinit.TaskFuture[int] = (*Task[int])(nil)
var _ svcinit.TaskWithInfo = (*Task[int])(nil)

func (t *Task[T]) Value(options ...svcinit.FutureValueOption) (T, error) {
	ret, err := t.future.Value(options...)
	if err != nil {
		return ret, fmt.Errorf("error resolving task data: %w", err)
	}
	return ret, nil
}

func (t *Task[T]) Done() <-chan struct{} {
	return t.future.Done()
}

// internal

type instanceTask[T any] = instancetask.Task[T]
