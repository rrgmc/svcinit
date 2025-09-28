package svcinit

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"gotest.tools/v3/assert"
)

func TestFuture_alreadyResolved(t *testing.T) {
	// When an already-resolved Future is resolved it should panic with ErrAlreadyResolved.
	defer func() {
		if recover() != ErrAlreadyResolved {
			t.Fatal("expected Future to panic with ErrAlreadyResolved")
		}
	}()

	fut := NewFuture[string]()
	fut.Resolve("Hello World!")
	fut.Resolve("Hello World!")
}

func TestFuture_notResolved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fut := NewFuture[string]()
		_, err := fut.Value(WithoutFutureWait())
		assert.ErrorIs(t, err, ErrNotResolved)
	})
}

func TestFuture_Value(t *testing.T) {
	for name, tt := range map[string]struct {
		v   string
		err error
	}{
		"with value": {"Hello World!", nil},
		"with error": {"", errors.New("error")},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fut := NewFuture[string]()
				if tt.err != nil {
					fut.ResolveError(tt.err)
				} else {
					fut.Resolve(tt.v)
				}

				v, err := fut.Value()
				assert.Equal(t, tt.err, err)
				assert.Equal(t, tt.v, v)
			})
		})
	}
}

func TestFuture_ValueCtx(t *testing.T) {
	for name, tt := range map[string]struct {
		v   string
		err error
	}{
		"with value": {"Hello World!", nil},
		"with error": {"", errors.New("error")},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fut := NewFuture[string]()
				if tt.err != nil {
					fut.ResolveError(tt.err)
				} else {
					fut.Resolve(tt.v)
				}

				v, err := fut.Value(WithFutureCtx(context.Background()))
				assert.Equal(t, tt.err, err)
				assert.Equal(t, tt.v, v)
			})
		})
	}

	t.Run("with canceled context", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			timeout := time.After(time.Second)
			done := make(chan bool)

			go func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				fut := NewFuture[string]()
				_, err := fut.Value(WithFutureCtx(ctx))
				assert.ErrorIs(t, err, context.Canceled)

				done <- true
			}()

			select {
			case <-timeout:
				t.Fatal("ValueCtx() future should not have blocked")
			case <-done:
			}
		})
	})
}

func TestLatch_alreadyResolved(t *testing.T) {
	// When an already-resolved Latch is resolved it should panic with ErrAlreadyResolved.
	defer func() {
		if recover() != ErrAlreadyResolved {
			t.Fatal("expected latch to panic with ErrAlreadyResolved")
		}
	}()

	var counter int
	inc := func() { counter++ }

	l := new(latch)
	l.resolve(inc)
	l.resolve(inc)

	assert.Equal(t, 1, counter, "expect resolve func to be called at most once")
}
