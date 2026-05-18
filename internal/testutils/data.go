package testutils

import (
	"cmp"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"
	cmp3 "gotest.tools/v3/assert/cmp"
)

type TestList[T any] struct {
	m    sync.Mutex
	list []T
}

func (l *TestList[T]) Add(item T) {
	l.m.Lock()
	l.list = append(l.list, item)
	l.m.Unlock()
}

func (l *TestList[T]) Get() []T {
	l.m.Lock()
	defer l.m.Unlock()
	return l.list
}

func (l *TestList[T]) AssertDeepEqual(t *testing.T, expected []T) {
	assert.DeepEqual(t, expected, l.Get(), cmpopts.SortSlices(cmp.Less[string]))
}

func (l *TestList[T]) CheckDeepEqual(t *testing.T, expected []T) bool {
	return assert.Check(t, cmp3.DeepEqual(expected, l.Get(), cmpopts.SortSlices(cmp.Less[string])))
}
