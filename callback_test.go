package svcinit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"gotest.tools/v3/assert"
	cmp2 "gotest.tools/v3/assert/cmp"
)

func TestCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runStarted, runStopped atomic.Int32
		testcb := &testCallback{}
		testtaskcb := &testCallback{}
		testruncb := &testCallback{}

		errCancelTask1 := errors.New("test context cancel 1")
		errCancelTask2 := errors.New("test context cancel 2")

		sinit, err := New(
			// WithLogger(defaultLogger(t.Output())),
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

		expectedAll := []testCallbackItem{
			{1, "s1", StepStart, CallbackStepBefore, nil},
			{1, "s1", StepStop, CallbackStepBefore, nil},
			{1, "s1", StepStart, CallbackStepAfter, errCancelTask1},
			{1, "s1", StepStop, CallbackStepAfter, nil},

			{2, "s2", StepStart, CallbackStepBefore, nil},
			{2, "s2", StepStart, CallbackStepAfter, nil},
			{2, "s2", StepStop, CallbackStepBefore, nil},
			{2, "s2", StepStop, CallbackStepAfter, nil},
		}

		assert.Equal(t, int32(4), runStarted.Load())
		assert.Equal(t, int32(4), runStopped.Load())
		testcb.assertExpectedNotExpected(t, expectedAll, nil)
		testtaskcb.assertExpectedNotExpected(t, expectedAll, nil)
		testruncb.assertExpectedNotExpected(t, expectedAll, nil)
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

type testStageStep struct {
	Stage string
	Step  Step
}

func testStageStepCompare(a, b testStageStep) int {
	if c := cmp.Compare(a.Stage, b.Stage); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Step, b.Step); c != 0 {
		return c
	}
	return 0
}

type testCallback struct {
	filterCallbackStep *CallbackStep

	m     sync.Mutex
	items []testCallbackItem
}

func (t *testCallback) add(taskNo int, stage string, step Step, callbackStep CallbackStep, err error) {
	if t.filterCallbackStep != nil && *t.filterCallbackStep != callbackStep {
		return
	}

	if taskNo < 0 {
		taskNo = 0
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
	t.items = append(t.items, item)
}

func (t *testCallback) containsAll(testTasks []testCallbackItem) []testCallbackItem {
	t.m.Lock()
	defer t.m.Unlock()
	var ret []testCallbackItem

	for _, task := range testTasks {
		if !slices.ContainsFunc(t.items, func(item testCallbackItem) bool {
			return item.Equal(task)
		}) {
			ret = append(ret, task)
		}
	}
	return ret
}

func (t *testCallback) assertExpectedNotExpected(tt *testing.T, expected, notExpected []testCallbackItem) {
	_ = assert.Check(tt, cmp2.DeepEqual([]testCallbackItem(nil), t.containsAll(expected)), "failed expected test")
	_ = assert.Check(tt, cmp2.DeepEqual(notExpected, t.containsAll(notExpected)), "failed not expected test")
}

func (t *testCallback) Callback(_ context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
	taskNo, _ := getTestTaskNo(task)
	t.add(taskNo, stage, step, callbackStep, err)
}

type testManagerCallback struct {
	filterCallbackStep *CallbackStep

	m     sync.Mutex
	steps []testStageStep
}

func (t *testManagerCallback) add(stage string, step Step, callbackStep CallbackStep, err error) {
	if t.filterCallbackStep != nil && *t.filterCallbackStep != callbackStep {
		return
	}

	t.m.Lock()
	defer t.m.Unlock()
	stageStep := testStageStep{
		Stage: stage,
		Step:  step,
	}
	if !slices.ContainsFunc(t.steps, func(step testStageStep) bool {
		return testStageStepCompare(step, stageStep) == 0
	}) {
		t.steps = append(t.steps, stageStep)
	}
}

func (t *testManagerCallback) assertStageSteps(tt *testing.T, expected []testStageStep) {
	t.m.Lock()
	defer t.m.Unlock()
	assert.DeepEqual(tt, expected, t.steps)
}

func (t *testManagerCallback) Callback(ctx context.Context, stage string, step Step, callbackStep CallbackStep) {
	cause, _ := CauseFromContext(ctx)
	t.add(stage, step, callbackStep, cause)
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
