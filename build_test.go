package svcinit

import (
	"context"
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
		assert.NilError(t, task.TaskInitError())
		assert.Equal(t, "renamed", task.TaskName())

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
	assert.ErrorIs(t, task.TaskInitError(), ErrDuplicateStep)

	assert.NilError(t, task.SetParent(nil))
	assert.NilError(t, task.TaskInitError())
}
