package instancetask

import (
	"cmp"
	"context"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/internal/testutils"
	"gotest.tools/v3/assert"
)

func TestBuildDataTaskEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, Build[int](nil))

		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, svcinit.ErrNilTask)
	})
}

func TestBuildDataTaskEmptyNil(t *testing.T) {
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

func TestBuildDataTask(t *testing.T) {
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
