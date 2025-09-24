package svcinit

import (
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
