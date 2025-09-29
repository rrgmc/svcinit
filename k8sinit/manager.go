package k8sinit

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"syscall"
	"time"

	"github.com/rrgmc/svcinit/v3"
)

type Manager struct {
	manager               *svcinit.Manager
	logger                *slog.Logger
	managerOptions        []svcinit.Option
	healthHandler         HealthHandler
	healthTask            svcinit.Task
	shutdownTimeout       time.Duration
	teardownTimeout       time.Duration
	disableSignalHandling bool
}

func New(options ...Option) (*Manager, error) {
	ret := &Manager{
		logger:          slog.New(slog.DiscardHandler),
		shutdownTimeout: time.Second * 20,
		teardownTimeout: time.Second * 5,
	}
	for _, option := range options {
		option(ret)
	}

	managerOptions := slices.Concat(ret.managerOptions,
		[]svcinit.Option{
			svcinit.WithStages(allStages...),
			svcinit.WithEnforceShutdownTimeout(true),
			svcinit.WithShutdownTimeout(ret.shutdownTimeout),
			svcinit.WithTeardownTimeout(ret.teardownTimeout),
		},
	)

	var err error
	ret.manager, err = svcinit.New(managerOptions...)
	if err != nil {
		return nil, err
	}

	err = ret.initHealth()
	if err != nil {
		return nil, err
	}

	if !ret.disableSignalHandling {
		ret.AddTask(StageManagement, svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))
	}

	return ret, nil
}

func (m *Manager) HealthHandler() HealthHandler {
	return m.healthHandler
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

func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		m.logger = logger
		m.managerOptions = append(m.managerOptions, svcinit.WithLogger(logger))
	}
}

// WithHealthHandler sets the HealthHandler.
func WithHealthHandler(h HealthHandler) Option {
	return func(manager *Manager) {
		manager.healthHandler = h
	}
}

// WithHealthHandlerTask sets the HealthHandler which is also a [svcinit.Task] to be initialized.
func WithHealthHandlerTask(h HealthHandlerTask) Option {
	return func(manager *Manager) {
		manager.healthHandler = h
		manager.healthTask = h
	}
}

// WithHealthTask sets a [svcinit.Task] to be started in the corresponding stage.
// It does NOT sets a HealthHandler, it must be set separately.
func WithHealthTask(h svcinit.Task) Option {
	return func(manager *Manager) {
		manager.healthTask = h
	}
}

// WithShutdownTimeout sets a shutdown timeout. The default is 20 seconds.
// If less then or equal to 0, no shutdown timeout will be set.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithTeardownTimeout sets a teardown timeout.
// If less then or equal to 0, makes it continue using the timeout set for shutdown instead of creating a new one.
// The default is 5 seconds.
func WithTeardownTimeout(teardownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.teardownTimeout = teardownTimeout
	}
}

func WithManagerOptions(options ...svcinit.Option) Option {
	return func(m *Manager) {
		m.managerOptions = append(m.managerOptions, options...)
	}
}

func withDisableSignalHandling() Option {
	return func(m *Manager) {
		m.disableSignalHandling = true
	}
}
