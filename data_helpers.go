package svcinit

import "sync/atomic"

type BaseResolvedData struct {
	isResolved   atomic.Bool
	resolvedChan chan struct{}
}

var _ ResolvedData = (*BaseResolvedData)(nil)

func NewBasedResolvedData() *BaseResolvedData {
	return &BaseResolvedData{
		resolvedChan: make(chan struct{}),
	}
}

func (d *BaseResolvedData) IsResolved() bool {
	return d.isResolved.Load()
}

func (d *BaseResolvedData) DoneResolved() <-chan struct{} {
	return d.resolvedChan
}

func (d *BaseResolvedData) SetResolved(options ...BaseResolvedDataSetOption) {
	var optns baseResolvedDataSetOptions
	for _, opt := range options {
		opt(&optns)
	}
	if d.isResolved.CompareAndSwap(false, true) {
		if optns.resolvedFunc != nil {
			optns.resolvedFunc()
		}
		close(d.resolvedChan)
	}
}

type BaseResolvedDataSetOption func(*baseResolvedDataSetOptions)

func WithResolvedFunc(resolvedFunc func()) BaseResolvedDataSetOption {
	return func(opts *baseResolvedDataSetOptions) {
		opts.resolvedFunc = resolvedFunc
	}
}

type baseResolvedDataSetOptions struct {
	resolvedFunc func()
}

type baseResolvedDataPrivate = BaseResolvedData
