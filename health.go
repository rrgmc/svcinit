package svcinit

import "context"

// HealthHandler is a health handler definition that allows implementing probes.
type HealthHandler interface {
	ServiceStarted(context.Context)
	ServiceTerminating(ctx context.Context)
}

// BuildHealthHandler builds a HealthHandler from callback functions.
func BuildHealthHandler(options ...BuildHealthHandlerOption) HealthHandler {
	ret := &buildHealthHandler{}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type BuildHealthHandlerOption func(*buildHealthHandler)

func WithHealthHandlerServiceStarted(f func(context.Context)) BuildHealthHandlerOption {
	return func(build *buildHealthHandler) {
		build.serviceStarted = f
	}
}

func WithHealthHandlerServiceTerminating(f func(context.Context)) BuildHealthHandlerOption {
	return func(build *buildHealthHandler) {
		build.serviceTerminating = f
	}
}

// internal

type buildHealthHandler struct {
	serviceStarted     func(context.Context)
	serviceTerminating func(context.Context)
}

var _ HealthHandler = (*buildHealthHandler)(nil)

func (t *buildHealthHandler) ServiceStarted(ctx context.Context) {
	if t.serviceStarted == nil {
		return
	}
	t.serviceStarted(ctx)
}

func (t *buildHealthHandler) ServiceTerminating(ctx context.Context) {
	if t.serviceTerminating == nil {
		return
	}
	t.serviceTerminating(ctx)
}
