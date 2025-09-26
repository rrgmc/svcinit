package svcinit

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"gotest.tools/v3/assert"
)

func TestTaskWrapper_stepCanRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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

		tw := newTaskWrapper(testTask)

		canStart, err := tw.checkStartStep(StepTeardown)
		assert.NilError(t, err)
		assert.Equal(t, canStart, false)
	})
}

func TestTaskWrapper_executeOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()

		logger := nullLogger()

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
			WithTeardown(func(ctx context.Context) error {
				return nil
			}),
		)

		tw := newTaskWrapper(testTask)

		canStart, err := tw.checkStartStep(StepSetup)
		assert.Assert(t, canStart)
		assert.NilError(t, err)
		canRun := tw.checkRunStep(StepSetup)
		assert.Assert(t, canRun)
		err = tw.run(ctx, logger, StageDefault, StepSetup, nil)
		assert.NilError(t, err)

		canStart, err = tw.checkStartStep(StepTeardown)
		assert.Assert(t, canStart)
		assert.NilError(t, err)
		canRun = tw.checkRunStep(StepTeardown)
		assert.Assert(t, canRun)
		err = tw.run(ctx, logger, StageDefault, StepTeardown, nil)
		assert.NilError(t, err)

		canStart, err = tw.checkStartStep(StepStop)
		assert.ErrorIs(t, err, ErrInvalidStepOrder)
		assert.Equal(t, false, canStart)
	})
}
