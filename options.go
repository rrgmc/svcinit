package svcinit

type TaskOption func(options *taskOptions)

func WithTaskCallback(callback TaskCallback) TaskOption {
	return func(options *taskOptions) {
		options.callback = callback
	}
}

type taskOptions struct {
	callback TaskCallback
}
