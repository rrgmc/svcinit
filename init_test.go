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

			var m sync.Mutex
			var orderedFinish, orderedStop []int
			var unorderedFinish, unorderedStop []int

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
								m.Lock()
								if ordered {
									orderedFinish = append(orderedFinish, taskNo)
								} else {
									unorderedFinish = append(unorderedFinish, taskNo)
								}
								m.Unlock()
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
								m.Lock()
								if ordered {
									orderedStop = append(orderedStop, taskNo)
								} else {
									unorderedStop = append(unorderedStop, taskNo)
								}
								m.Unlock()
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
				ManualStopFuncCancel(ServiceAsTask(task3.svc, false))

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

			assert.DeepEqual(t, test.expectedOrderedFinish, orderedFinish)
			assert.DeepEqual(t, test.expectedOrderedStop, orderedStop)
			assert.DeepEqual(t, test.expectedUnorderedFinish, unorderedFinish, cmpopts.SortSlices(cmp.Less[int]))
			assert.DeepEqual(t, test.expectedUnorderedStop, unorderedStop, cmpopts.SortSlices(cmp.Less[int]))
		})
	}
}

func TestSvcInitParallelStop(t *testing.T) {
	sinit := New(context.Background())

	var m sync.Mutex
	var started []int
	var stopped []int

	stopTask1 := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			m.Lock()
			defer m.Unlock()
			started = append(started, 1)
			return nil
		}).
		ManualStopFunc(func(ctx context.Context) error {
			m.Lock()
			defer m.Unlock()
			stopped = append(stopped, 1)
			return nil
		})

	stopTask2 := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			m.Lock()
			defer m.Unlock()
			started = append(started, 2)
			return nil
		}).
		ManualStopFunc(func(ctx context.Context) error {
			m.Lock()
			defer m.Unlock()
			stopped = append(stopped, 2)
			return nil
		})

	sinit.StopTasksParallel(stopTask1, stopTask2)

	err := sinit.Run()

	assert.NilError(t, err)

	assert.DeepEqual(t, []int{1, 2}, started, cmpopts.SortSlices(cmp.Less[int]))
	assert.DeepEqual(t, []int{1, 2}, stopped, cmpopts.SortSlices(cmp.Less[int]))
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
