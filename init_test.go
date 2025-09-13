package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"

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

func TestSvcInit(t *testing.T) {
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
					svc: ServiceTaskFunc(
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
				ManualStop(ServiceAsTask(task2.svc, false))

			task3 := defaultTaskSvc(3, true)
			i3Stop := sinit.
				StartTask(ServiceAsTask(task3.svc, true)).
				ManualStopCancelTask(ServiceAsTask(task3.svc, false))

			task4 := defaultTaskSvc(4, true)
			i4Stop := sinit.
				StartService(task4.svc).
				ManualStop()

			task5 := defaultTaskSvc(5, false)
			sinit.
				StartTask(ServiceAsTask(task5.svc, true)).
				AutoStop()

			sinit.ExecuteTask(SignalTask(os.Interrupt, syscall.SIGTERM))

			tasks := []testService{task1, task2, task3, task4, task5}

			sinit.SetStartedCallback(TaskFunc(func(ctx context.Context) error {
				for _, taskNo := range test.cancelFn() {
					tasks[taskNo-1].cancel()
				}
				return nil
			}))

			sinit.StopTask(i2Stop)
			sinit.StopTask(i3Stop)
			sinit.StopTask(i4Stop)

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

func TestSvcInitStopMultipleTasks(t *testing.T) {
	sinit := New(context.Background())

	started := &testList[int]{}
	stopped := &testList[int]{}

	stopTask1 := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			started.add(1)
			return nil
		}).
		ManualStopFunc(func(ctx context.Context) error {
			stopped.add(1)
			return nil
		})

	stopTask2 := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			started.add(2)
			return nil
		}).
		ManualStopFunc(func(ctx context.Context) error {
			stopped.add(2)
			return nil
		})

	sinit.
		StopMultipleTasks(stopTask1, stopTask2)

	err := sinit.Run()

	assert.NilError(t, err)

	assert.DeepEqual(t, []int{1, 2}, started.get(), cmpopts.SortSlices(cmp.Less[int]))
	assert.DeepEqual(t, []int{1, 2}, stopped.get(), cmpopts.SortSlices(cmp.Less[int]))
}

func TestSvcInitCallback(t *testing.T) {
	sinit := New(context.Background())

	started := &testList[int]{}
	stopped := &testList[int]{}

	getTaskCallback := func(taskNo int, isStop bool) TaskCallback {
		stdAdd := 0
		if isStop {
			stdAdd = 2
		}
		return TaskCallbackFunc(func(ctx context.Context, task Task) {
			if !isStop {
				started.add((taskNo * 10) + stdAdd)
			} else {
				stopped.add((taskNo * 10) + stdAdd)
			}
		}, func(ctx context.Context, task Task, err error) {
			if !isStop {
				started.add((taskNo * 10) + stdAdd + 1)
			} else {
				stopped.add((taskNo * 10) + stdAdd + 1)
			}
		})
	}

	stopTask1 := sinit.
		StartTask(TaskFuncWithCallback(func(ctx context.Context) error {
			started.add(1)
			return nil
		}, getTaskCallback(1, false))).
		ManualStop(TaskFuncWithCallback(func(ctx context.Context) error {
			stopped.add(1)
			return nil
		}, getTaskCallback(1, true)))

	stopTask2 := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			started.add(2)
			return nil
		}).
		ManualStop(TaskFuncWithCallback(func(ctx context.Context) error {
			stopped.add(2)
			return nil
		}, getTaskCallback(2, true)))

	sinit.StopTask(stopTask1)
	sinit.StopTask(stopTask2)

	err := sinit.Run()

	assert.NilError(t, err)

	assert.DeepEqual(t, []int{1, 2, 10, 11}, started.get(), cmpopts.SortSlices(cmp.Less[int]))
	assert.DeepEqual(t, []int{1, 2, 12, 13, 22, 23}, stopped.get(), cmpopts.SortSlices(cmp.Less[int]))
}

func TestSvcInitPendingStart(t *testing.T) {
	sinit := New(context.Background())

	// must call one StartTaskCmd method
	_ = sinit.StartTaskFunc(func(ctx context.Context) error {
		return nil
	})

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestSvcInitPendingStartService(t *testing.T) {
	sinit := New(context.Background())

	// must call one StartServiceCmd method
	_ = sinit.StartService(ServiceTaskFunc(func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		return nil
	}))

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestSvcInitPendingStop(t *testing.T) {
	sinit := New(context.Background())

	// must add stop function to StopTask
	_ = sinit.StartTaskFunc(func(ctx context.Context) error {
		return nil
	}).ManualStopFunc(func(ctx context.Context) error {
		return nil
	})

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
}

func TestSvcInitPendingStopService(t *testing.T) {
	sinit := New(context.Background())

	// must add stop function to StopTask
	_ = sinit.StartService(ServiceTaskFunc(func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		return nil
	})).ManualStop()

	err := sinit.Run()
	assert.ErrorIs(t, err, ErrPending)
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
