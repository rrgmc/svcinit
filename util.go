package svcinit

import (
	"context"
	"slices"
	"sync"
)

// valuePtr allows changing the value even on value declarations.
// It uses a pointer to a pointer internally.
type valuePtr[T any] struct {
	value **T
}

func newValuePtr[T any]() valuePtr[T] {
	return valuePtr[T]{
		value: new(*T),
	}
}

func newValuePtrWithValue[T any](value T) valuePtr[T] {
	ret := newValuePtr[T]()
	ret.Set(value)
	return ret
}

func (v valuePtr[T]) Set(value T) {
	*v.value = &value
}

func (v valuePtr[T]) Clear() {
	*v.value = nil
}

func (v valuePtr[T]) IsNil() bool {
	return *v.value == nil
}

func (v valuePtr[T]) Get() T {
	if v.IsNil() {
		var zero T
		return zero
	}
	return **v.value
}

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

// waitGroupWaitWithContext waits for the waitgroup or the context to be done.
// Returns false if waiting timed out.
func waitGroupWaitWithContext(ctx context.Context, wg *sync.WaitGroup) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return true // completed normally
	case <-ctx.Done():
		return false // timed out
	}
}

type multiErrorBuilder struct {
	m    sync.Mutex
	errs []error
}

func newMultiErrorBuilder() *multiErrorBuilder {
	return &multiErrorBuilder{}
}

func (b *multiErrorBuilder) add(err error) {
	if err == nil {
		return
	}
	b.m.Lock()
	defer b.m.Unlock()
	if me, ok := err.(*multiError); ok {
		b.errs = append(b.errs, me.Errors...)
	} else {
		b.errs = append(b.errs, err)
	}
}

func (b *multiErrorBuilder) build() error {
	b.m.Lock()
	defer b.m.Unlock()
	if len(b.errs) == 0 {
		return nil
	} else if len(b.errs) == 1 {
		return b.errs[0]
	}
	return &multiError{
		Errors: slices.Clone(b.errs),
	}
}
