package svcinit

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"gotest.tools/v3/assert"
)

func TestBuildTaskEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)

		sinit.AddTask(StageDefault, BuildTask())

		sinit.AddTask(StageDefault, TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, ErrNilTask)
	})
}

func TestBuildTaskEmptyNil(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)

		sinit.AddTask(StageDefault, BuildTask(WithStart(nil)))

		sinit.AddTask(StageDefault, TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, ErrNilTask)
	})
}

func TestBuildTaskParentOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)

		task := BuildTask(
			WithParent(TimeoutTask(time.Second, WithoutTimeoutTaskError())),
			WithName("renamed"),
		)
		assert.NilError(t, task.TaskInfo().InitError)
		assert.Equal(t, "renamed", task.TaskInfo().Name)

		sinit.AddTask(StageDefault, task)

		err = sinit.Run(t.Context())
		assert.NilError(t, err)
	})
}

func TestBuildTaskDuplicateParentStep(t *testing.T) {
	task := BuildTask(
		WithParent(TimeoutTask(time.Second)),
		WithStart(func(ctx context.Context) error { return nil }),
	)
	assert.ErrorIs(t, task.TaskInfo().InitError, ErrDuplicateStep)

	assert.NilError(t, task.SetParent(nil))
	assert.NilError(t, task.TaskInfo().InitError)
}

func TestBuildTaskDecorate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var handlerSteps []Step
		decoratedCh := make(chan Task, 1)

		sm, err := New(
			WithTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step,
				callbackStep CallbackStep, err error) {
				// callbacks receive the task passed to AddTask, there is nothing to unwrap.
				if GetTaskName(task) != TaskNameTimeout {
					assert.Check(t, task == <-decoratedCh)
					decoratedCh <- task
				}
			})),
		)
		assert.NilError(t, err)

		inner := &testTaskInfo{
			task: TaskFunc(func(ctx context.Context, step Step) error {
				// only finishes because the WithCancelContext(true) option from inner is applied.
				<-ctx.Done()
				return nil
			}),
			info: TaskInfo{
				Name:    "inner",
				Steps:   []Step{StepStart},
				Options: []TaskInstanceOption{WithCancelContext(true)},
			},
		}
		decorated := BuildTask(WithParent(inner), WithName("outer"))
		decoratedCh <- decorated

		info := decorated.TaskInfo()
		assert.NilError(t, info.InitError)
		assert.Equal(t, "outer", info.Name)
		assert.DeepEqual(t, []Step{StepStart}, info.Steps)
		assert.Equal(t, 1, len(info.Options))

		sm.AddTask(StageDefault, decorated, WithHandler(func(ctx context.Context, task Task, step Step) error {
			handlerSteps = append(handlerSteps, step)
			return task.Run(ctx, step)
		}))
		sm.AddTask(StageDefault, TimeoutTask(time.Second, WithoutTimeoutTaskError()))

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []Step{StepStart}, handlerSteps)
	})
}

func TestBuildTaskParentInitError(t *testing.T) {
	sinit, err := New()
	assert.NilError(t, err)

	task := BuildTask(WithParent(BuildTask(WithStart(nil))))
	assert.ErrorIs(t, task.TaskInfo().InitError, ErrNilTask)

	sinit.AddTask(StageDefault, task)

	err = sinit.Run(t.Context())
	assert.ErrorIs(t, err, ErrNilTask)
}

func TestBuildTaskSkipped(t *testing.T) {
	var calls []string
	errCause := errors.New("cause")

	task := BuildTask(
		WithParent(BuildTask(
			WithStart(func(ctx context.Context) error { return nil }),
			WithSkipped(func(ctx context.Context, cause error) {
				assert.Check(t, errors.Is(cause, errCause))
				calls = append(calls, "parent")
			}),
		)),
		WithSkipped(func(ctx context.Context, cause error) { calls = append(calls, "task1") }),
		WithSkipped(func(ctx context.Context, cause error) { calls = append(calls, "task2") }),
	)

	skipped := task.TaskInfo().Skipped
	assert.Assert(t, skipped != nil)
	skipped(t.Context(), errCause)
	assert.DeepEqual(t, []string{"task1", "task2", "parent"}, calls)

	assert.Assert(t, BuildTask(WithStart(func(ctx context.Context) error { return nil })).TaskInfo().Skipped == nil)
}

type testTaskInfo struct {
	task Task
	info TaskInfo
}

func (t *testTaskInfo) Run(ctx context.Context, step Step) error {
	return t.task.Run(ctx, step)
}

func (t *testTaskInfo) TaskInfo() TaskInfo {
	return t.info
}
