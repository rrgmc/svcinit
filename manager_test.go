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
	isDebug := false

	for _, test := range []struct {
		name                                           string
		cancelFn                                       func() []int
		expectedTaskErr                                int
		expectedErr                                    error
		expectedOrderedFinish, expectedOrderedStop     []int
		expectedUnorderedFinish, expectedUnorderedStop []int
	}{
		{
			name: "stop 1: cancel execute unordered",
			cancelFn: func() []int {
				return []int{1}
			},
			expectedTaskErr:         1,
			expectedOrderedFinish:   []int{2, 3, 4},
			expectedOrderedStop:     []int{2, 3, 4},
			expectedUnorderedFinish: []int{1, 5},
		},
		{
			name: "stop 2: cancel task ordered",
			cancelFn: func() []int {
				return []int{2}
			},
			expectedTaskErr:         2,
			expectedOrderedFinish:   []int{2, 3, 4},
			expectedOrderedStop:     []int{2, 3, 4},
			expectedUnorderedFinish: []int{1, 5},
		},
		{
			name: "stop 3: cancel task context ordered",
			cancelFn: func() []int {
				return []int{3}
			},
			expectedTaskErr:         3,
			expectedOrderedFinish:   []int{3, 2, 4},
			expectedOrderedStop:     []int{2, 3, 4},
			expectedUnorderedFinish: []int{1, 5},
		},
		{
			name: "stop 4: cancel service ordered",
			cancelFn: func() []int {
				return []int{4}
			},
			expectedTaskErr:         4,
			expectedOrderedFinish:   []int{4, 2, 3},
			expectedOrderedStop:     []int{2, 3, 4},
			expectedUnorderedFinish: []int{1, 5},
		},
		{
			name: "stop 5: cancel task auto",
			cancelFn: func() []int {
				return []int{5}
			},
			expectedTaskErr:         5,
			expectedOrderedFinish:   []int{2, 3, 4},
			expectedOrderedStop:     []int{2, 3, 4},
			expectedUnorderedFinish: []int{1, 5},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			orderedFinish := &testList[int]{}
			orderedStop := &testList[int]{}
			unorderedFinish := &testList[int]{}
			unorderedStop := &testList[int]{}

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
					svc: ServiceFunc(
						func(ctx context.Context) error {
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
						}, func(ctx context.Context) error {
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
							return nil
						}),
				}
			}

			sinit := New(ctx)

			task1 := defaultTaskSvc(1, false)
			sinit.ExecuteTask(ServiceAsTask(task1.svc, true))

			task2 := defaultTaskSvc(2, true)
			i2Stop := sinit.
				StartTask(ServiceAsTask(task2.svc, true)).
				FutureStop(ServiceAsTask(task2.svc, false))

			task3 := defaultTaskSvc(3, true)
			i3Stop := sinit.
				StartTask(ServiceAsTask(task3.svc, true)).
				FutureStop(ServiceAsTask(task3.svc, false), WithCancelContext(true))

			task4 := defaultTaskSvc(4, true)
			i4Stop := sinit.
				StartService(task4.svc).
				FutureStop()

			task5 := defaultTaskSvc(5, false)
			sinit.
				StartTask(ServiceAsTask(task5.svc, true)).
				AutoStopContext()

			sinit.ExecuteTask(SignalTask(os.Interrupt, syscall.SIGTERM))

			tasks := []testService{task1, task2, task3, task4, task5}

			sinit.SetOptions(
				WithStartedCallback(func(ctx context.Context) error {
					for _, taskNo := range test.cancelFn() {
						tasks[taskNo-1].cancel()
					}
					return nil
				}),
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
		})
	}
}

func TestManagerStopMultipleTasks(t *testing.T) {
	sinit := New(context.Background())

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
}

