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

	"gotest.tools/v3/assert"
)

func TestCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runStarted, runStopped atomic.Int32
		testcb := &testCallback{}
		testtaskcb := &testCallback{}
		testruncb := &testCallback{}

		errCancelTask1 := errors.New("test context cancel 1")
		errCancelTask2 := errors.New("test context cancel 2")

		// logger := defaultLogger(t.Output())

		sinit, err := New(
			// WithLogger(logger),
			WithStages("s1", "s2"),
			WithManagerCallback(ManagerCallbackFunc(func(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
				switch step {
				case StepStart:
					runStarted.Add(1)
				case StepStop:
					runStopped.Add(1)
				default:
				}
			})),
			WithTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
				assertTestTask(t, task, stage, step, callbackStep)
			})),
			WithTaskCallback(testcb),
		)
		assert.NilError(t, err)

		sinit.
			AddTask("s1", newTestTask(1, BuildTask(
				WithStart(func(ctx context.Context) (err error) {
					testruncb.add(1, "s1", StepStart, CallbackStepBefore, nil)
					defer func() {
						testruncb.add(1, "s1", StepStart, CallbackStepAfter, err)
					}()
					return sleepContext(ctx, 2*10*time.Second,
						withSleepContextError(true))
				}),
				WithStop(func(ctx context.Context) (err error) {
					testruncb.add(1, "s1", StepStop, CallbackStepBefore, nil)
					defer func() {
						testruncb.add(1, "s1", StepStop, CallbackStepAfter, err)
					}()

					ssm := StartStepManagerFromContext(ctx)
					// fmt.Printf("t1 stop ctx: (cancancel:%t)(canfinished:%t)\n",
					// 	ssm.CanContextCancel(), ssm.CanFinished())
					ssm.ContextCancel(errCancelTask1)
					select {
					case <-ctx.Done():
					case <-ssm.Finished():
					}
					return nil
				}),
			)),
				// WithCancelContext(),
				WithStartStepManager(),
				WithCallback(testtaskcb))

		sinit.AddTask("s2", newTestTask(2, BuildTask(
			WithStart(func(ctx context.Context) (err error) {
				testruncb.add(2, "s2", StepStart, CallbackStepBefore, nil)
				defer func() {
					testruncb.add(2, "s2", StepStart, CallbackStepAfter, err)
				}()
				return sleepContext(ctx, 10*time.Second, withSleepContextError(true)) // 10 seconds, will stop before task 1 which is 20 seconds
			}),
			WithPreStop(func(ctx context.Context) (err error) {
				testruncb.add(2, "s2", StepPreStop, CallbackStepBefore, nil)
				defer func() {
					testruncb.add(2, "s2", StepPreStop, CallbackStepAfter, err)
				}()
				return nil
			}),
			WithStop(func(ctx context.Context) (err error) {
				testruncb.add(2, "s2", StepStop, CallbackStepBefore, nil)
				defer func() {
					testruncb.add(2, "s2", StepStop, CallbackStepAfter, err)
				}()

				ssm := StartStepManagerFromContext(ctx)
				// fmt.Printf("t2 stop ctx: (cancancel:%t)(canfinished:%t)\n",
				// 	ssm.CanContextCancel(), ssm.CanFinished())
				ssm.ContextCancel(errCancelTask2)
				select {
				case <-ctx.Done():
				case <-ssm.Finished():
				}

				return nil
			}),
		)),
			// WithCancelContext(),
			WithStartStepManager(),
			WithCallback(testtaskcb))

		err = sinit.Run(t.Context())
		assert.NilError(t, err)

		expected := map[int][]testCallbackItem{
			1: {
				{1, "s1", StepStart, CallbackStepBefore, nil},
				{1, "s1", StepStop, CallbackStepBefore, nil},
				{1, "s1", StepStart, CallbackStepAfter, errCancelTask1},
				{1, "s1", StepStop, CallbackStepAfter, nil},
			},
			2: {
				{2, "s2", StepStart, CallbackStepBefore, nil},
				{2, "s2", StepStart, CallbackStepAfter, nil},
				{2, "s2", StepPreStop, CallbackStepBefore, nil},
				{2, "s2", StepPreStop, CallbackStepAfter, nil},
				{2, "s2", StepStop, CallbackStepBefore, nil},
				{2, "s2", StepStop, CallbackStepAfter, nil},
			},
		}

		assert.Equal(t, int32(6), runStarted.Load())
		assert.Equal(t, int32(6), runStopped.Load())
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

type testCallbackItem struct {
	taskNo       int
	stage        string
	step         Step
	callbackStep CallbackStep
	err          error
}

func (t testCallbackItem) Equal(other testCallbackItem) bool {
	if t.taskNo != other.taskNo || t.stage != other.stage || t.step != other.step || t.callbackStep != other.callbackStep {
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
	if c := cmp.Compare(a.callbackStep, b.callbackStep); c != 0 {
		return c
	}
	return 0
}

type testCount struct {
	stage        string
	step         Step
	callbackStep CallbackStep
}

type testCallback struct {
	m                sync.Mutex
	counts           map[testCount]int
	allTestTasks     []testCallbackItem
	allTestTasksByNo map[int][]testCallbackItem
}

func (t *testCallback) add(taskNo int, stage string, step Step, callbackStep CallbackStep, err error) {
	t.m.Lock()
	if t.counts == nil {
		t.counts = make(map[testCount]int)
	}
	t.counts[testCount{stage: stage, step: step, callbackStep: callbackStep}]++
	t.m.Unlock()

	if taskNo < 0 {
		return
	}
	t.m.Lock()
	defer t.m.Unlock()
	item := testCallbackItem{
		taskNo:       taskNo,
		stage:        stage,
		step:         step,
		callbackStep: callbackStep,
		err:          err,
	}
	if t.allTestTasksByNo == nil {
		t.allTestTasksByNo = make(map[int][]testCallbackItem)
	}
	t.allTestTasks = append(t.allTestTasks, item)
	t.allTestTasksByNo[taskNo] = append(t.allTestTasksByNo[taskNo], item)
}

func (t *testCallback) Callback(_ context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
	taskNo, _ := getTestTaskNo(task)
	t.add(taskNo, stage, step, callbackStep, err)
}

func printTaskCallback(startTime time.Time, ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
	taskNo, _ := getTestTaskNo(task)
	fmt.Printf("[%s] --- [TASK %d] (stage:%s)(step:%s)(callbackStep:%s)(err:%v)\n", time.Since(startTime), // time.Now().Format("15:04:05.000000000"),
		taskNo, stage, step, callbackStep, err)
}

func printManagerCallback(startTime time.Time, ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
	cause, _ := CauseFromContext(ctx)
	fmt.Printf("[%s] $$$ [MANAGER] (stage:%s)(step:%s)(callbackStep:%s)(err:%v)\n", time.Since(startTime), // time.Now().Format("15:04:05.000000000"),
		stage, step, callbackStep, cause)
}

func testTaskCallbackPrint() TaskCallback {
	startTime := time.Now()
	var m sync.Mutex
	return TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
		m.Lock()
		defer m.Unlock()
		printTaskCallback(startTime, ctx, task, stage, step, callbackStep, err)
	})
}

func testCallbacksPrint() (ManagerCallback, TaskCallback) {
	startTime := time.Now()

	var m sync.Mutex
	return ManagerCallbackFunc(func(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
			m.Lock()
			defer m.Unlock()
			printManagerCallback(startTime, ctx, stage, step, callbackStep)
		}),
		TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
			m.Lock()
			defer m.Unlock()
			printTaskCallback(startTime, ctx, task, stage, step, callbackStep, err)
		})
}
