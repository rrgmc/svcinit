package svcinit

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"time"
)

// waitGroupWaitWithContext waits for the WaitGroup or the context to be done.
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

func sliceMap[S ~[]E, E, R any](slice S, mapper func(int, E) R) []R {
	mappedSlice := make([]R, len(slice))
	for i, v := range slice {
		mappedSlice[i] = mapper(i, v)
	}
	return mappedSlice
}

func sliceFilter[S ~[]E, E any](slice S, filter func(int, E) bool) []E {
	filteredSlice := make([]E, 0, len(slice))
	for i, v := range slice {
		if filter(i, v) {
			filteredSlice = append(filteredSlice, v)
		}
	}
	return slices.Clip(filteredSlice)
}

// reversedSlice returns a reversed iterator to a slice.
func reversedSlice[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := len(s) - 1; i >= 0; i-- {
			if !yield(s[i]) {
				return
			}
		}
	}
}

func stringerString[T fmt.Stringer](s []T) string {
	return strings.Join(stringerList(s), ",")
}

func stringerList[T fmt.Stringer](s []T) []string {
	ss := make([]string, len(s))
	for i, v := range s {
		ss[i] = v.String()
	}
	return ss
}

func stringerIter[T fmt.Stringer](s iter.Seq[T]) []string {
	var ss []string
	for v := range s {
		ss = append(ss, v.String())
	}
	return ss
}

// multiError is an error containing a list of errors.
type multiError struct {
	errors []error
}

func (e *multiError) Error() string {
	if len(e.errors) == 0 {
		return "empty errors"
	}
	return e.errors[0].Error()
}

func (e *multiError) JoinedError() error {
	return errors.Join(e.errors...)
}

func (e *multiError) Unwrap() []error {
	return e.errors
}

// multiErrorBuilder is a thread-safe error builder. It is used to avoid a mutex being return in the final error.
// Returns a multiError error.
type multiErrorBuilder struct {
	m    sync.Mutex
	errs []error
}

func newMultiErrorBuilder() *multiErrorBuilder {
	return &multiErrorBuilder{}
}

func (b *multiErrorBuilder) hasErrors() bool {
	b.m.Lock()
	defer b.m.Unlock()
	return len(b.errs) > 0
}

func (b *multiErrorBuilder) add(err error) {
	if err == nil {
		return
	}
	b.m.Lock()
	defer b.m.Unlock()
	if me, ok := err.(*multiError); ok {
		b.errs = append(b.errs, me.errors...)
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
		errors: slices.Clone(b.errs),
	}
}

func buildMultiErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	} else if len(errs) == 1 {
		return errs[0]
	}
	return &multiError{
		errors: slices.Clone(errs),
	}
}

// sleepContext sleeps while checking for context cancellation.
// Returns nil for any option by default. These can be changed by options.
func sleepContext(ctx context.Context, duration time.Duration, options ...sleepContextOption) error {
	var optns sleepContextOptions
	for _, opt := range options {
		opt(&optns)
	}
	select {
	case <-ctx.Done():
		if optns.contextError {
			return context.Cause(ctx)
		}
		return nil
	case <-time.After(duration):
		return optns.timeoutErr
	}
}

type sleepContextOption func(*sleepContextOptions)

func withSleepContextError(contextError bool) sleepContextOption {
	return func(opts *sleepContextOptions) {
		opts.contextError = contextError
	}
}

func withSleepContextTimeoutError(timeoutErr error) sleepContextOption {
	return func(o *sleepContextOptions) {
		o.timeoutErr = timeoutErr
	}
}

type sleepContextOptions struct {
	contextError bool
	timeoutErr   error
}
