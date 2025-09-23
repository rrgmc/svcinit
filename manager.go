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

type Manager struct {
	mu                     sync.Mutex
	stages                 []string
	tasks                  *stageTasks
	shutdownTimeout        time.Duration
	enforceShutdownTimeout bool
	managerCallbacks       []ManagerCallback
	taskCallbacks          []TaskCallback
	defaultStage           string
	initData               []string
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
		defaultStage:           StageDefault,
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

func (m *Manager) Stages() []string {
	return m.stages
}

func (m *Manager) IsRunning() bool {
	return m.isRunning.Load()
}

func (m *Manager) AddTask(task Task, options ...TaskOption) {
	if m.isRunning.Load() {
		if m.startupCancel != nil {
			m.startupCancel(fmt.Errorf("%w: cannot add task", ErrAlreadyRunning))
		}
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if task == nil {
		m.addInitError(ErrNilTask)
		return
	}
	tw := m.newTaskWrapper(task, options...)
	if !slices.Contains(m.stages, tw.options.stage) {
		m.addInitError(newInvalidStage(tw.options.stage))
		return
	}
	m.tasks.add(tw)
}

func (m *Manager) AddTaskFunc(f TaskFunc, options ...TaskOption) {
	m.AddTask(f, options...)
}

func (m *Manager) AddService(service Service, options ...TaskOption) {
	m.AddTask(ServiceAsTask(service), options...)
}

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

func (m *Manager) RunWithStopErrors(ctx context.Context, options ...RunOption) (cause error, stopErr error) {
	return m.runWithStopErrors(ctx, options...)
}

type Option func(*Manager)

func WithLogger(logger *slog.Logger) Option {
	return func(m *Manager) {
		m.logger = logger
	}
}

func WithStages(stages ...string) Option {
	return func(m *Manager) {
		m.stages = stages
		if m.defaultStage != "" && !slices.Contains(m.stages, m.defaultStage) {
			if len(m.stages) == 0 {
				m.defaultStage = ""
			} else {
				m.defaultStage = m.stages[0]
			}
		}
	}
}

func WithDefaultStage(stage string) Option {
	return func(m *Manager) {
		m.defaultStage = stage
	}
}

// WithShutdownTimeout sets a shutdown timeout. The default is 10 seconds.
// If less then or equal to 0, no shutdown timeout will be set.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(s *Manager) {
		s.shutdownTimeout = shutdownTimeout
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

func WithManagerCallback(callbacks ...ManagerCallback) Option {
	return func(s *Manager) {
		s.managerCallbacks = append(s.managerCallbacks, callbacks...)
	}
}

func WithTaskCallback(callbacks ...TaskCallback) Option {
	return func(s *Manager) {
		s.taskCallbacks = append(s.taskCallbacks, callbacks...)
	}
}

func WithInitData(names ...string) Option {
	return func(s *Manager) {
		s.initData = append(s.initData, names...)
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
