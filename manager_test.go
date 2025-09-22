package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
)

func TestManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := &testList[string]{}
		stopped := &testList[string]{}
		prestopped := &testList[string]{}

		sm, err := New(
			WithStages("health", "service"),
			WithDefaultStage("health"),
		)

		sm.AddTask(BuildTask(
			WithStart(func(ctx context.Context) error {
				started.add("task1")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				stopped.add("task1")
				return nil
			}),
			WithPreStop(func(ctx context.Context) error {
				prestopped.add("task1")
				return nil
			}),
		))
		assert.NilError(t, err)

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"task1"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task1"}, prestopped.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task1"}, stopped.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestManagerWorkflows(t *testing.T) {
	for _, test := range []struct {
		name            string
		cancelFn        func() []int
		expectedTaskErr int
		expectedErr     error
	}{
		{
			name: "stop 1: cancel execute unordered",
			cancelFn: func() []int {
				return []int{1}
			},
			expectedTaskErr: 1,
		},
		{
			name: "stop 2: cancel task ordered",
			cancelFn: func() []int {
				return []int{2}
			},
			expectedTaskErr: 2,
		},
		{
			name: "stop 3: cancel task context ordered",
			cancelFn: func() []int {
				return []int{3}
			},
			expectedTaskErr: 3,
		},
		{
			name: "stop 4: cancel service ordered",
			cancelFn: func() []int {
				return []int{4}
			},
			expectedTaskErr: 4,
		},
		{
			name: "stop 5: cancel task auto",
			cancelFn: func() []int {
				return []int{5}
			},
			expectedTaskErr: 5,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			type testService struct {
				svc    Task
				cancel context.CancelFunc
			}

			defaultTaskSvc := func(taskNo int) testService {
				dtCtx, dtCancel := context.WithCancelCause(ctx)
				stCtx, stCancel := context.WithCancel(ctx)
				return testService{
					cancel: func() {
						dtCancel(testTaskNoError{taskNo: taskNo})
					},
					svc: TaskFunc(func(ctx context.Context, step Step) error {
						switch step {
						case StepStart:
							defer stCancel()

							select {
							case <-dtCtx.Done():
								return context.Cause(dtCtx)
							case <-ctx.Done():
								return context.Cause(ctx)
							}
						case StepStop:
							dtCancel(ErrExit)
							select {
							case <-stCtx.Done():
							case <-ctx.Done():
							}
						default:
						}
						return nil
					}),
				}
			}

			tasks := make([]testService, 5)
			for i := range 5 {
				tasks[i] = defaultTaskSvc(i + 1)
			}

			sinit, err := New(
				WithStages("default", "s2", "s3", "s4"),
				WithDefaultStage("default"),
				WithManagerCallback(ManagerCallbackFunc(func(ctx context.Context, stage string, step Step, callbackStep CallbackStep) error {
					if step != StepStart || callbackStep != CallbackStepAfter {
						return nil
					}
					for _, taskNo := range test.cancelFn() {
						tasks[taskNo-1].cancel()
					}
					return nil
				})),
			)
			assert.NilError(t, err)

			sinit.AddTask(tasks[0].svc)

			sinit.AddTask(tasks[1].svc, WithStage("s2"))

			sinit.AddTask(tasks[2].svc,
				WithStage("s3"),
				WithCancelContext(true))

			sinit.AddTask(tasks[3].svc, WithStage("s4"))

			sinit.AddTask(tasks[4].svc,
				WithCancelContext(true))

			sinit.AddTask(SignalTask(os.Interrupt, syscall.SIGTERM))

			err = sinit.Run(ctx)
			if test.expectedErr != nil {
				assert.ErrorIs(t, err, test.expectedErr)
			} else if test.expectedTaskErr > 0 {
				checkTestTaskError(t, err, test.expectedTaskErr)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestManagerWithoutTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)
		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, ErrNoStartTask)
	})
}

func TestManagerShutdownOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		shutdownCtx := context.WithValue(t.Context(), "test-shutdown", 5)

		sinit, err := New()
		assert.NilError(t, err)

		sinit.AddTask(BuildTask(
			WithStart(func(ctx context.Context) error {
				assert.Check(t, !cmp2.Equal(5, ctx.Value("test-shutdown")), "not expected context value")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				assert.Check(t, cmp2.Equal(5, ctx.Value("test-shutdown")), "expected context value is different")
				return nil
			}),
		))

		// sinit.Shutdown()
		err = sinit.Run(t.Context(), WithRunShutdownContext(shutdownCtx))
		assert.NilError(t, err)
	})
}

