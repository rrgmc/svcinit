package svcinit

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

func TestBuildDataTask(t *testing.T) {
	type data struct {
		value1 string
		value2 int
	}

	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)

		sinit.
			AddTask(BuildDataTask[*data](func(ctx context.Context) (*data, error) {
				return &data{
					value1: "test",
					value2: 13,
				}, nil
			},
				WithDataStart(func(ctx context.Context, data *data) error {
					assert.Check(t, cmp.Equal("test", data.value1))
					assert.Check(t, cmp.Equal(13, data.value2))
					select {
					case <-time.After(1 * time.Second):
						return nil
					case <-ctx.Done():
					}
					return nil
				}),
				WithDataStop(func(ctx context.Context, data *data) error {
					assert.Check(t, cmp.Equal("test", data.value1))
					assert.Check(t, cmp.Equal(13, data.value2))
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)
	})
}
