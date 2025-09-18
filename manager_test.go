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
		assert.Assert(t, false, "unexpected error type %T", err)
	}
}

func TestManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := &testList[string]{}
		stopped := &testList[string]{}
		prestopped := &testList[string]{}

		sinit := New(t.Context())

		sinit.
			StartTask(TaskFunc(func(ctx context.Context) error {
				started.add("task1")
				return nil
			})).
			PreStop(TaskFunc(func(ctx context.Context) error {
				prestopped.add("task1")
				return nil
			})).
			AutoStop(TaskFunc(func(ctx context.Context) error {
				stopped.add("task1")
				return nil
			}))

		sinit.
			StartService(ServiceFunc(func(ctx context.Context, stage Stage) error {
				switch stage {
				case StageStart:
					started.add("service1")
				case StagePreStop:
					prestopped.add("service1")
				case StageStop:
					stopped.add("service1")
				}
				return nil
			})).
			AutoStop()

		sinit.Shutdown()
		err := sinit.Run()
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"task1", "service1"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task1", "service1"}, prestopped.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task1", "service1"}, stopped.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestManagerWorkflows(t *testing.T) {
	isDebug := false

	for _, test := range []struct {
		name                                           string
		cancelFn                                       func() []int
		expectedTaskErr                                int
		expectedErr                                    error
		expectedOrderedFinish, expectedOrderedStop     []int
		expectedUnorderedFinish, expectedUnorderedStop []int
		expectedUnorderedPreStop                       []int
	}{
		{
			name: "stop 1: cancel execute unordered",
			cancelFn: func() []int {
				return []int{1}
			},
			expectedTaskErr:          1,
			expectedOrderedFinish:    []int{2, 3, 4},
			expectedOrderedStop:      []int{2, 3, 4},
			expectedUnorderedPreStop: []int{3, 4},
			expectedUnorderedFinish:  []int{1, 5},
		},
		{
			name: "stop 2: cancel task ordered",
			cancelFn: func() []int {
				return []int{2}
			},
			expectedTaskErr:          2,
			expectedOrderedFinish:    []int{2, 3, 4},
			expectedOrderedStop:      []int{2, 3, 4},
			expectedUnorderedPreStop: []int{3, 4},
			expectedUnorderedFinish:  []int{1, 5},
		},
		{
			name: "stop 3: cancel task context ordered",
			cancelFn: func() []int {
				return []int{3}
			},
			expectedTaskErr:          3,
			expectedOrderedFinish:    []int{3, 2, 4},
			expectedOrderedStop:      []int{2, 3, 4},
			expectedUnorderedPreStop: []int{3, 4},
			expectedUnorderedFinish:  []int{1, 5},
		},
		{
			name: "stop 4: cancel service ordered",
			cancelFn: func() []int {
				return []int{4}
			},
			expectedTaskErr:          4,
			expectedOrderedFinish:    []int{4, 2, 3},
			expectedOrderedStop:      []int{2, 3, 4},
			expectedUnorderedPreStop: []int{3, 4},
			expectedUnorderedFinish:  []int{1, 5},
		},
		{
			name: "stop 5: cancel task auto",
			cancelFn: func() []int {
				return []int{5}
			},
			expectedTaskErr:          5,
			expectedOrderedFinish:    []int{2, 3, 4},
			expectedOrderedStop:      []int{2, 3, 4},
			expectedUnorderedPreStop: []int{3, 4},
			expectedUnorderedFinish:  []int{1, 5},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			orderedFinish := &testList[int]{}
			orderedStop := &testList[int]{}
			unorderedFinish := &testList[int]{}
			unorderedStop := &testList[int]{}
			unorderedPreStop := &testList[int]{}

			type testService struct {
				svc    Service
				cancel context.CancelFunc
			}

			defaultTaskSvc := func(taskNo int, ordered bool) testService {
				dtCtx, dtCancel := context.WithCancelCause(ctx)
				stCtx, stCancel := context.WithCancel(ctx)
				return testService{
					cancel: func() {
						dtCancel(testTaskNoError{taskNo: taskNo})
					},
					svc: ServiceFunc(func(ctx context.Context, stage Stage) error {
						switch stage {
						case StageStart:
							defer stCancel()

							if isDebug {
								fmt.Printf("Task %d running\n", taskNo)
								defer func() {
									fmt.Printf("Task %d finished\n", taskNo)
								}()
							}

							defer func() {
								if ordered {
									orderedFinish.add(taskNo)
								} else {
									unorderedFinish.add(taskNo)
								}
							}()

							select {
							case <-dtCtx.Done():
								if isDebug {
									fmt.Printf("Task %d dtCtx done [err:%v]\n", taskNo, context.Cause(dtCtx))
								}
								return context.Cause(dtCtx)
							case <-ctx.Done():
								if isDebug {
									fmt.Printf("Task %d ctx done [err:%v]\n", taskNo, context.Cause(ctx))
								}
								return context.Cause(ctx)
							}
						case StagePreStop:
							if isDebug {
								fmt.Printf("PreStopping task %d\n", taskNo)
							}
							defer func() {
								unorderedPreStop.add(taskNo)
							}()
						case StageStop:
							if isDebug {
								fmt.Printf("Stopping task %d\n", taskNo)
							}
							defer func() {
								if ordered {
									orderedStop.add(taskNo)
								} else {
									unorderedStop.add(taskNo)
								}
							}()
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

			sinit := New(ctx)

			task1 := defaultTaskSvc(1, false)
			sinit.ExecuteTask(ServiceAsTask(task1.svc, StageStart))

			task2 := defaultTaskSvc(2, true)
			i2Stop := sinit.
				StartTask(ServiceAsTask(task2.svc, StageStart)).
				FutureStop(ServiceAsTask(task2.svc, StageStop))

			task3 := defaultTaskSvc(3, true)
			i3Stop := sinit.
				StartTask(ServiceAsTask(task3.svc, StageStart)).
				PreStop(ServiceAsTask(task3.svc, StagePreStop)).
				FutureStop(ServiceAsTask(task3.svc, StageStop), WithCancelContext(true))

			task4 := defaultTaskSvc(4, true)
			i4Stop := sinit.
				StartService(task4.svc).
				FutureStop()

			task5 := defaultTaskSvc(5, false)
			sinit.
				StartTask(ServiceAsTask(task5.svc, StageStart)).
				AutoStopContext()

			sinit.ExecuteTask(SignalTask(os.Interrupt, syscall.SIGTERM))

			tasks := []testService{task1, task2, task3, task4, task5}

			sinit.SetOptions(
				WithManagerCallback(ManagerCallbackFunc(func(ctx context.Context, stage Stage, step Step, cause error) error {
					if stage != StageStart || step != StepAfter {
						return nil
					}
					for _, taskNo := range test.cancelFn() {
						tasks[taskNo-1].cancel()
					}
					return nil
				})),
			)

			sinit.StopFuture(i2Stop)
			sinit.StopFuture(i3Stop)
			sinit.StopFuture(i4Stop)

			err := sinit.Run()
			if test.expectedErr != nil {
				assert.ErrorIs(t, err, test.expectedErr)
			} else if test.expectedTaskErr > 0 {
				checkTestTaskError(t, err, test.expectedTaskErr)
			} else {
				assert.NilError(t, err)
			}

			assert.DeepEqual(t, test.expectedOrderedFinish, orderedFinish.get())
			assert.DeepEqual(t, test.expectedOrderedStop, orderedStop.get())
			assert.DeepEqual(t, test.expectedUnorderedFinish, unorderedFinish.get(), cmpopts.SortSlices(cmp.Less[int]))
			assert.DeepEqual(t, test.expectedUnorderedStop, unorderedStop.get(), cmpopts.SortSlices(cmp.Less[int]))
			assert.DeepEqual(t, test.expectedUnorderedPreStop, unorderedPreStop.get(), cmpopts.SortSlices(cmp.Less[int]))
		})
	}
}

func TestManagerStopMultipleTasks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit := New(t.Context())

		started := &testList[int]{}
		stopped := &testList[int]{}

		stopTask1 := sinit.
			StartTask(TaskFunc(func(ctx context.Context) error {
				started.add(1)
				return nil
			})).
			FutureStop(TaskFunc(func(ctx context.Context) error {
				stopped.add(1)
				return nil
			}))

		stopTask2 := sinit.
			StartTask(TaskFunc(func(ctx context.Context) error {
				started.add(2)
				return nil
			})).
			FutureStop(TaskFunc(func(ctx context.Context) error {
				stopped.add(2)
				return nil
			}))

		sinit.
			StopFutureMultiple(stopTask1, stopTask2)

		err := sinit.Run()

		assert.NilError(t, err)

		assert.DeepEqual(t, []int{1, 2}, started.get(), cmpopts.SortSlices(cmp.Less[int]))
		assert.DeepEqual(t, []int{1, 2}, stopped.get(), cmpopts.SortSlices(cmp.Less[int]))
	})
}

func TestManagerOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runStarted, runStopped atomic.Int32
		testcb := &testCallback{}
		testtaskcb := &testCallback{}
		testruncb := &testCallback{}

		sinit := New(t.Context(),
			WithManagerCallback(ManagerCallbackFunc(func(ctx context.Context, stage Stage, step Step, cause error) error {
				switch stage {
				case StageStart:
					runStarted.Add(1)
				case StageStop:
					runStopped.Add(1)
				default:
				}
				return nil
			})),
			WithGlobalTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage Stage, step Step, err error) {
				assertTestTask(t, task, stage, step)
			})),
			WithGlobalTaskCallback(testcb),
		)

		stopTask1 := sinit.
			StartTask(newTestTask(1, func(ctx context.Context) error {
				testruncb.add(1, StageStart, StepBefore, nil)
				defer testruncb.add(1, StageStart, StepAfter, nil)
				_ = SleepContext(ctx, 2*time.Second)
				return nil
			}), WithTaskCallback(testtaskcb, WaitStartTaskCallback())).
			FutureStop(newTestTask(1, func(ctx context.Context) error {
				testruncb.add(1, StageStop, StepBefore, nil)
				defer testruncb.add(1, StageStop, StepAfter, nil)
				return nil
			}), WithCancelContext(true))

		stopTask2 := sinit.
			StartTask(newTestTask(2, func(ctx context.Context) error {
				testruncb.add(2, StageStart, StepBefore, nil)
				defer testruncb.add(2, StageStart, StepAfter, nil)
				_ = SleepContext(ctx, time.Second)
				return nil
			}), WithTaskCallback(testtaskcb, WaitStartTaskCallback())).
			PreStop(newTestTask(2, func(ctx context.Context) error {
				testruncb.add(2, StagePreStop, StepBefore, nil)
				defer testruncb.add(2, StagePreStop, StepAfter, nil)
				return nil
			})).
			FutureStop(newTestTask(2, func(ctx context.Context) error {
				testruncb.add(2, StageStop, StepBefore, nil)
				defer testruncb.add(2, StageStop, StepAfter, nil)
				return nil
			}), WithCancelContext(true))

		svc := newTestService(3, func(ctx context.Context, stage Stage) error {
			switch stage {
			case StageStart:
				testruncb.add(3, StageStart, StepBefore, nil)
				defer testruncb.add(3, StageStart, StepAfter, nil)
				_ = SleepContext(ctx, 2*time.Second)
			case StagePreStop:
				testruncb.add(3, StagePreStop, StepBefore, nil)
				defer testruncb.add(3, StagePreStop, StepAfter, nil)
			case StageStop:
				testruncb.add(3, StageStop, StepBefore, nil)
				defer testruncb.add(3, StageStop, StepAfter, nil)
			default:
			}
			return nil
		})

		stopService := sinit.
			StartService(svc, WithTaskCallback(testtaskcb, WaitStartTaskCallback())).
			FutureStopContext()

		sinit.StopFuture(stopTask1)
		sinit.StopFuture(stopTask2)
		sinit.StopFuture(stopService)

		err := sinit.Run()
		assert.NilError(t, err)

		expected := map[int][]testCallbackItem{
			1: {
				{1, StageStart, StepBefore, nil},
				{1, StageStop, StepBefore, nil},
				{1, StageStop, StepAfter, nil},
				{1, StageStart, StepAfter, nil},
			},
			2: {
				{2, StageStart, StepBefore, nil},
				{2, StageStart, StepAfter, nil},
				{2, StagePreStop, StepBefore, nil},
				{2, StagePreStop, StepAfter, nil},
				{2, StageStop, StepBefore, nil},
				{2, StageStop, StepAfter, nil},
			},
			3: {
				{3, StageStart, StepBefore, nil},
				{3, StagePreStop, StepBefore, nil},
				{3, StagePreStop, StepAfter, nil},
				{3, StageStop, StepBefore, nil},
				{3, StageStop, StepAfter, nil},
				{3, StageStart, StepAfter, nil},
			},
		}

		assert.Equal(t, int32(2), runStarted.Load())
		assert.Equal(t, int32(2), runStopped.Load())
		testcb.m.Lock()
		defer testcb.m.Unlock()
		testtaskcb.m.Lock()
		defer testtaskcb.m.Unlock()
		testruncb.m.Lock()
		defer testruncb.m.Unlock()
		assert.DeepEqual(t, expected, testcb.allTestTasksByNo)
		assert.DeepEqual(t, expected, testtaskcb.allTestTasksByNo)
		assert.DeepEqual(t, expected, testruncb.allTestTasksByNo)
	})
}

func TestManagerWithoutTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit := New(t.Context())
		err := sinit.Run()
		assert.ErrorIs(t, err, ErrNoTask)
	})
}

func TestManagerShutdownOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		shutdownCtx := context.WithValue(t.Context(), "test-shutdown", 5)

		sinit := New(t.Context(),
			WithShutdownContext(shutdownCtx))

		sinit.
			StartTask(TaskFunc(func(ctx context.Context) error {
				assert.Check(t, !cmp2.Equal(5, ctx.Value("test-shutdown")), "not expected context value")
				return nil
			})).
			AutoStop(TaskFunc(func(ctx context.Context) error {
				assert.Check(t, cmp2.Equal(5, ctx.Value("test-shutdown")), "expected context value is different")
				return nil
			}))

		sinit.Shutdown()
		err := sinit.Run()
		assert.NilError(t, err)
	})
}

func TestManagerTaskWithID(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := &testList[string]{}
		stopped := &testList[string]{}

		sinit := New(t.Context(),
			WithGlobalTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage Stage, step Step, err error) {
				if step != StepBefore {
					return
				}
				if tid, ok := task.(TaskWithID); ok {
					if tstr, ok := tid.TaskID().(string); ok {
						switch stage {
						case StageStart:
							started.add(tstr)
						case StageStop:
							stopped.add(tstr)
						default:
						}
					}
				}
			})),
		)

		sinit.
			StartTask(TaskFuncWithID("t1start", func(ctx context.Context) error {
				return nil
			})).
			AutoStop(TaskFuncWithID("t1stop", func(ctx context.Context) error {
				return nil
			}))

		sinit.
			StartService(ServiceFuncWithID("s1", func(ctx context.Context, stage Stage) error {
				return nil
			})).
			AutoStop()

		sinit.Shutdown()
		err := sinit.Run()
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"s1", "t1start"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"s1", "t1stop"}, stopped.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestManagerShutdownTimeout(t *testing.T) {
	timeoutTest := func(taskStopSleep time.Duration, isError bool) {
		synctest.Test(t, func(t *testing.T) {
			sinit := New(t.Context(),
				WithShutdownTimeout(30*time.Second))

			sinit.
				StartTask(TaskFunc(func(ctx context.Context) error {
					return nil
				})).
				AutoStop(TaskFunc(func(ctx context.Context) error {
					return SleepContext(ctx, taskStopSleep)
				}))

			err, cleanupErr := sinit.RunWithErrors()
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

func TestManagerRunMultiple(t *testing.T) {
	sinit := New(context.Background())

	// must call one StartTaskCmd method
	sinit.ExecuteTask(TaskFunc(func(ctx context.Context) error {
		return nil
	}))

	err := sinit.Run()
	assert.NilError(t, err)
	err = sinit.Run()
	assert.ErrorIs(t, err, ErrAlreadyRunning)
}

func TestManagerNullTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sinit := New(t.Context())

		stopTask1 := sinit.
			StartTask(nil, WithTaskCallback(WaitStartTaskCallback())).
			PreStop(nil).
			FutureStop(nil, WithCancelContext(true))

		stopTask2 := sinit.
			StartTask(newTestTask(2, func(ctx context.Context) error {
				_ = SleepContext(ctx, time.Second)
				return nil
			}), WithTaskCallback(WaitStartTaskCallback())).
			FutureStop(nil, WithCancelContext(true))

		stopTask3 := sinit.
			StartTask(newTestTask(3, func(ctx context.Context) error {
				_ = SleepContext(ctx, time.Second)
				return nil
			}), WithTaskCallback(WaitStartTaskCallback())).
			PreStop(newTestTask(3, func(ctx context.Context) error {
				return nil
			})).
			FutureStop(newTestTask(3, func(ctx context.Context) error {
				return nil
			}), WithCancelContext(true))

		stopService := sinit.
			StartService(nil, WithTaskCallback(WaitStartTaskCallback())).
			FutureStopContext()

		sinit.StopFuture(stopTask1)
		sinit.StopFuture(stopTask2)
		sinit.StopFuture(stopTask3)
		sinit.StopFuture(stopService)

		err := sinit.Run()
		assert.ErrorIs(t, err, ErrNilTasks)
	})
}

func TestManagerPendingStart(t *testing.T) {
	sinit := New(context.Background())

	// must call one StartTaskCmd method
	_ = sinit.StartTask(TaskFunc(func(ctx context.Context) error {
		return nil
	}))

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestManagerPendingStartService(t *testing.T) {
	sinit := New(context.Background())

	// must call one StartServiceCmd method
	_ = sinit.StartService(ServiceFunc(func(ctx context.Context, stage Stage) error {
		return nil
	}))

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestManagerPendingStop(t *testing.T) {
	sinit := New(context.Background())

	// must add stop function to FutureStop
	_ = sinit.StartTask(TaskFunc(func(ctx context.Context) error {
		return nil
	})).FutureStop(TaskFunc(func(ctx context.Context) error {
		return nil
	}))

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestManagerPendingStopService(t *testing.T) {
	sinit := New(context.Background())

	// must add stop function to FutureStop
	_ = sinit.StartService(ServiceFunc(func(ctx context.Context, stage Stage) error {
		return nil
	})).FutureStop()

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

type testTask struct {
	taskNo int
	task   TaskFunc
}

func newTestTask(taskNo int, task TaskFunc) *testTask {
	return &testTask{
		taskNo: taskNo,
		task:   task,
	}
}

func (t *testTask) TaskNo() int {
	return t.taskNo
}

func (t *testTask) Run(ctx context.Context) error {
	return t.task(ctx)
}

type testService struct {
	taskNo  int
	handler func(ctx context.Context, stage Stage) error
}

func newTestService(taskNo int, handler func(ctx context.Context, stage Stage) error) *testService {
	return &testService{
		taskNo:  taskNo,
		handler: handler,
	}
}

func (t *testService) TaskNo() int {
	return t.taskNo
}

func (t *testService) RunService(ctx context.Context, stage Stage) error {
	return t.handler(ctx, stage)
}

func getTestTaskNo(task Task) (int, bool) {
	if st, ok := task.(ServiceTask); ok {
		if ts, ok := st.Service().(*testService); ok {
			return ts.TaskNo(), true
		}
	} else if tt, ok := task.(*testTask); ok {
		return tt.TaskNo(), true
	}
	return -1, false
}

func assertTestTask(t *testing.T, task Task, stage Stage, step Step) bool {
	if st, ok := task.(ServiceTask); ok {
		if _, ok := st.Service().(*testService); !ok {
			return assert.Check(t, false, "service is not of the expected type but %T (stage:%s)(step:%s)",
				task, stage, step)
		}
	} else if _, ok := task.(*testTask); !ok {
		return assert.Check(t, false, "task is not of the expected type but %T (stage:%s)(step:%s)",
			task, stage, step)
	}
	return true
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

type testCallbackItem struct {
	taskNo int
	stage  Stage
	step   Step
	err    error
}

func (t testCallbackItem) Equal(other testCallbackItem) bool {
	if t.taskNo != other.taskNo || t.stage != other.stage || t.step != other.step {
		return false
	}
	if t.err == nil && other.err == nil {
		return true
	}
	return errors.Is(other.err, t.err)
}

func testCallbackItemCompare(a, b testCallbackItem) int {
	if c := cmp.Compare(a.taskNo, b.taskNo); c != 0 {
		return c
	}
	if c := cmp.Compare(a.stage, b.stage); c != 0 {
		return c
	}
	if c := cmp.Compare(a.step, b.step); c != 0 {
		return c
	}
	return 0
}

type testCount struct {
	stage Stage
	step  Step
}

type testCallback struct {
	m                sync.Mutex
	counts           map[testCount]int
	allTestTasks     []testCallbackItem
	allTestTasksByNo map[int][]testCallbackItem
}

func (t *testCallback) add(taskNo int, stage Stage, step Step, err error) {
	t.m.Lock()
	if t.counts == nil {
		t.counts = make(map[testCount]int)
	}
	t.counts[testCount{stage: stage, step: step}]++
	t.m.Unlock()

	if taskNo < 0 {
		return
	}
	t.m.Lock()
	defer t.m.Unlock()
	item := testCallbackItem{
		taskNo: taskNo,
		stage:  stage,
		step:   step,
		err:    err,
	}
	if t.allTestTasks == nil {
		t.allTestTasksByNo = make(map[int][]testCallbackItem)
	}
	t.allTestTasks = append(t.allTestTasks, item)
	t.allTestTasksByNo[taskNo] = append(t.allTestTasksByNo[taskNo], item)
}

func (t *testCallback) Callback(_ context.Context, task Task, stage Stage, step Step, err error) {
	taskNo, ok := getTestTaskNo(task)
	if !ok {
		taskNo = -1
	}
	t.add(taskNo, stage, step, err)
}
