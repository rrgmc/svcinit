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

// func parseTaskOptions(opts ...TaskOption) taskOptions {
// 	var optns taskOptions
// 	for _, opt := range opts {
// 		opt(&optns)
// 	}
// 	return optns
// }

// func parseServiceTaskOptions(svc Service, opts ...TaskOption) Service {
// 	var optns taskOptions
// 	for _, opt := range opts {
// 		opt(&optns)
// 	}
// 	if optns.callback != nil {
// 		svc = ServiceWithCallback(svc, optns.callback)
// 	}
// 	return svc
// }
