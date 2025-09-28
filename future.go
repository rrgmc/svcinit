package svcinit

// https://github.com/ianlopshire/go-async

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrAlreadyResolved = errors.New("already resolved")
	ErrNotResolved     = errors.New("not resolved")
)

// Future is a proxy for a result that will be resolved in the future.
type Future[T any] interface {
	// Value gets the resolved future value. By default, it does not wait the value to be resolved, use the
	// WithFutureWait option to make it wait.
	Value(options ...FutureValueOption) (T, error)
	// Done is a channel that is closed when the future value is resolved.
	Done() <-chan struct{}
}

// FutureResolver is a resolvable Future.
type FutureResolver[T any] interface {
	Future[T]
	// Resolve resolves the Future with the passed value.
	Resolve(value T)
	// ResolveError resolves the Future with the passed error.
	ResolveError(err error)
}

var _ FutureResolver[int] = (*future[int])(nil)

func NewFuture[T any]() FutureResolver[T] {
	return &future[T]{}
}

type FutureValueOption func(*futureValueOptions)

// WithFutureCtx adds a context to be checked if WithFutureWait is set.
func WithFutureCtx(ctx context.Context) FutureValueOption {
	return func(o *futureValueOptions) {
		o.ctx = ctx
	}
}

// WithFutureWait makes the [Future.Value] wait until the Future is resolved.
func WithFutureWait() FutureValueOption {
	return func(o *futureValueOptions) {
		o.wait = true
	}
}

type future[T any] struct {
	l   latch
	v   T
	err error
}

func (f *future[T]) Value(options ...FutureValueOption) (T, error) {
	var optns futureValueOptions
	for _, option := range options {
		option(&optns)
	}

	if !optns.wait {
		select {
		case <-f.l.Done():
			return f.v, f.err
		default:
			var empty T
			return empty, ErrNotResolved
		}
	}

	var ctxDone <-chan struct{}
	if optns.ctx != nil {
		ctxDone = optns.ctx.Done()
	}

	select {
	case <-ctxDone:
		var empty T
		return empty, context.Cause(optns.ctx)
	case <-f.l.Done():
		return f.v, f.err
	}
}

func (f *future[T]) Done() <-chan struct{} {
	return f.l.Done()
}

func (f *future[T]) Resolve(value T) {
	f.l.resolve(func() {
		f.v = value
	})
}

func (f *future[T]) ResolveError(err error) {
	f.l.resolve(func() {
		f.err = err
	})
}

type futureValueOptions struct {
	ctx  context.Context
	wait bool
}

// closedchan is a reusable closed channel.
var closedchan = make(chan struct{})

func init() {
	close(closedchan)
}

// latch is a synchronization primitive that can be used to block until a desired state is
// reached.
//
// The zero value for latch is in an open (blocking) state. Use the package level Resolve
// function to resolve a latch. Once resolved, the latch cannot be reopened.
//
// Attempting to resolve a latch more than once will panic with ErrAlreadyResolved.
//
// A latch must not be copied after first use.
type latch struct {
	mu   sync.Mutex
	done chan struct{}
}

// Done returns a channel that will be closed when the latch is resolved.
func (l *latch) Done() <-chan struct{} {
	l.mu.Lock()
	if l.done == nil {
		l.done = make(chan struct{})
	}
	d := l.done
	l.mu.Unlock()
	return d
}

func (l *latch) resolve(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if fn != nil {
		fn()
	}

	if l.done == nil {
		l.done = closedchan
		return
	}

	select {
	case <-l.done:
		panic(ErrAlreadyResolved)
	default:
		// Intentionally left blank.
	}

	close(l.done)
}
