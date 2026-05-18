package futuretask

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/instancetask"
	"github.com/rrgmc/svcinit/v3/internal/testutils"
	"gotest.tools/v3/assert"
)

func TestManagerInitData(t *testing.T) {
	type idata1 struct {
		value1 string
		value2 int
	}
	type idata2 struct {
		value3 int
		value4 string
	}

	synctest.Test(t, func(t *testing.T) {
		items := &testutils.TestList[string]{}

		sinit, err := svcinit.New(
			svcinit.WithStages("init", "service"),
			// WithLogger(defaultLogger(t.Output())),
		)
		assert.NilError(t, err)

		initTask1 := NewTaskFuture[*idata1](
			func(ctx context.Context) (*idata1, error) {
				items.Add("i1setup")
				ivalue := idata1{
					value1: "test33",
					value2: 33,
				}
				return &ivalue, nil
			},
			instancetask.WithDataName[*idata1]("idata1"))
		sinit.AddTask("init", initTask1)

		initTask2 := NewTaskFuture[*idata2](
			func(ctx context.Context) (*idata2, error) {
				items.Add("i2setup")
				ivalue := idata2{
					value3: 88,
					value4: "test88",
				}
				return &ivalue, nil
			},
			instancetask.WithDataName[*idata2]("idata2"))

		sinit.AddTask("init", initTask2)

		sinit.
			AddTask("service", svcinit.BuildTask(
				svcinit.WithStart(func(ctx context.Context) error {
					items.Add("sstart")
					initdata1, err := initTask1.Value()
					if !assert.Check(t, cmp2.Equal(nil, err)) {
						return err
					}
					initdata2, err := initTask2.Value()
					if !assert.Check(t, cmp2.Equal(nil, err)) {
						return err
					}

					assert.Check(t, cmp2.Equal(initdata1.value1, "test33"))
					assert.Check(t, cmp2.Equal(initdata1.value2, 33))
					assert.Check(t, cmp2.Equal(initdata2.value3, 88))
					assert.Check(t, cmp2.Equal(initdata2.value4, "test88"))

					return testutils.SleepContext(ctx, time.Second)
				}),
				svcinit.WithStop(func(ctx context.Context) error {
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		items.AssertDeepEqual(t, []string{"i1setup", "i2setup", "sstart"})
	})
}
