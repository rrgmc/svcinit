package svcinit

import "context"

// HealthHandler is a health handler definition that allows implementing probes.
type HealthHandler interface {
	ServiceStarted(context.Context)
	ServiceTerminating(ctx context.Context)
}
