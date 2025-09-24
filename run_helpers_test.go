package svcinit

import (
	"cmp"
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
)

func TestTaskWrapper_executeOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		sinit, err := New()
		assert.NilError(t, err)

		sinit.AddTask(StageDefault, BuildTask(
			WithSetup(func(ctx context.Context) error {
				items.add("setup")
				return nil
			}),
			WithStart(func(ctx context.Context) error {
				items.add("start")
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(time.Second):
					return nil
				}
			}),
			WithPreStop(func(ctx context.Context) error {
				items.add("pre-stop")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				items.add("stop")
				return nil
			}),
		), WithCallback(
			TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {

			}),
		))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"setup", "start", "stop", "pre-stop"}, items.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}
