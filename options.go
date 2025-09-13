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

func parseTaskOptions(task Task, opts ...TaskOption) Task {
	var optns taskOptions
	for _, opt := range opts {
		opt(&optns)
	}
	if optns.callback != nil {
		task = TaskWithCallback(task, optns.callback)
	}
	return task
}

func parseServiceTaskOptions(svc Service, opts ...TaskOption) Service {
	var optns taskOptions
	for _, opt := range opts {
		opt(&optns)
	}
	if optns.callback != nil {
		svc = ServiceWithCallback(svc, optns.callback)
	}
	return svc
}
