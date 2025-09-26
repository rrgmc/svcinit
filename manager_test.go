package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

		sm, err := New(
			WithStages("health", "service"),
		)

		sm.AddTask("health", BuildTask(
			WithStart(func(ctx context.Context) error {
				started.add("task1")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				stopped.add("task1")
				return nil
			}),
		))
		assert.NilError(t, err)

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"task1"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
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
			name: "stop 1: cancel task",
			cancelFn: func() []int {
				return []int{1}
			},
			expectedTaskErr: 1,
		},
		{
			name: "stop 2: cancel task",
			cancelFn: func() []int {
				return []int{2}
			},
			expectedTaskErr: 2,
		},
		{
			name: "stop 3: cancel task context",
			cancelFn: func() []int {
				return []int{3}
			},
			expectedTaskErr: 3,
		},
		{
			name: "stop 4: cancel service",
			cancelFn: func() []int {
				return []int{4}
			},
			expectedTaskErr: 4,
		},
		{
			name: "stop 5: cancel task context",
			cancelFn: func() []int {
				return []int{5}
			},
			expectedTaskErr: 5,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()

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
					WithStages(StageDefault, "s2", "s3", "s4"),
					WithManagerCallback(ManagerCallbackFunc(func(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
						if step != StepStart || callbackStep != CallbackStepAfter {
							return
						}
						for _, taskNo := range test.cancelFn() {
							tasks[taskNo-1].cancel()
						}
					})),
				)
				assert.NilError(t, err)

				sinit.AddTask(StageDefault, tasks[0].svc)

				sinit.AddTask("s2", tasks[1].svc)

				sinit.AddTask("s3", tasks[2].svc,
					WithCancelContext(true))

				sinit.AddTask("s4", tasks[3].svc)

				sinit.AddTask(StageDefault, tasks[4].svc,
					WithCancelContext(true))

				err = sinit.Run(ctx)
				if test.expectedErr != nil {
					assert.ErrorIs(t, err, test.expectedErr)
				} else if test.expectedTaskErr > 0 {
					checkTestTaskError(t, err, test.expectedTaskErr)
				} else {
					assert.NilError(t, err)
				}
			})
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

		sinit.AddTask(StageDefault, BuildTask(
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

			sinit.AddTask(StageDefault, BuildTask(
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
				sinit.AddTask(StageDefault, nil) // wait
			},
		},
		{
			name: "nil task start, stop",
			fn: func(sinit *Manager) {
				sinit.AddTask(StageDefault, BuildTask(
					WithStart(nil),
					WithStop(nil)),
					WithCancelContext(true))
			},
		},
		{
			name: "nil task stop",
			fn: func(sinit *Manager) {
				sinit.AddTask(StageDefault, BuildTask(
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
			name: "nil task multiple",
			fn: func(sinit *Manager) {
				sinit.AddTask(StageDefault, BuildTask(
					WithStart(nil),
					WithStop(nil)),
					WithCancelContext(true))
				sinit.AddTask(StageDefault, BuildTask(
					WithStart(nil),
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
			AddTask(StageDefault, BuildTask(
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

func TestManagerTaskHandler(t *testing.T) {
	var (
		err1 = errors.New("err1")
		err2 = errors.New("err2")
	)

	synctest.Test(t, func(t *testing.T) {
		sinit, err := New()
		assert.NilError(t, err)

		sinit.
			AddTask(StageDefault, BuildTask(
				WithStart(func(ctx context.Context) error {
					return sleepContext(ctx, time.Second,
						withSleepContextTimeoutError(err1))
				}),
				WithStop(func(ctx context.Context) error {
					return nil
				}),
			), WithHandler(func(ctx context.Context, task Task, step Step) error {
				switch step {
				case StepStart:
					return sleepContext(ctx, time.Second,
						withSleepContextTimeoutError(err2))
				default:
				}
				return nil
			}))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, err2)
	})
}

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

		sinit, err := New(
			WithStages("init", "service"),
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
			WithDataDescription[*idata1]("idata1"))
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
			WithDataDescription[*idata2]("idata2"))

		sinit.AddTask("init", initTask2)

		sinit.
			AddTask("service", BuildTask(
				WithStart(func(ctx context.Context) error {
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
				WithStop(func(ctx context.Context) error {
					return nil
				}),
			))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"i1setup", "i2setup", "sstart"}, items.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestManagerErrorReturns(t *testing.T) {
	var (
		err1 = errors.New("err1")
		err2 = errors.New("err2")
		err3 = errors.New("err3")
	)

	for _, test := range []struct {
		name            string
		setupFn         func(*Manager)
		expectedErr     error
		expectedStopErr []error
		expectedCounts  map[testCount]int
	}{
		{
			name:        "return error from s1 setup step",
			expectedErr: err2,
			expectedCounts: map[testCount]int{
				testCount{"s1", StepSetup, CallbackStepBefore}:    1,
				testCount{"s1", StepSetup, CallbackStepAfter}:     1,
				testCount{"s1", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s1", StepTeardown, CallbackStepAfter}:  1,
				testCount{"s2", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s2", StepTeardown, CallbackStepAfter}:  1,
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithSetup(func(ctx context.Context) error {
						return err2
					}),
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second,
							withSleepContextError(true))
					}),
					WithStop(func(ctx context.Context) error {
						return nil
					}),
					WithTeardown(func(ctx context.Context) error {
						return nil
					}),
				)))
				m.AddTask("s2", newTestTask(2, BuildTask(
					WithSetup(func(ctx context.Context) error {
						return nil
					}),
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, 2*time.Second,
							withSleepContextError(true))
					}),
					WithStop(func(ctx context.Context) error {
						return nil
					}),
					WithTeardown(func(ctx context.Context) error {
						return nil
					}),
				)))
			},
		},
		{
			name:        "return error from start step",
			expectedErr: err1,
			expectedCounts: map[testCount]int{
				testCount{"s1", StepStart, CallbackStepBefore}:    1,
				testCount{"s1", StepStart, CallbackStepAfter}:     1,
				testCount{"s1", StepStop, CallbackStepBefore}:     1,
				testCount{"s1", StepStop, CallbackStepAfter}:      1,
				testCount{"s1", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s1", StepTeardown, CallbackStepAfter}:  1,
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second,
							withSleepContextError(true),
							withSleepContextTimeoutError(err1))
					}),
					WithStop(func(ctx context.Context) error {
						return nil
					}),
					WithTeardown(func(ctx context.Context) error {
						return nil
					}),
				)))
			},
		},
		{
			name:            "return error from stop step",
			expectedStopErr: []error{err1},
			expectedCounts: map[testCount]int{
				testCount{"s1", StepStart, CallbackStepBefore}:    1,
				testCount{"s1", StepStart, CallbackStepAfter}:     1,
				testCount{"s1", StepStop, CallbackStepBefore}:     1,
				testCount{"s1", StepStop, CallbackStepAfter}:      1,
				testCount{"s1", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s1", StepTeardown, CallbackStepAfter}:  1,
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(2, BuildTask(
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second,
							withSleepContextError(true))
					}),
					WithStop(func(ctx context.Context) error {
						return err1
					}),
					WithTeardown(func(ctx context.Context) error {
						return nil
					}),
				)))
			},
		},
		{
			name:            "return error from teardown step",
			expectedStopErr: []error{err2},
			expectedCounts: map[testCount]int{
				testCount{"s1", StepSetup, CallbackStepBefore}:    1,
				testCount{"s1", StepSetup, CallbackStepAfter}:     1,
				testCount{"s1", StepStart, CallbackStepBefore}:    1,
				testCount{"s1", StepStart, CallbackStepAfter}:     1,
				testCount{"s1", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s1", StepTeardown, CallbackStepAfter}:  1,
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithSetup(func(ctx context.Context) error {
						return nil
					}),
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second,
							withSleepContextError(true))
					}),
					WithTeardown(func(ctx context.Context) error {
						return err2
					}),
				)))
			},
		},
		{
			name:            "return error from stop and teardown steps",
			expectedStopErr: []error{err2, err3},
			expectedCounts: map[testCount]int{
				testCount{"s1", StepStart, CallbackStepBefore}:    1,
				testCount{"s1", StepStart, CallbackStepAfter}:     1,
				testCount{"s1", StepStop, CallbackStepBefore}:     1,
				testCount{"s1", StepStop, CallbackStepAfter}:      1,
				testCount{"s1", StepTeardown, CallbackStepBefore}: 1,
				testCount{"s1", StepTeardown, CallbackStepAfter}:  1,
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithStart(func(ctx context.Context) error {
						return sleepContext(ctx, time.Second,
							withSleepContextError(true))
					}),
					WithStop(func(ctx context.Context) error {
						return err2
					}),
					WithTeardown(func(ctx context.Context) error {
						return err3
					}),
				)))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()

				testcb := &testCallback{}

				sopts := []Option{
					WithStages("s1", "s2"),
					WithTaskCallback(testcb),
					WithLogger(defaultLogger(t.Output()).With("category", test.name)),
				}

				sinit, err := New(sopts...)
				assert.NilError(t, err)

				test.setupFn(sinit)

				err, stopErr := sinit.RunWithStopErrors(ctx)
				if test.expectedErr == nil {
					assert.NilError(t, err)
				} else {
					assert.ErrorIs(t, err, test.expectedErr)
				}
				for _, serr := range test.expectedStopErr {
					assert.ErrorIs(t, stopErr, serr)
				}
				assert.DeepEqual(t, test.expectedCounts, testcb.counts)
			})
		})
	}
}

type testTask struct {
	taskNo int
	task   Task
}

var _ TaskSteps = (*testTask)(nil)

func newTestTask(taskNo int, task Task) *testTask {
	return &testTask{
		taskNo: taskNo,
		task:   task,
	}
}

func (t *testTask) TaskNo() int {
	return t.taskNo
}

func (t *testTask) TaskSteps() []Step {
	if tt, ok := t.task.(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return allSteps
}

func (t *testTask) Run(ctx context.Context, step Step) error {
	return t.task.Run(ctx, step)
}

func getTestTaskNo(task Task) (int, bool) {
	if tt, ok := task.(*testTask); ok {
		return tt.TaskNo(), true
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
