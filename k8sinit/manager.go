package k8sinit

import (
	"context"
	"slices"

	"github.com/rrgmc/svcinit/v3"
)

type Manager struct {
	manager        *svcinit.Manager
	managerOptions []svcinit.Option
}

func New(options ...Option) (*Manager, error) {
	ret := &Manager{}
	for _, option := range options {
		option(ret)
	}

	managerOptions := slices.Concat(ret.managerOptions,
		[]svcinit.Option{
			svcinit.WithStages(allStages...),
		},
	)

	var err error
	ret.manager, err = svcinit.New(managerOptions...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Stages returns the stages configured for execution.
func (m *Manager) Stages() []string {
	return m.manager.Stages()
}

func (m *Manager) IsRunning() bool {
	return m.manager.IsRunning()
}

// AddTask add a Task to be executed at the passed stage.
func (m *Manager) AddTask(stage string, task svcinit.Task, options ...svcinit.TaskOption) {
	m.manager.AddTask(stage, task, options...)
}

func (m *Manager) AddTaskFunc(stage string, f svcinit.TaskFunc, options ...svcinit.TaskOption) {
	m.AddTaskFunc(stage, f, options...)
}

func (m *Manager) AddService(stage string, service svcinit.Service, options ...svcinit.TaskOption) {
	m.AddService(stage, service, options...)
}

func (m *Manager) Run(ctx context.Context) error {
	return m.manager.Run(ctx)
}

func (m *Manager) RunWithStopErrors(ctx context.Context) (cause error, stopErr error) {
	return m.manager.RunWithStopErrors(ctx)
}

type Option func(*Manager)

func WithManagerOptions(options ...svcinit.Option) Option {
	return func(m *Manager) {
		m.managerOptions = append(m.managerOptions, options...)
	}
}
