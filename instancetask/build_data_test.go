package instancetask

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
	"gotest.tools/v3/assert"
	cmp3 "gotest.tools/v3/assert/cmp"
)

func TestBuildDataTaskEmpty(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, BuildDataTask[int](nil))

		sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(time.Second))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, svcinit.ErrNilTask)
	})
}

func TestBuildDataTaskEmptyNil(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.AddTask(svcinit.StageDefault, BuildDataTask[int](
			func(ctx context.Context) (int, error) {
				return 1, nil
			},
			WithDataStart[int](nil),
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
		items := &testList[string]{}

		sinit, err := svcinit.New()
		assert.NilError(t, err)

		sinit.
			AddTask(svcinit.StageDefault, BuildDataTask(func(ctx context.Context) (*data, error) {
				return &data{
					value1: "test",
					value2: 13,
				}, nil
			},
				WithDataStart(func(ctx context.Context, data *data) error {
					items.add("start")
					assert.Check(t, cmp2.Equal("test", data.value1))
					assert.Check(t, cmp2.Equal(13, data.value2))
					return sleepContext(ctx, time.Second)
				}),
				WithDataStop(func(ctx context.Context, data *data) error {
					items.add("stop")
					assert.Check(t, cmp2.Equal("test", data.value1))
					assert.Check(t, cmp2.Equal(13, data.value2))
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"start", "stop"}, items.get(), cmpopts.SortSlices(cmp.Less[string]))
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
