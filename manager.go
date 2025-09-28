package svcinit

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Manager manages the complete execution lifecycle.
type Manager struct {
	mu                     sync.Mutex
	stages                 []string
	tasks                  *stageTasks
	shutdownTimeout        time.Duration
	teardownTimeout        time.Duration
	enforceShutdownTimeout bool
	beforeRun              []func(ctx context.Context) (context.Context, error)
	afterRun               []func(ctx context.Context, cause error, stopErr error) error
	managerCallbacks       []ManagerCallback
	taskCallbacks          []TaskCallback
	taskErrorHandler       TaskErrorHandler
	logger                 *slog.Logger

	isRunning                     atomic.Bool
	startupCtx, taskDoneCtx       context.Context
	startupCancel, taskDoneCancel context.CancelCauseFunc
	tasksRunning                  sync.WaitGroup
	initErrors                    []error
}

func New(options ...Option) (*Manager, error) {
	ret := &Manager{
		stages:                 []string{StageDefault},
		tasks:                  newStageTasks(),
		shutdownTimeout:        10 * time.Second,
		enforceShutdownTimeout: true,
		logger:                 slog.New(slog.DiscardHandler),
	}
	for _, option := range options {
		option(ret)
	}
	err := ret.init()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Stages returns the stages configured for execution.
func (m *Manager) Stages() []string {
	return m.stages
}

func (m *Manager) IsRunning() bool {
	return m.isRunning.Load()
}

// AddTask add a Task to be executed at the passed stage.
func (m *Manager) AddTask(stage string, task Task, options ...TaskOption) {
	if m.isRunning.Load() {
		if m.startupCancel != nil {
			m.startupCancel(fmt.Errorf("%w: cannot add task", ErrAlreadyRunning))
		}
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if task == nil {
		m.AddInitError(ErrNilTask)
		return
	}
	if te, ok := task.(TaskWithInitError); ok {
		if te.TaskInitError() != nil {
			m.AddInitError(te.TaskInitError())
			return
		}
	}
	tw := newTaskWrapper(task, options...)
	if !slices.Contains(m.stages, stage) {
		m.AddInitError(newInvalidStage(stage))
		return
	}
	m.tasks.add(stage, tw)
}

// AddTaskFunc add a Task to be executed at the passed stage.
func (m *Manager) AddTaskFunc(stage string, f TaskFunc, options ...TaskOption) {
	m.AddTask(stage, f, options...)
}

// AddService add a Service to be executed at the passed stage.
func (m *Manager) AddService(stage string, service Service, options ...TaskOption) {
	m.AddTask(stage, ServiceAsTask(service), options...)
}

// Run executes the initialization and returns the error of the first task stop step that returns.
func (m *Manager) Run(ctx context.Context, options ...RunOption) error {
	cause, _ := m.RunWithStopErrors(ctx, options...)
	return cause
}

// Shutdown starts the shutdown process as if a task finished.
func (m *Manager) Shutdown() {
	if m.startupCancel != nil {
		m.startupCancel(ErrExit)
	}
}

// RunWithStopErrors executes the initialization and returns the error of the first task stop step that returns, and
// also any errors happening during shutdown in a wrapped error.
func (m *Manager) RunWithStopErrors(ctx context.Context, options ...RunOption) (cause error, stopErr error) {
	return m.runWithStopErrors(ctx, options...)
}

type Option func(*Manager)

func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		m.logger = logger
	}
}

// WithStages sets the initialization stages.
// The default value is "[StageDEFAULT]".
func WithStages(stages ...string) Option {
	return func(m *Manager) {
		m.stages = stages
	}
}

// WithShutdownTimeout sets a shutdown timeout. The default is 10 seconds.
// If less then or equal to 0, no shutdown timeout will be set.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.shutdownTimeout = shutdownTimeout
	}
}

// WithTeardownTimeout sets a teardown timeout.
// If less then or equal to 0, makes it continue using the timeout set for shutdown instead of creating a new one.
// The default is 0.
func WithTeardownTimeout(teardownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.teardownTimeout = teardownTimeout
	}
}

// WithEnforceShutdownTimeout don't wait for all shutdown tasks to complete if they are over the shutdown timeout.
// Usually the shutdown timeout only sets a timeout in the context, but it can't guarantee that all tasks will follow it.
// Default is true.
func WithEnforceShutdownTimeout(enforceShutdownTimeout bool) Option {
	return func(s *Manager) {
		s.enforceShutdownTimeout = enforceShutdownTimeout
	}
}

// WithBeforeRun adds a callback to be executed before stages are run.
// Return a changed context, or the same one received.
// Any error that is returned will abort the [Manager.Run] execution with the passed error.
func WithBeforeRun(beforeRun func(ctx context.Context) (context.Context, error)) Option {
	return func(s *Manager) {
		s.beforeRun = append(s.beforeRun, beforeRun)
	}
}

// WithAfterRun adds a callback to be executed after all stages run.
// The returned error will be the cause returned from [Manager.Run]. Return the same cause parameter as error to
// keep it.
func WithAfterRun(afterRun func(ctx context.Context, cause error, stopErr error) error) Option {
	return func(s *Manager) {
		s.afterRun = append(s.afterRun, afterRun)
	}
}

// WithManagerCallback adds a manager callback. Multiple callbacks may be added.
func WithManagerCallback(callbacks ...ManagerCallback) Option {
	return func(s *Manager) {
		s.managerCallbacks = append(s.managerCallbacks, callbacks...)
	}
}

// WithTaskCallback adds a task callback. Multiple callbacks may be added.
func WithTaskCallback(callbacks ...TaskCallback) Option {
	return func(s *Manager) {
		s.taskCallbacks = append(s.taskCallbacks, callbacks...)
	}
}

// WithTaskErrorHandler sets a callback that can change the error returned from a task.
// This can be used for example to ignore errors that are not errors, like [http.ErrServerClosed].
func WithTaskErrorHandler(handler TaskErrorHandler) Option {
	return func(s *Manager) {
		s.taskErrorHandler = handler
	}
}

type RunOption func(options *runOptions)

// WithRunShutdownContext sets a context to use for shutdown.
// If not set, "context.WithoutCancel(baseContext)" will be used.
func WithRunShutdownContext(ctx context.Context) RunOption {
	return func(options *runOptions) {
		options.shutdownCtx = ctx
	}
}
