package svcinit

import "context"

type ManagerCallback interface {
	BeforeRun(ctx context.Context, stage TaskStage, cause error) error
	AfterRun(ctx context.Context, stage TaskStage, cause error) error
}

type TaskOption func(options *taskOptions)

func WithTaskCallback(callback TaskCallback) TaskOption {
	return func(options *taskOptions) {
		options.callback = callback
	}
}

type StopOption func(options *stopOptions)

// WithCancelContext sets whether to cancel the context on stop.
// Default is false.
func WithCancelContext(cancelContext bool) StopOption {
	return func(options *stopOptions) {
		options.cancelContext = cancelContext
	}
}

// definitions

type taskOptions struct {
	callback TaskCallback
}

type stopOptions struct {
	cancelContext bool
}
