package svcinit

import "context"

type Step int

const (
	StepBefore Step = iota
	StepAfter
)

// type ManagerCallback interface {
// 	BeforeRun(ctx context.Context, stage TaskStage, cause error) error
// 	AfterRun(ctx context.Context, stage TaskStage, cause error) error
// }

// ManagerCallback is a callback for manager events.
// The cause parameter is only set if stage == TaskStageStop.
type ManagerCallback interface {
	Callback(ctx context.Context, stage TaskStage, step Step, cause error) error
}

type ManagerCallbackFunc func(ctx context.Context, stage TaskStage, step Step, cause error) error

func (f ManagerCallbackFunc) Callback(ctx context.Context, stage TaskStage, step Step, cause error) error {
	return f(ctx, stage, step, cause)
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
