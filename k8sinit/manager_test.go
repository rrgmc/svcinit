package k8sinit

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/internal/testutils"
	"gotest.tools/v3/assert"
)

func TestManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testutils.TestList[string]{}

		sm, err := New(
			// WithLogger(defaultLogger(os.Stdout)),
			WithDisableSignalHandling(), // not compatible with synctest.
		)

		sm.AddTask(StageService, svcinit.BuildTask(
			svcinit.WithStart(func(ctx context.Context) error {
				items.Add("start")
				return nil
			}),
			svcinit.WithStop(func(ctx context.Context) error {
				items.Add("stop")
				return nil
			}),
		))
		assert.NilError(t, err)

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		items.AssertDeepEqual(t, []string{"start", "stop"})
	})
}
