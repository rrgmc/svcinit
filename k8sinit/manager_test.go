package k8sinit

import (
	"cmp"
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/rrgmc/svcinit/v3"
	"gotest.tools/v3/assert"
	cmp3 "gotest.tools/v3/assert/cmp"
)

func TestManager(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		items := &testList[string]{}

		sm, err := New(
			WithHealthMode(HealthModeHTTPServer),
			WithHealthHTTPAddress(":6060"),
		)

		sm.AddTask(StageService, svcinit.BuildTask(
			svcinit.WithStart(func(ctx context.Context) error {
				items.add("start")
				return nil
			}),
			svcinit.WithStop(func(ctx context.Context) error {
				items.add("stop")
				return nil
			}),
		))
		assert.NilError(t, err)

		err = sm.Run(t.Context())
		assert.NilError(t, err)

		items.assertDeepEqual(t, []string{"start", "stop"})
	})
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

func (l *testList[T]) assertDeepEqual(t *testing.T, expected []T) {
	assert.DeepEqual(t, expected, l.get(), cmpopts.SortSlices(cmp.Less[string]))
}

func (l *testList[T]) checkDeepEqual(t *testing.T, expected []T) bool {
	return assert.Check(t, cmp3.DeepEqual(expected, l.get(), cmpopts.SortSlices(cmp.Less[string])))
}
