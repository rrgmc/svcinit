package futuretask

import (
	"cmp"
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/instancetask"
	"gotest.tools/v3/assert"
	cmp3 "gotest.tools/v3/assert/cmp"
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
		items := &testList[string]{}

		sinit, err := svcinit.New(
			svcinit.WithStages("init", "service"),
			// WithLogger(defaultLogger(t.Output())),
		)
		assert.NilError(t, err)

		initTask1 := NewTaskFuture[*idata1](
			func(ctx context.Context) (*idata1, error) {
				items.add("i1setup")
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
				items.add("i2setup")
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
					items.add("sstart")
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

					return sleepContext(ctx, time.Second)
				}),
				svcinit.WithStop(func(ctx context.Context) error {
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		items.assertDeepEqual(t, []string{"i1setup", "i2setup", "sstart"})
	})
}

type testList[T any] struct {
	m    sync.Mutex
	list []T
}

func (l *testList[T]) add(item T) {
	l.m.Lock()
	l.list = append(l.list, item)
	l.m.Unlock()
}

func (l *testList[T]) get() []T {
	l.m.Lock()
	defer l.m.Unlock()
	return l.list
}

func (l *testList[T]) assertDeepEqual(t *testing.T, expected []T) {
	assert.DeepEqual(t, expected, l.get(), cmpopts.SortSlices(cmp.Less[string]))
}

func (l *testList[T]) checkDeepEqual(t *testing.T, expected []T) bool {
	return assert.Check(t, cmp3.DeepEqual(expected, l.get(), cmpopts.SortSlices(cmp.Less[string])))
}

// sleepContext sleeps while checking for context cancellation.
// Returns nil for any option by default. These can be changed by options.
func sleepContext(ctx context.Context, duration time.Duration, options ...sleepContextOption) error {
	var optns sleepContextOptions
	for _, opt := range options {
		opt(&optns)
	}
	select {
	case <-ctx.Done():
		if optns.contextError {
			return context.Cause(ctx)
		}
		return nil
	case <-time.After(duration):
		return optns.timeoutErr
	}
}

type sleepContextOption func(*sleepContextOptions)

func withSleepContextError(contextError bool) sleepContextOption {
	return func(opts *sleepContextOptions) {
		opts.contextError = contextError
	}
}

func withSleepContextTimeoutError(timeoutErr error) sleepContextOption {
	return func(o *sleepContextOptions) {
		o.timeoutErr = timeoutErr
	}
}

type sleepContextOptions struct {
	contextError bool
	timeoutErr   error
}
