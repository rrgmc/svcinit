package svcinit_poc1

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestSvcInit(t *testing.T) {
	ctx := context.Background()

	sinit := New(ctx)

	defaultTaskSvc := func(taskNo int) Service {
		durationSec := 3 + rand.Intn(5)
		var dtCtx context.Context
		var dtCancel context.CancelCauseFunc
		return ServiceFunc(
			func(ctx context.Context) error {
				dtCtx, dtCancel = context.WithCancelCause(ctx)
				fmt.Printf("Task %d running [%d seconds]\n", taskNo, durationSec)
				defer func() {
					fmt.Printf("Task %d finished\n", taskNo)
				}()

				select {
				case <-dtCtx.Done():
					fmt.Printf("Task %d dtCtx done [err:%v]\n", taskNo, context.Cause(dtCtx))
					return context.Cause(ctx)
				case <-ctx.Done():
					fmt.Printf("Task %d ctx done [err:%v]\n", taskNo, context.Cause(ctx))
					return context.Cause(ctx)
				case <-time.After(time.Duration(durationSec) * time.Second):
					fmt.Printf("Task %d timeout [%d seconds]\n", taskNo, durationSec)
				}
				return nil
			}, func(ctx context.Context) error {
				fmt.Printf("Stopping task %d\n", taskNo)
				if dtCancel != nil {
					dtCancel(ErrExit)
				}
				return nil
			})
	}

	defaultTask := func(taskNo int) Task {
		return defaultTaskSvc(taskNo).Start
	}

	sinit.ExecuteTask(defaultTask(1))

	task2 := defaultTaskSvc(2)
	i2Stop := sinit.
		StartTask(task2.Start).
		Stop(task2.Stop)

	i3Stop := sinit.
		StartTask(defaultTask(3)).
		CtxStop()

	i4Stop := sinit.
		StartService(defaultTaskSvc(4)).
		Stop()

	sinit.
		StartTask(defaultTask(5)).
		AutoStop()

	sinit.ExecuteTask(SignalTask(os.Interrupt, syscall.SIGINT, syscall.SIGTERM))

	sinit.StopTask(i2Stop)
	sinit.StopTask(i3Stop)
	sinit.StopTask(i4Stop)

	err := sinit.Run()
	assert.NilError(t, err)
}
