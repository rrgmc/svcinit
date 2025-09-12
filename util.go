package svcinit

import (
	"context"
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

// WaitGroupWaitWithContext waits for the waitgroup or the context to be done.
// Returns false if waiting timed out.
func WaitGroupWaitWithContext(ctx context.Context, wg *sync.WaitGroup) bool {
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
