package k8sinit

import "github.com/rrgmc/svcinit/v3"

type Manager struct {
	manager        *svcinit.Manager
	managerOptions []svcinit.Option
}

func New(options ...Option) (*Manager, error) {
	ret := &Manager{}
	for _, option := range options {
		option(ret)
	}

	var err error
	ret.manager, err = svcinit.New(ret.managerOptions...)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

type Option func(*Manager)

func WithManagerOptions(options ...svcinit.Option) Option {
	return func(m *Manager) {
		m.managerOptions = append(m.managerOptions, options...)
	}
}
