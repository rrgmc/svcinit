package svcinit

import (
	"cmp"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rrgmc/svcinit/v3/internal/testutils"
	"gotest.tools/v3/assert"
)

func TestService(t *testing.T) {
	svc := &testService{}
	task := ServiceAsTask(svc)
	for _, step := range allSteps {
		err := task.Run(t.Context(), step)
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, []string{"Setup", "Start", "Stop", "Teardown"}, svc.tl.Get(), cmpopts.SortSlices(cmp.Less[string]))

	ti2 := &testService2{}
	task2 := ServiceAsTask(ti2)
	for _, step := range allSteps {
		err := task2.Run(t.Context(), step)
		assert.NilError(t, err)
	}
	assert.DeepEqual(t, []string{"Setup", "Start", "Stop"}, ti2.tl.Get(), cmpopts.SortSlices(cmp.Less[string]))
}

type testService struct {
	tl testutils.TestList[string]
}

var _ Service = (*testService)(nil)
var _ ServiceWithSetup = (*testService)(nil)
var _ ServiceWithTeardown = (*testService)(nil)

func (t *testService) Setup(ctx context.Context) error {
	t.tl.Add("Setup")
	return nil
}

func (t *testService) Teardown(ctx context.Context) error {
	t.tl.Add("Teardown")
	return nil
}

func (t *testService) Start(ctx context.Context) error {
	t.tl.Add("Start")
	return nil
}

func (t *testService) Stop(ctx context.Context) error {
	t.tl.Add("Stop")
	return nil
}

type testService2 struct {
	tl testutils.TestList[string]
}

var _ Service = (*testService2)(nil)
var _ ServiceWithSetup = (*testService2)(nil)

func (t *testService2) Setup(ctx context.Context) error {
	t.tl.Add("Setup")
	return nil
}

func (t *testService2) Start(ctx context.Context) error {
	t.tl.Add("Start")
	return nil
}

func (t *testService2) Stop(ctx context.Context) error {
	t.tl.Add("Stop")
	return nil
}
