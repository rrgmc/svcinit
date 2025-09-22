package svcinit

import (
	"cmp"
	"context"
	"testing"
	"testing/synctest"

	cmp2 "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
)

func TestWrapTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := &testList[string]{}
		stopped := &testList[string]{}
		prestopped := &testList[string]{}

		sm, err := New(
			WithTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
				_, ok := getTestTaskNo(task)
				assert.Check(t, !ok)

				tn, ok := getTestTaskNo(UnwrapTask(task))
				assert.Check(t, ok)
				assert.Check(t, cmp2.Equal(3, tn))
			})),
		)
		assert.NilError(t, err)

		task1 := newTestTask(3, BuildTask(
			WithStart(func(ctx context.Context) error {
				started.add("task3")
				return nil
			}),
			WithStop(func(ctx context.Context) error {
				stopped.add("task3")
				return nil
			}),
			WithPreStop(func(ctx context.Context) error {
				prestopped.add("task3")
				return nil
			}),
		))

		sm.AddTask(NewWrappedTask(task1))

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"task3"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task3"}, prestopped.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task3"}, stopped.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

func TestWrapTaskImplements(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := &testList[string]{}
		prestopped := &testList[string]{}

		sm, err := New(
			WithTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task, stage string, step Step, callbackStep CallbackStep, err error) {
				_, ok := task.(*testTaskComplete)
				assert.Check(t, !ok)

				_, ok = UnwrapTask(task).(*testTaskComplete)
				if !assert.Check(t, ok) {
					return
				}

				ts, ok := task.(TaskSteps)
				if !assert.Check(t, ok) {
					return
				}
				assert.Check(t, cmp2.Equal([]Step{StepStart, StepPreStop}, ts.TaskSteps()))

				to, ok := task.(TaskWithOptions)
				if !assert.Check(t, ok) {
					return
				}
				assert.Check(t, 1 == len(to.TaskOptions()))
			})),
		)
		assert.NilError(t, err)

		task1 := &testTaskComplete{
			task: BuildTask(
				WithStart(func(ctx context.Context) error {
					started.add("task3")
					return nil
				}),
				WithPreStop(func(ctx context.Context) error {
					prestopped.add("task3")
					return nil
				}),
			),
			steps: []Step{
				StepStart, StepPreStop,
			},
			options: []TaskInstanceOption{
				WithCancelContext(true),
			},
		}

		sm.AddTask(NewWrappedTask(task1))

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		assert.DeepEqual(t, []string{"task3"}, started.get(), cmpopts.SortSlices(cmp.Less[string]))
		assert.DeepEqual(t, []string{"task3"}, prestopped.get(), cmpopts.SortSlices(cmp.Less[string]))
	})
}

type testTaskComplete struct {
	task    Task
	steps   []Step
	options []TaskInstanceOption
}

var _ Task = (*testTaskComplete)(nil)
var _ TaskSteps = (*testTaskComplete)(nil)
var _ TaskWithOptions = (*testTaskComplete)(nil)

func (t *testTaskComplete) Run(ctx context.Context, step Step) error {
	return t.task.Run(ctx, step)
}

func (t *testTaskComplete) TaskSteps() []Step {
	return t.steps
}

func (t *testTaskComplete) TaskOptions() []TaskInstanceOption {
	return t.options
}
