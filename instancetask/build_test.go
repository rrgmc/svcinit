package instancetask

import (
	"cmp"
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/internal/testutils"
	"gotest.tools/v3/assert"
)

func TestBuildTaskEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, Build[int](nil))

		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, svcinit.ErrNilTask)
	})
}

func TestBuildTaskEmptyNil(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, Build[int](
			func(ctx context.Context) (int, error) {
				return 1, nil
			},
			WithStart[int](nil),
		))

		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, svcinit.ErrNilTask)
	})
}

func TestBuildTask(t *testing.T) {
	type data struct {
		value1 string
		value2 int
	}

	synctest.Test(t, func(t *testing.T) {
		items := &testutils.TestList[string]{}

		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.
			AddTask(svcinit.StageDefault, Build(func(ctx context.Context) (*data, error) {
				return &data{
					value1: "test",
					value2: 13,
				}, nil
			},
				WithStart(func(ctx context.Context, data *data) error {
					items.Add("start")
					assert.Check(t, cmp2.Equal("test", data.value1))
					assert.Check(t, cmp2.Equal(13, data.value2))
					return testutils.SleepContext(ctx, time.Second)
				}),
				WithStop(func(ctx context.Context, data *data) error {
					items.Add("stop")
					assert.Check(t, cmp2.Equal("test", data.value1))
					assert.Check(t, cmp2.Equal(13, data.value2))
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"start", "stop"}, items.Get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestBuildTaskInfoFromParent(t *testing.T) {
	var calls []string
	parent := svcinit.BuildTask(
		svcinit.WithName("parent"),
		svcinit.WithStart(func(ctx context.Context) error { return nil }),
		svcinit.WithTaskOptions(svcinit.WithCancelContext(true)),
		svcinit.WithNotRun(func(ctx context.Context, cause error) { calls = append(calls, "parent") }),
	)

	task := Build[int](func(ctx context.Context) (int, error) { return 1, nil },
		WithParent[int](parent),
		WithNotRun[int](func(ctx context.Context, cause error) { calls = append(calls, "task") }),
	)

	info := svcinit.GetTaskInfo(task)
	assert.NilError(t, info.InitError)
	assert.Equal(t, "parent", info.Name)
	assert.DeepEqual(t, []svcinit.Step{svcinit.StepSetup, svcinit.StepStart}, info.Steps,
		cmpopts.SortSlices(cmp.Less[svcinit.Step]))
	assert.Equal(t, 1, len(info.Options))
	assert.Assert(t, info.NotRun != nil)
	info.NotRun(t.Context(), nil)
	assert.DeepEqual(t, []string{"task", "parent"}, calls)
}

func TestProviderInitErrorFromSetup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, Provider(func(ctx context.Context) (svcinit.Task, error) {
			// a provided task with an initialization error must fail the setup step.
			return svcinit.BuildTask(svcinit.WithStart(nil)), nil
		}))
		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, svcinit.ErrNilTask)
	})
}

// TestProviderTaskOptions is a regression test: the options of the task returned by the Provider callback used to
// be ignored, as task options were computed when the task was added, before the "setup" step set the parent.
func TestProviderTaskOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var startCancelled, ssmCanCancel atomic.Bool

		sinit, err := svcinit.New(
			// if the options are ignored, the start steps are never cancelled: give up waiting instead of blocking.
			svcinit.WithEnforceShutdownTimeout(true),
		)
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, Provider(func(ctx context.Context) (svcinit.Task, error) {
			return svcinit.BuildTask(
				svcinit.WithStart(func(ctx context.Context) error {
					// only cancelled by the Manager because of the WithCancelContext(true) option.
					<-ctx.Done()
					startCancelled.Store(true)
					return nil
				}),
				svcinit.WithTaskOptions(svcinit.WithCancelContext(true)),
			), nil
		}))

		sinit.AddTask(svcinit.StageDefault, Provider(func(ctx context.Context) (svcinit.Task, error) {
			return svcinit.BuildTask(
				svcinit.WithStart(func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				}),
				svcinit.WithStop(func(ctx context.Context) error {
					// only available because of the WithStartStepManager() option.
					ssm := svcinit.StartStepManagerFromContext(ctx)
					ssmCanCancel.Store(ssm.CanContextCancel())
					ssm.ContextCancel(context.Canceled)
					return nil
				}),
				svcinit.WithTaskOptions(svcinit.WithStartStepManager()),
			), nil
		}))

		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second, svcinit.WithoutTimeoutTaskError()))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)
		assert.Check(t, startCancelled.Load())
		assert.Check(t, ssmCanCancel.Load())
	})
}