func TestManagerCallback(t *testing.T) {
	var runStarted, runStopped atomic.Int32
	started := &testList[int]{}
	stopped := &testList[int]{}

	globalTaskCallback := func(ctx context.Context, task Task) {
		if st, ok := task.(ServiceTask); ok {
			if _, ok := st.Service().(*testService); !ok {
				assert.Check(t, false, "service is not of the expected type")
			}
		} else if _, ok := task.(*testTask); !ok {
			assert.Check(t, false, "task is not of the expected type")
		}
	}

	individualTaskCallback := func(taskNo int, isStart bool, isBefore bool) func(ctx context.Context, task Task) {
		return func(ctx context.Context, task Task) {
			stdAdd := 0
			if st, ok := task.(ServiceTask); ok {
				if _, ok := st.Service().(*testService); !ok {
					assert.Check(t, false, "service is not of the expected type but %T", task)
				}
				isStart = st.IsStart()
				stdAdd = 4
			} else if _, ok := task.(*testTask); !ok {
				assert.Check(t, false, "task %d (isStart:%t)(isBefore:%t) is not of the expected type but %T",
					taskNo, isStart, isBefore, task)
			}
			if !isBefore {
				stdAdd++
			}
			if !isStart {
				stdAdd += 2
			}

			if isStart {
				started.add((taskNo * 10) + stdAdd)
			} else {
				stopped.add((taskNo * 10) + stdAdd)
			}
		}
	}

	sinit := New(context.Background(),
		WithStartingCallback(func(ctx context.Context) error {
			runStarted.Add(1)
			return nil
		}),
		WithStartedCallback(func(ctx context.Context) error {
			runStarted.Add(1)
			return nil
		}),
		WithStoppingCallback(func(ctx context.Context, cause error) error {
			runStopped.Add(1)
			return nil
		}),
		WithStoppedCallback(func(ctx context.Context, cause error) error {
			runStopped.Add(1)
			return nil
		}),
		WithGlobalTaskCallback(
			TaskCallbackFunc(func(ctx context.Context, task Task, isStart bool) {
				globalTaskCallback(ctx, task)
			}, func(ctx context.Context, task Task, isStart bool, err error) {
				globalTaskCallback(ctx, task)
			})),
	)

	getTaskCallback := func(taskNo int) TaskCallback {
		return TaskCallbackFunc(func(ctx context.Context, task Task, isStart bool) {
			individualTaskCallback(taskNo, isStart, true)(ctx, task)
		}, func(ctx context.Context, task Task, isStart bool, err error) {
			individualTaskCallback(taskNo, isStart, false)(ctx, task)
		})
	}

	stopTask1 := sinit.
		StartTask(newTestTask(1, func(ctx context.Context) error {
			started.add(1)
			return nil
		}), WithTaskCallback(getTaskCallback(1))).
		FutureStop(newTestTask(1, func(ctx context.Context) error {
			stopped.add(1)
			return nil
		}))

	stopTask2 := sinit.
		StartTask(newTestTask(2, func(ctx context.Context) error {
			started.add(2)
			return nil
		}), WithTaskCallback(getTaskCallback(2))).
		FutureStop(newTestTask(2, func(ctx context.Context) error {
			stopped.add(2)
			return nil
		}))

	svc := newTestService(3, func(ctx context.Context) error {
		started.add(3)
		return nil
	}, func(ctx context.Context) error {
		stopped.add(3)
		return nil
	})

	stopService := sinit.
		StartService(svc, WithTaskCallback(getTaskCallback(3))).
		FutureStop()

	sinit.StopFuture(stopTask1)
	sinit.StopFuture(stopTask2)
	sinit.StopFuture(stopService)

	err := sinit.Run()
	assert.NilError(t, err)
	assert.Equal(t, int32(2), runStarted.Load())
	assert.Equal(t, int32(2), runStopped.Load())
	assert.DeepEqual(t, []int{1, 2, 3, 10, 11, 20, 21, 34, 35}, started.get(), cmpopts.SortSlices(cmp.Less[int]))
	assert.DeepEqual(t, []int{1, 2, 3, 12, 13, 22, 23, 36, 37}, stopped.get(), cmpopts.SortSlices(cmp.Less[int]))
}

func TestManagerShutdownOptions(t *testing.T) {
	ctx := context.Background()
	shutdownCtx := context.WithValue(ctx, "test-shutdown", 5)

	sinit := newTestManager(ctx,
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
}

func TestManagerTaskWithID(t *testing.T) {
	ctx := context.Background()

	started := &testList[string]{}
	stopped := &testList[string]{}

	sinit := newTestManager(ctx,
		WithGlobalTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, isStart bool) {
			if tid, ok := task.(TaskWithID); ok {
				if tstr, ok := tid.TaskID().(string); ok {
					if isStart {
						started.add(tstr)
					} else {
						stopped.add(tstr)
					}
				}
			}
		}, nil)),
	)

	sinit.
		StartTask(WrapTaskWithID("t1start", TaskFunc(func(ctx context.Context) error {
			return nil
		}))).
		AutoStop(WrapTaskWithID("t1stop", TaskFunc(func(ctx context.Context) error {
			return nil
		})))

	sinit.
		StartService(WrapServiceWithID("s1", ServiceFunc(func(ctx context.Context) error {
			return nil
		}, func(ctx context.Context) error {
			return nil
		}))).
		AutoStop()

	sinit.Shutdown()
	err := sinit.Run()
	assert.NilError(t, err)

	assert.DeepEqual(t, []string{"s1", "t1start"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
	assert.DeepEqual(t, []string{"s1", "t1stop"}, stopped.get(), cmpopts.SortSlices(cmp.Less[string]))
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
	_ = sinit.StartService(ServiceFunc(func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
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
	_ = sinit.StartService(ServiceFunc(func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
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

func (t *testTask) Run(ctx context.Context) error {
	return t.task(ctx)
}

type testService struct {
	taskNo int
	start  func(ctx context.Context) error
	stop   func(ctx context.Context) error
}

func newTestService(taskNo int, start func(ctx context.Context) error, stop func(ctx context.Context) error) *testService {
	return &testService{
		taskNo: taskNo,
		start:  start,
		stop:   stop,
	}
}

func (t *testService) Start(ctx context.Context) error {
	return t.start(ctx)
}

func (t *testService) Stop(ctx context.Context) error {
	return t.stop(ctx)
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

func newTestManager(ctx context.Context, options ...Option) *Manager {
	sinit := New(ctx, options...)
	sinit.ExecuteTask(TimeoutTask(5*time.Second, errors.New("test timeout")))
	return sinit
}
