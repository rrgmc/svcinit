package svcinit_poc1

import "context"

type Task func(ctx context.Context) error

type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

func ServiceFunc(start, stop Task) Service {
	return &serviceFunc{start: start, stop: stop}
}

type serviceFunc struct {
	start func(ctx context.Context) error
	stop  func(ctx context.Context) error
}

func (sf *serviceFunc) Start(ctx context.Context) error {
	if sf.start == nil {
		return nil
	}
	return sf.start(ctx)
}

func (sf *serviceFunc) Stop(ctx context.Context) error {
	if sf.stop == nil {
		return nil
	}
	return sf.stop(ctx)
}

type StopTask interface {
	Stop(ctx context.Context) error
}

type StopTaskFunc func(ctx context.Context) error

func (sf StopTaskFunc) Stop(ctx context.Context) error {
	return sf(ctx)
}
