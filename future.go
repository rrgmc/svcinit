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

type Future[T any] interface {
	Value(options ...FutureValueOption) (T, error)
	Done() <-chan struct{}
}

type FutureResolver[T any] interface {
	Future[T]
	Resolve(value T)
	ResolveError(err error)
}

type DefaultFuture[T any] struct {
	l   latch
	v   T
	err error
}

var _ FutureResolver[int] = (*DefaultFuture[int])(nil)

func NewFuture[T any]() *DefaultFuture[T] {
	return &DefaultFuture[T]{}
}

func (f *DefaultFuture[T]) Value(options ...FutureValueOption) (T, error) {
	var optns futureValueOptions
	for _, option := range options {
		option(&optns)
	}

	if optns.noWait {
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

func (f *DefaultFuture[T]) Done() <-chan struct{} {
	return f.l.Done()
}

func (f *DefaultFuture[T]) Resolve(value T) {
	f.l.resolve(func() {
		f.v = value
	})
}

func (f *DefaultFuture[T]) ResolveError(err error) {
	f.l.resolve(func() {
		f.err = err
	})
}

type FutureValueOption func(*futureValueOptions)

func WithFutureCtx(ctx context.Context) FutureValueOption {
	return func(o *futureValueOptions) {
		o.ctx = ctx
	}
}

func WithFutureNoWait() FutureValueOption {
	return func(o *futureValueOptions) {
		o.noWait = true
	}
}

type futureValueOptions struct {
	ctx    context.Context
	noWait bool
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
