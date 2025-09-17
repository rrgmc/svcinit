package svcinit

import (
	"context"
	"slices"
	"sync"
)

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
	b.errs = append(b.errs, err)
}

func (b *multiErrorBuilder) build() error {
	b.m.Lock()
	defer b.m.Unlock()
	if len(b.errs) == 0 {
		return nil
	}
	return &multiError{
		Errors: slices.Clone(b.errs),
	}
}
