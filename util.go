package svcinit

type resolved struct {
	value *bool
}

func newResolved() resolved {
	value := false
	return resolved{
		value: &value,
	}
}

func (r resolved) isResolved() bool {
	return *r.value
}

func (r resolved) setResolved() {
	*r.value = true
}