func TestManagerShutdownTimeout(t *testing.T) {
	timeoutTest := func(taskStopSleep time.Duration, isError bool) {
		synctest.Test(t, func(t *testing.T) {
			sinit, err := New(WithShutdownTimeout(30 * time.Second))
			assert.NilError(t, err)

			sinit.AddTask(BuildTask(
				WithStart(func(ctx context.Context) error {
					return nil
				}),
				WithStop(func(ctx context.Context) error {
					return sleepContext(ctx, taskStopSleep)
				}),
			))

			err, cleanupErr := sinit.RunWithStopErrors(t.Context())
			assert.NilError(t, err)
			if isError {
				assert.ErrorIs(t, cleanupErr, ErrShutdownTimeout)
			} else {
				assert.NilError(t, cleanupErr)
			}
		})
	}

	timeoutTest(40*time.Second, true)
	timeoutTest(10*time.Second, false)
}

func TestManagerNilTask(t *testing.T) {
	for _, test := range []struct {
		name string
		fn   func(sinit *Manager)
	}{
		{
			name: "nil execute",
			fn: func(sinit *Manager) {
				sinit.AddTask(nil) // wait
			},
		},
		{
			name: "nil task start, prestop and stop",
			fn: func(sinit *Manager) {
				sinit.AddTask(BuildTask(
					WithStart(nil),
					WithPreStop(nil),
					WithStop(nil)),
					WithCancelContext(true))
			},
		},
		{
			name: "nil task stop",
			fn: func(sinit *Manager) {
				sinit.AddTask(BuildTask(
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second)
					}),
					WithStop(nil),
				),
					WithCancelContext(true),
				)
			},
		},
		{
			name: "nil task prestop",
			fn: func(sinit *Manager) {
				sinit.AddTask(BuildTask(
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second)
					}),
					WithPreStop(nil),
					WithStop(func(ctx context.Context) error {
						return nil
					}),
				),
					WithCancelContext(true),
				)
			},
		},
		{
			name: "nil task multiple",
			fn: func(sinit *Manager) {
				sinit.AddTask(BuildTask(
					WithStart(nil),
					WithPreStop(nil),
					WithStop(nil)),
					WithCancelContext(true))
				sinit.AddTask(BuildTask(
					WithStart(nil),
					WithPreStop(nil),
					WithStop(nil)),
					WithCancelContext(true))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				sinit, err := New()
				assert.NilError(t, err)
				test.fn(sinit)
				err = sinit.Run(t.Context())
				assert.ErrorIs(t, err, ErrNilTask)
			})
		})
	}
}

func TestManagerShutdownContextNotCancelledByMainContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var isStart, isStop atomic.Bool

		mainCtx, mainCancel := context.WithCancel(t.Context())

		sinit, err := New()
		assert.NilError(t, err)

		sinit.
			AddTask(BuildTask(
				WithStart(func(ctx context.Context) error {
					select {
					case <-ctx.Done():
					}
					assert.Check(t, ctx.Err() != nil)
					isStart.Store(true)
					return nil
				}),
				WithStop(func(ctx context.Context) error {
					isStop.Store(true)
					assert.Check(t, ctx.Err() == nil)
					return nil
				}),
			))

		time.AfterFunc(10*time.Second, func() {
			mainCancel()
		})

		err = sinit.Run(mainCtx)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, isStart.Load(), true)
		assert.Equal(t, isStop.Load(), true)
	})
}

type testTask = TaskWithID[int]

func newTestTask(id int, t Task) *testTask {
	return NewTaskWithID(id, t)
}

func getTestTaskNo(task Task) (int, bool) {
	if tt, ok := task.(*testTask); ok {
		return tt.TaskID(), true
	}
	return -1, false
}

func assertTestTask(t *testing.T, task Task, stage string, step Step, callbackStep CallbackStep) bool {
	if _, ok := task.(*testTask); !ok {
		return assert.Check(t, false, "task is not of the expected type but %T (stage:%s)(step:%s)(callbackStep:%s)",
			task, stage, step, callbackStep)
	}
	return true
}

type testTaskNoError struct {
	taskNo int
}

func (t testTaskNoError) Error() string {
	return fmt.Sprintf("task %d error", t.taskNo)
}

func checkTestTaskError(t *testing.T, err error, taskNo int) {
	var tne testTaskNoError
	if errors.As(err, &tne) {
		assert.Equal(t, tne.taskNo, taskNo, "expected task %d error got task %d", taskNo, tne.taskNo)
	} else {
		assert.Assert(t, false, "unexpected error type %T (%v)", err, err)
	}
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
