package svcinit

type TaskOption interface {
	baseTaskOption
	FutureStopOption
}

func WithTaskCallback(callback TaskCallback) TaskOption {
	return &taskOptionImpl{
		to: func(options *taskOptions) {
			options.callback = callback
		},
	}
}

type FutureStopOption interface {
	futureStopOption(options *futureStopOptions)
}

func WithCancelContext(cancelContext bool) FutureStopOption {
	return &futureStopOptionImpl{
		fso: func(options *futureStopOptions) {
			options.cancelContext = cancelContext
		},
	}
}

// impl

type taskOptions struct {
	callback TaskCallback
}

type futureStopOptions struct {
	cancelContext bool
}

type baseTaskOption interface {
	taskOption(options *taskOptions)
}

type taskOptionImpl struct {
	to func(options *taskOptions)
}

var _ TaskOption = (*taskOptionImpl)(nil)

func (t *taskOptionImpl) taskOption(options *taskOptions) {
	t.to(options)
}

func (t *taskOptionImpl) futureStopOption(options *futureStopOptions) {}

type futureStopOptionImpl struct {
	fso func(options *futureStopOptions)
}

var _ FutureStopOption = (*futureStopOptionImpl)(nil)

func (t *futureStopOptionImpl) futureStopOption(options *futureStopOptions) {
	t.fso(options)
}

// util

func parseFutureStopOptions(options ...FutureStopOption) parsedFutureStopOption {
	ret := parsedFutureStopOption{}
	for _, option := range options {
		if fso, ok := option.(FutureStopOption); ok {
			fso.futureStopOption(&ret.futureStopOptions)
		}
		if to, ok := option.(TaskOption); ok {
			ret.options = append(ret.options, to)
		}
	}
	return ret
}

func castFutureStopOptions(options []TaskOption) []FutureStopOption {
	ret := make([]FutureStopOption, len(options))
	for i, opt := range options {
		ret[i] = opt
	}
	return ret
}

type parsedFutureStopOption struct {
	options           []TaskOption
	futureStopOptions futureStopOptions
}
