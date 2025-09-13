package svcinit

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
)

func TestTaskCallbackRecursive(t *testing.T) {
	data := &testList[int]{}

	task1 := TaskFunc(func(ctx context.Context) error {
		data.add(0)
		return nil
	})
	task1cb1 := TaskWithCallback(task1, TaskCallbackFunc(func(ctx context.Context, task Task) {
		data.add(1)
	}, func(ctx context.Context, task Task, err error) {
		data.add(2)
	}))
	task1cb2 := TaskWithCallback(task1cb1, TaskCallbackFunc(func(ctx context.Context, task Task) {
		data.add(3)
	}, func(ctx context.Context, task Task, err error) {
		data.add(4)
	}))

	err := task1cb2.Run(context.Background())
	assert.NilError(t, err)
	assert.DeepEqual(t, []int{3, 1, 0, 2, 4}, data.get())
}

func TestServiceCallbackRecursive(t *testing.T) {
	data := &testList[int]{}

	svc1 := ServiceTaskFunc(func(ctx context.Context) error {
		data.add(0)
		return nil
	}, func(ctx context.Context) error {
		data.add(1)
		return nil
	})
	svc1cb1 := ServiceWithCallback(svc1, TaskCallbackFunc(func(ctx context.Context, task Task) {
		data.add(2)
	}, func(ctx context.Context, task Task, err error) {
		data.add(3)
	}))
	svc1cb2 := ServiceWithCallback(svc1cb1, TaskCallbackFunc(func(ctx context.Context, task Task) {
		data.add(4)
	}, func(ctx context.Context, task Task, err error) {
		data.add(5)
	}))

	startTask, stopTask := ServiceAsTasks(svc1cb2)

	ctx := context.Background()

	err := startTask.Run(ctx)
	assert.NilError(t, err)
	err = stopTask.Run(ctx)
	assert.NilError(t, err)
	assert.DeepEqual(t, []int{3, 1, 2, 4}, data.get())
}
