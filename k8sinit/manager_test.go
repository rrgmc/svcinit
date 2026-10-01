package k8sinit

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

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

func TestManagerHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testutils.TestList[string]{}

		sm, err := New(
			WithDisableSignalHandling(), // not compatible with synctest.
		)
		assert.NilError(t, err)

		sm.SetHealthHandler(svcinit.BuildHealthHandler(
			svcinit.WithHealthHandlerServiceStarted(func(ctx context.Context) {
				items.Add("started")
			}),
			svcinit.WithHealthHandlerServiceTerminating(func(ctx context.Context) {
				items.Add("terminating")
			}),
		))
		sm.SetTelemetryHandler(BuildTelemetryHandler(
			WithTelemetryHandlerFlushTelemetry(func(ctx context.Context) error {
				items.Add("flush")
				return nil
			}),
		))

		sm.AddTask(StageService, svcinit.BuildTask(
			svcinit.WithStart(func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}),
			svcinit.WithTaskOptions(svcinit.WithCancelContext(true)),
		))
		sm.AddTask(StageService, svcinit.TimeoutTask(time.Second, svcinit.WithoutTimeoutTaskError()))

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		items.AssertDeepEqual(t, []string{"started", "flush", "terminating"})
	})
}

func TestManagerHandlersAlreadySet(t *testing.T) {
	sm, err := New(
		WithDisableSignalHandling(), // not compatible with synctest.
	)
	assert.NilError(t, err)

	sm.SetHealthHandler(svcinit.BuildHealthHandler())
	sm.SetHealthHandler(svcinit.BuildHealthHandler())

	err = sm.Run(t.Context())
	assert.ErrorIs(t, err, svcinit.ErrAlreadyInitialized)
}
