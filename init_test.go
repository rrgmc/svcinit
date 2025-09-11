package svcinit_poc1

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"

	"gotest.tools/v3/assert"
)

type testTaskNoError struct {
	taskNo int
}

func (t testTaskNoError) Error() string {
	return fmt.Sprintf("task %d error", t.taskNo)
}

func TestSvcInit(t *testing.T) {
	ctx := context.Background()

	var m sync.Mutex
	var orderedFinish, orderedStop []int
	var unorderedFinish, unorderedStop []int

	sinit := New(ctx)

	type testService struct {
		svc    Service
		cancel context.CancelFunc
	}

	defaultTaskSvc := func(taskNo int, ordered bool) testService {
		dtCtx, dtCancel := context.WithCancelCause(ctx)
		return testService{
			cancel: func() {
				dtCancel(testTaskNoError{taskNo: taskNo})
			},
			svc: ServiceFunc(
				func(ctx context.Context) error {
					fmt.Printf("Task %d running\n", taskNo)
					defer func() {
						fmt.Printf("Task %d finished\n", taskNo)
					}()

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
						fmt.Printf("Task %d dtCtx done [err:%v]\n", taskNo, context.Cause(dtCtx))
						return context.Cause(ctx)
					case <-ctx.Done():
						fmt.Printf("Task %d ctx done [err:%v]\n", taskNo, context.Cause(dtCtx))
						return context.Cause(ctx)
					}
				}, func(ctx context.Context) error {
					fmt.Printf("Stopping task %d\n", taskNo)
					defer func() {
						m.Lock()
						if ordered {
							orderedStop = append(orderedStop, taskNo)
						} else {
							unorderedStop = append(unorderedStop, taskNo)
						}
						m.Unlock()
					}()
					if dtCancel != nil {
						dtCancel(ErrExit)
					}
					return nil
				}),
		}
	}

	task1 := defaultTaskSvc(1, false)
	sinit.ExecuteTask(task1.svc.Start)

	task2 := defaultTaskSvc(2, true)
	i2Stop := sinit.
		StartTask(task2.svc.Start).
		Stop(task2.svc.Stop)

	task3 := defaultTaskSvc(3, true)
	i3Stop := sinit.
		StartTask(task3.svc.Start).
		StopCtx()

	task4 := defaultTaskSvc(4, true)
	i4Stop := sinit.
		StartService(task4.svc).
		Stop()

	task5 := defaultTaskSvc(5, false)
	sinit.
		StartTask(task5.svc.Start).
		AutoStop()

	sinit.ExecuteTask(SignalTask(os.Interrupt, syscall.SIGINT, syscall.SIGTERM))

	sinit.ExecuteTask(func(ctx context.Context) error {
		task4.cancel()
		return nil
	})

	sinit.StopTask(i2Stop)
	sinit.StopTask(i3Stop)
	sinit.StopTask(i4Stop)

	err := sinit.Run()
	assert.NilError(t, err)
}
