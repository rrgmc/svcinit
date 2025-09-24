package svcinit

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"gotest.tools/v3/assert"
)

func TestTaskWrapper_invalidStep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		testTask := BuildTask(
			WithSetup(func(ctx context.Context) error {
				return nil
			}),
			WithStart(func(ctx context.Context) error {
				return sleepContext(ctx, time.Second)
			}),
			WithStop(func(ctx context.Context) error {
				return nil
			}),
		)

		tw := newTaskWrapper(StageDefault, testTask)

		err := tw.run(ctx, StageDefault, StepPreStop, nil)
		assert.ErrorIs(t, err, ErrInvalidTaskStep)
	})
}

func TestTaskWrapper_executeOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		testTask := BuildTask(
			WithSetup(func(ctx context.Context) error {
				return nil
			}),
			WithStart(func(ctx context.Context) error {
				return sleepContext(ctx, time.Second)
			}),
			WithPreStop(func(ctx context.Context) error {
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				return nil
			}),
		)

		tw := newTaskWrapper(StageDefault, testTask)

		err := tw.run(ctx, StageDefault, StepSetup, nil)
		assert.NilError(t, err)

		err = tw.run(ctx, StageDefault, StepStart, nil)
		assert.NilError(t, err)

		err = tw.run(ctx, StageDefault, StepStop, nil)
		assert.Check(t, errors.Is(err, ErrInvalidStepOrder))

		err = tw.run(ctx, StageDefault, StepPreStop, nil)
		assert.NilError(t, err)
	})
}
