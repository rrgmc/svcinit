package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
	cmp3 "gotest.tools/v3/assert/cmp"
)

func TestManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		sm, err := New(
			WithStages("health", "service"),
		)

		sm.AddTask("health", BuildTask(
			WithStart(func(ctx context.Context) error {
				items.add("start")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				items.add("stop")
				return nil
			}),
		))
		assert.NilError(t, err)

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		items.assertDeepEqual(t, []string{"start", "stop"})
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
		items := &testList[string]{}

		shutdownCtx := context.WithValue(t.Context(), "test-shutdown", 5)

		sinit, err := New()
		assert.NilError(t, err)

		sinit.AddTask(StageDefault, BuildTask(
			WithStart(func(ctx context.Context) error {
				items.add("start")
				assert.Check(t, !cmp2.Equal(5, ctx.Value("test-shutdown")), "not expected context value")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				items.add("stop")
				assert.Check(t, cmp2.Equal(5, ctx.Value("test-shutdown")), "expected context value is different")
				return nil
			}),
		))

		// sinit.Shutdown()
		err = sinit.Run(t.Context(), WithRunShutdownContext(shutdownCtx))
		assert.NilError(t, err)
		items.assertDeepEqual(t, []string{"start", "stop"})
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
		items := &testList[string]{}

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
					items.add("start")
					return nil
				}),
				WithStop(func(ctx context.Context) error {
					items.add("stop")
					assert.Check(t, ctx.Err() == nil)
					return nil
				}),
			))

		time.AfterFunc(10*time.Second, func() {
			mainCancel()
		})

		err = sinit.Run(mainCtx)
		assert.ErrorIs(t, err, context.Canceled)
		items.assertDeepEqual(t, []string{"start", "stop"})
	})
}

func TestManagerTaskHandler(t *testing.T) {
	var (
		err1 = errors.New("err1")
	)

	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		sinit, err := New()
		assert.NilError(t, err)

		sinit.
			AddTask(StageDefault, BuildTask(
				WithStart(func(ctx context.Context) error {
					items.add("start")
					return sleepContext(ctx, time.Second,
						withSleepContextTimeoutError(err1))
				}),
				WithStop(func(ctx context.Context) error {
					items.add("stop")
					return nil
				}),
			), WithHandler(func(ctx context.Context, task Task, step Step) error {
				switch step {
				case StepStart:
					items.add("handler_start")
					return task.Run(ctx, step)
				case StepStop:
					items.add("handler_stop")
					return task.Run(ctx, step)
				default:
				}
				return nil
			}))

		err = sinit.Run(t.Context())
		assert.ErrorIs(t, err, err1)
		items.assertDeepEqual(t, []string{"start", "stop", "handler_start", "handler_stop"})
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
			WithDataName[*idata1]("idata1"))
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
			WithDataName[*idata2]("idata2"))

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

		items.assertDeepEqual(t, []string{"i1setup", "i2setup", "sstart"})
	})
}

func TestManagerSetupErrorReturnsEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var (
			err1 = errors.New("err1")
		)

		testcb := &testCallback{
			filterCallbackStep: ptr(CallbackStepAfter),
		}

		sm, err := New(
			WithStages("s1", "s2", "s3", "s4", "s5", "s6"),
			WithTaskCallback(testcb),
		)

		sm.AddTask("s1", BuildTask(
			WithSetup(testEmptyStep),
			WithStart(testDefaultStart(1)),
			WithStop(testEmptyStep),
			WithTeardown(testEmptyStep),
		))
		sm.AddTask("s2", BuildTask(
			WithStart(testDefaultStart(1)),
			WithStop(testEmptyStep),
		))
		sm.AddTask("s3", BuildTask(
			WithStop(testEmptyStep),
			WithTeardown(testEmptyStep),
		))
		sm.AddTask("s4", BuildTask(
			WithTeardown(testEmptyStep),
		))
		sm.AddTask("s5", BuildTask(
			WithSetup(func(ctx context.Context) error {
				return err1
			}),
			WithStart(testDefaultStart(1)),
			WithStop(testEmptyStep),
			WithTeardown(testEmptyStep),
		))
		sm.AddTask("s6", BuildTask(
			WithSetup(testEmptyStep),
			WithStart(testDefaultStart(1)),
			WithStop(testEmptyStep),
			WithTeardown(testEmptyStep),
		))

		err = sm.Run(t.Context())
		assert.ErrorIs(t, err, ErrInitialization)
		assert.ErrorIs(t, err, err1)

		expectedTasks := []testCallbackItem{
			{0, "s1", StepSetup, CallbackStepAfter, nil},
			{0, "s1", StepStart, CallbackStepAfter, nil},
			{0, "s1", StepStop, CallbackStepAfter, nil},
			{0, "s1", StepTeardown, CallbackStepAfter, nil},

			{0, "s2", StepStart, CallbackStepAfter, nil},
			{0, "s2", StepStop, CallbackStepAfter, nil},

			{0, "s3", StepStop, CallbackStepAfter, nil},
			{0, "s3", StepTeardown, CallbackStepAfter, nil},

			{0, "s4", StepTeardown, CallbackStepAfter, nil},

			{0, "s5", StepSetup, CallbackStepAfter, err1},
		}

		notExpectedTasks := []testCallbackItem{
			{0, "s2", StepSetup, CallbackStepAfter, nil},
			{0, "s2", StepTeardown, CallbackStepAfter, nil},

			{0, "s3", StepSetup, CallbackStepAfter, nil},
			{0, "s3", StepStart, CallbackStepAfter, err1},

			{0, "s4", StepSetup, CallbackStepAfter, nil},
			{0, "s4", StepStart, CallbackStepAfter, nil},
			{0, "s4", StepStop, CallbackStepAfter, nil},

			{0, "s5", StepStart, CallbackStepAfter, err1},
			{0, "s5", StepStop, CallbackStepAfter, err1},
			{0, "s5", StepTeardown, CallbackStepAfter, err1},

			{0, "s6", StepSetup, CallbackStepAfter, nil},
			{0, "s6", StepStart, CallbackStepAfter, nil},
			{0, "s6", StepStop, CallbackStepAfter, nil},
			{0, "s6", StepTeardown, CallbackStepAfter, nil},
		}

		testcb.assertExpectedNotExpected(t, expectedTasks, notExpectedTasks)
	})
}

func TestManagerErrorReturns(t *testing.T) {
	var (
		err1 = errors.New("err1")
		err2 = errors.New("err2")
		err3 = errors.New("err3")
	)

	for _, test := range []struct {
		name             string
		setupFn          func(*Manager)
		expectedErr      error
		expectedStopErr  []error
		expectedTasks    []testCallbackItem
		notExpectedTasks []testCallbackItem
	}{
		{
			name:        "return error from s1 setup step",
			expectedErr: err2,
			expectedTasks: []testCallbackItem{
				{1, "s1", StepSetup, CallbackStepAfter, err2},
			},
			notExpectedTasks: []testCallbackItem{
				{1, "s1", StepStart, CallbackStepAfter, nil},
				{1, "s1", StepStop, CallbackStepAfter, nil},
				{1, "s1", StepTeardown, CallbackStepAfter, err2},

				{2, "s2", StepSetup, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepTeardown, CallbackStepAfter, nil},
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithSetup(func(ctx context.Context) error {
						return err2
					}),
					WithStart(testDefaultStart(1)),
					WithStop(testEmptyStep),
					WithTeardown(testEmptyStep),
				)))
			},
		},
		{
			name:        "return error from start step",
			expectedErr: err1,
			expectedTasks: []testCallbackItem{
				{1, "s1", StepStart, CallbackStepAfter, err1},
				{1, "s1", StepStop, CallbackStepAfter, nil},
				{1, "s1", StepTeardown, CallbackStepAfter, nil},

				{2, "s2", StepSetup, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepTeardown, CallbackStepAfter, nil},
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithStart(testDefaultStart(1, withSleepContextTimeoutError(err1))),
					WithStop(testEmptyStep),
					WithTeardown(testEmptyStep),
				)))
			},
		},
		{
			name:            "return error from stop step",
			expectedStopErr: []error{err1},
			expectedTasks: []testCallbackItem{
				{1, "s1", StepStart, CallbackStepAfter, nil},
				{1, "s1", StepStop, CallbackStepAfter, err1},
				{1, "s1", StepTeardown, CallbackStepAfter, nil},

				{2, "s2", StepSetup, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepTeardown, CallbackStepAfter, nil},
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithStart(testDefaultStart(1)),
					WithStop(func(ctx context.Context) error {
						return err1
					}),
					WithTeardown(testEmptyStep),
				)))
			},
		},
		{
			name:            "return error from teardown step",
			expectedStopErr: []error{err2},
			expectedTasks: []testCallbackItem{
				{1, "s1", StepSetup, CallbackStepAfter, nil},
				{1, "s1", StepStart, CallbackStepAfter, nil},
				{1, "s1", StepTeardown, CallbackStepAfter, err2},

				{2, "s2", StepSetup, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepTeardown, CallbackStepAfter, nil},
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithSetup(testEmptyStep),
					WithStart(testDefaultStart(1)),
					WithTeardown(func(ctx context.Context) error {
						return err2
					}),
				)))
			},
		},
		{
			name:            "return error from stop and teardown steps",
			expectedStopErr: []error{err2, err3},
			expectedTasks: []testCallbackItem{
				{1, "s1", StepStart, CallbackStepAfter, nil},
				{1, "s1", StepStop, CallbackStepAfter, err2},
				{1, "s1", StepTeardown, CallbackStepAfter, err3},

				{2, "s2", StepSetup, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepTeardown, CallbackStepAfter, nil},
			},
			setupFn: func(m *Manager) {
				m.AddTask("s1", newTestTask(1, BuildTask(
					WithStart(testDefaultStart(1)),
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

				testcb := &testCallback{
					filterCallbackStep: ptr(CallbackStepAfter),
				}

				sopts := []Option{
					WithStages("s1", "s2"),
					WithTaskCallback(testcb),
					// WithLogger(defaultLogger(t.Output()).With("category", test.name)),
				}

				sinit, err := New(sopts...)
				assert.NilError(t, err)

				test.setupFn(sinit)

				sinit.AddTask("s2", newTestTask(2, BuildTask(
					WithSetup(testEmptyStep),
					WithStart(testDefaultStart(2)),
					WithStop(testEmptyStep),
					WithTeardown(testEmptyStep),
				)))

				err, stopErr := sinit.RunWithStopErrors(ctx)
				if test.expectedErr == nil {
					assert.NilError(t, err)
				} else {
					assert.ErrorIs(t, err, test.expectedErr)
				}
				for _, serr := range test.expectedStopErr {
					assert.ErrorIs(t, stopErr, serr)
				}
				testcb.assertExpectedNotExpected(t, test.expectedTasks, test.notExpectedTasks)
			})
		})
	}
}

