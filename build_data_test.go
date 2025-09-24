package svcinit

import (
	cmp2 "cmp"
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
)

func TestBuildDataTask(t *testing.T) {
	type data struct {
		value1 string
		value2 int
	}

	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		sinit, err := New()
		assert.NilError(t, err)

		sinit.
			AddTask(StageDefault, BuildDataTask(func(ctx context.Context) (*data, error) {
				return &data{
					value1: "test",
					value2: 13,
				}, nil
			},
				WithDataStart(func(ctx context.Context, data *data) error {
					items.add("start")
					assert.Check(t, cmp.Equal("test", data.value1))
					assert.Check(t, cmp.Equal(13, data.value2))
					return sleepContext(ctx, time.Second)
				}),
				WithDataStop(func(ctx context.Context, data *data) error {
					items.add("stop")
					assert.Check(t, cmp.Equal("test", data.value1))
					assert.Check(t, cmp.Equal(13, data.value2))
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"start", "stop"}, items.get(), cmpopts.SortSlices(cmp2.Less[string]))
	})
}
