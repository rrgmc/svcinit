package svcinit

import (
	"cmp"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
)

func TestService(t *testing.T) {
	svc := &testService{}
	task := ServiceAsTask(svc)
	for _, step := range allSteps {
		err := task.Run(t.Context(), step)
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, []string{"PreStop", "Setup", "Start", "Stop", "Teardown"}, svc.tl.get(), cmpopts.SortSlices(cmp.Less[string]))

	ti2 := &testService2{}
	task2 := ServiceAsTask(ti2)
	for _, step := range allSteps {
		err := task2.Run(t.Context(), step)
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, []string{"Setup", "Start", "Stop", "Teardown"}, ti2.tl.get(), cmpopts.SortSlices(cmp.Less[string]))
}

type testService struct {
	tl testList[string]
}

var _ Service = (*testService)(nil)
var _ ServiceWithSetup = (*testService)(nil)

func (t *testService) PreStop(ctx context.Context) error {
	t.tl.add("PreStop")
	return nil
}

func (t *testService) Setup(ctx context.Context) error {
	t.tl.add("Setup")
	return nil
}

func (t *testService) Teardown(ctx context.Context) error {
	t.tl.add("Teardown")
	return nil
}

func (t *testService) Start(ctx context.Context) error {
	t.tl.add("Start")
	return nil
}

func (t *testService) Stop(ctx context.Context) error {
	t.tl.add("Stop")
	return nil
}

type testService2 struct {
	tl testList[string]
}

var _ Service = (*testService2)(nil)
var _ ServiceWithSetup = (*testService2)(nil)

func (t *testService2) Setup(ctx context.Context) error {
	t.tl.add("Setup")
	return nil
}

func (t *testService2) Start(ctx context.Context) error {
	t.tl.add("Start")
	return nil
}

func (t *testService2) Stop(ctx context.Context) error {
	t.tl.add("Stop")
	return nil
}

func (t *testService2) Teardown(ctx context.Context) error {
	t.tl.add("Teardown")
	return nil
}