func TestManagerSSM(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		err1 := errors.New("err1")
		err2 := errors.New("err2")
		err3 := errors.New("err3")
		err4 := errors.New("err4")

		sm, err := New()

		sm.AddTask(StageDefault, BuildTask(
			WithStart(func(ctx context.Context) error {
				items.add("start")
				select {
				case <-ctx.Done():
					assert.Check(t, errors.Is(context.Cause(ctx), err3))
					return err4
				}
			}),
			WithStop(func(ctx context.Context) error {
				items.add("stop")
				ssm := StartStepManagerFromContext(ctx)
				ssm.ContextCancel(err3)
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-ssm.Finished():
					assert.Check(t, errors.Is(ssm.FinishedErr(), err4))
					return err1
				}
			}),
		), WithStartStepManager())
		assert.NilError(t, err)

		sm.AddTask(StageDefault, TimeoutTask(time.Second, WithTimeoutTaskError(err2)))

		err, stopErr := sm.RunWithStopErrors(t.Context())
		assert.ErrorIs(t, err, err2)
		assert.ErrorIs(t, stopErr, err1)
		assert.DeepEqual(t, []string{"start", "stop"}, items.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestManagerSSMNotBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		err1 := errors.New("err1")
		err2 := errors.New("err2")

		sm, err := New()

		sm.AddTask(StageDefault, BuildTask(
			WithStart(func(ctx context.Context) error {
				items.add("start")
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(time.Second):
					return err2
				}
			}),
			WithStop(func(ctx context.Context) error {
				items.add("stop")
				ssm := StartStepManagerFromContext(ctx)
				ssm.ContextCancel(context.Canceled)
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-ssm.Finished():
					// finished returns a closed channel if "ssm.CanFinished" is false, otherwise the step
					// would be deadlocked.
					assert.Check(t, ssm.FinishedErr() == nil)
					return err1
				}
			}),
		) /* WithStartStepManager(), // not adding the parameter */)
		assert.NilError(t, err)

		err, stopErr := sm.RunWithStopErrors(t.Context())
		assert.ErrorIs(t, err, err2)
		assert.ErrorIs(t, stopErr, err1)
		assert.DeepEqual(t, []string{"start", "stop"}, items.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func testEmptyStep(ctx context.Context) error {
	return nil
}

func testDefaultStart(delay int, options ...sleepContextOption) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return sleepContext(ctx, time.Duration(delay)*time.Second,
			append(options, withSleepContextError(true))...)
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

func (l *testList[T]) assertDeepEqual(t *testing.T, expected []T) {
	assert.DeepEqual(t, expected, l.get(), cmpopts.SortSlices(cmp.Less[string]))
}

func (l *testList[T]) checkDeepEqual(t *testing.T, expected []T) bool {
	return assert.Check(t, cmp3.DeepEqual(expected, l.get(), cmpopts.SortSlices(cmp.Less[string])))
}
