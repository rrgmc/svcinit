package svcinit

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestValuePtr(t *testing.T) {
	type def struct {
		intValue valuePtr[int]
	}

	change := func(v valuePtr[int], value int) {
		v.Set(value)
	}
	clr := func(v valuePtr[int]) {
		v.Clear()
	}

	data := def{
		intValue: newValuePtr[int](),
	}

	assert.Assert(t, data.intValue.IsNil())

	change(data.intValue, 15)

	assert.Assert(t, !data.intValue.IsNil())
	assert.Equal(t, 15, data.intValue.Get())

	clr(data.intValue)

	assert.Assert(t, data.intValue.IsNil())
}
