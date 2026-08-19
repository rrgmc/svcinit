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
	pendingStartupCancel          error // set if Shutdown/AddTask request a cancel before startupCancel exists
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

// IsRunning returns whether [Manager.Run] has been called on this Manager. A Manager is single-use: once
// this returns true it stays true forever, even after Run has returned, and any later call to Run always
// fails with [ErrAlreadyRunning].
func (m *Manager) IsRunning() bool {
	return m.isRunning.Load()
}

// AddTask add a Task to be executed at the passed stage.
func (m *Manager) AddTask(stage string, task Task, options ...TaskOption) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isRunning.Load() {
		m.requestStartupCancel(fmt.Errorf("%w: cannot add task", ErrAlreadyRunning))
		return
	}
	if task == nil {
		m.addInitError(ErrNilTask)
		return
	}
	if te, ok := task.(TaskWithInitError); ok {
		if te.TaskInitError() != nil {
			m.addInitError(te.TaskInitError())
			return
		}
	}
	tw := newTaskWrapper(task, options...)
	if !slices.Contains(m.stages, stage) {
		m.addInitError(newInvalidStage(stage))
		return
	}
	m.tasks.add(stage, tw)
}

// InitAddTask initialize and add a Task to be executed at the passed stage.
func (m *Manager) InitAddTask[T Task](stage string, init func() T, options ...TaskOption) T {
	task := init()
	m.AddTask(stage, task, options...)
	return task
}

// InitCheckAddTask initialize and add a Task to be executed at the passed stage.
func (m *Manager) InitCheckAddTask[T Task](stage string, init func() (T, error), options ...TaskOption) (T, error) {
	task, err := init()
	if err != nil {
		return task, err
	}
	m.AddTask(stage, task, options...)
	return task, nil
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
//
// If a stage's setup step fails, Run returns immediately without running any later stage at all —
// including their stop and teardown steps. Cleanup for resources acquired in stages before the failing
// one still runs during shutdown, but stages after the failure point never execute any step.
func (m *Manager) Run(ctx context.Context, options ...RunOption) error {
	cause, _ := m.RunWithStopErrors(ctx, options...)
	return cause
}

// Shutdown starts the shutdown process as if a task finished. It is safe to call from a goroutine other
// than the one calling Run, including before Run has finished setting up.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestStartupCancel(ErrExit)
}

// requestStartupCancel cancels the startup context with cause if it has already been created by Run,
// otherwise records cause so it is applied as soon as Run creates it. Must be called with m.mu held.
func (m *Manager) requestStartupCancel(cause error) {
	if m.startupCancel != nil {
		m.startupCancel(cause)
		return
	}
	if m.pendingStartupCancel == nil {
		m.pendingStartupCancel = cause
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
