package svcinit

import (
	"context"
	"time"
)

// SleepContext sleeps while checking for context cancellation.
// Returns [context.Cause] of the context error, or nil if timed out.
func SleepContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(duration):
		return nil
	}
}
