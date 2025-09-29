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
	healthHandler         svcinit.HealthHandler
	healthTask            svcinit.Task
	telemetryHandler      TelemetryHandler
	telemetryTask         svcinit.Task
	shutdownTimeout       time.Duration
	teardownTimeout       time.Duration
	handleSignals         []os.Signal
	disableSignalHandling bool
}

func New(options ...Option) (*Manager, error) {
	ret := &Manager{
		logger:          slog.New(slog.DiscardHandler),
		handleSignals:   []os.Signal{os.Interrupt, syscall.SIGTERM},
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

	if !ret.disableSignalHandling && len(ret.handleSignals) > 0 {
		ret.AddTask(StageManagement, svcinit.SignalTask(ret.handleSignals...))
	}

	return ret, nil
}

// HealthHandler returns the svcinit.HealthHandler being used.
// It is never nil.
func (m *Manager) HealthHandler() svcinit.HealthHandler {
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

// Run executes the initialization and returns the error of the first task stop step that returns.
func (m *Manager) Run(ctx context.Context) error {
	return m.manager.Run(ctx)
}

// Shutdown starts the shutdown process as if a task finished.
func (m *Manager) Shutdown(ctx context.Context) {
	m.manager.Shutdown()
}

// RunWithStopErrors executes the initialization and returns the error of the first task stop step that returns, and
// also any errors happening during shutdown in a wrapped error.
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

// WithHealthHandler sets the [svcinit.HealthHandler].
func WithHealthHandler(h svcinit.HealthHandler) Option {
	return func(manager *Manager) {
		manager.healthHandler = h
	}
}

// WithHealthHandlerTask sets the [svcinit.HealthHandler] which is also a [svcinit.Task] to be initialized.
func WithHealthHandlerTask(h HealthHandlerTask) Option {
	return func(manager *Manager) {
		manager.healthHandler = h
		manager.healthTask = h
	}
}

// WithHealthTask sets a [svcinit.Task] to be started in the corresponding stage.
// It does NOT set a [svcinit.HealthHandler], it must be set separately.
func WithHealthTask(h svcinit.Task) Option {
	return func(manager *Manager) {
		manager.healthTask = h
	}
}

// WithTelemetryHandler sets the TelemetryHandler.
func WithTelemetryHandler(h TelemetryHandler) Option {
	return func(manager *Manager) {
		manager.telemetryHandler = h
	}
}

// WithTelemetryHandlerTask sets the TelemetryHandler which is also a [svcinit.Task] to be initialized.
func WithTelemetryHandlerTask(h TelemetryHandlerTask) Option {
	return func(manager *Manager) {
		manager.telemetryHandler = h
		manager.telemetryTask = h
	}
}

// WithTelemetryTask sets a [svcinit.Task] to be started in the corresponding stage.
// It does NOT set a TelemetryHandler, it must be set separately.
func WithTelemetryTask(h svcinit.Task) Option {
	return func(manager *Manager) {
		manager.telemetryTask = h
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

// WithManagerOptions sets [svcinit.Manager] options manually. Some options are required and can't be overridden.
func WithManagerOptions(options ...svcinit.Option) Option {
	return func(m *Manager) {
		m.managerOptions = append(m.managerOptions, options...)
	}
}

// WithHandleSignals sets the OS signals to be handled.
// The default is "{os.Interrupt, syscall.SIGTERM}".
func WithHandleSignals(signals ...os.Signal) Option {
	return func(m *Manager) {
		m.handleSignals = signals
	}
}

// WithDisableSignalHandling disables signal handling.
func WithDisableSignalHandling() Option {
	return func(m *Manager) {
		m.disableSignalHandling = true
	}
}
