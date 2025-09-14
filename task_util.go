package svcinit

import (
	"context"
	"sync"
)

type taskCallbackFunc struct {
	beforeRun func(ctx context.Context, task Task)
	afterRun  func(ctx context.Context, task Task, err error)
}

func (t taskCallbackFunc) BeforeRun(ctx context.Context, task Task) {
	if t.beforeRun != nil {
		t.beforeRun(ctx, task)
	}
}

func (t taskCallbackFunc) AfterRun(ctx context.Context, task Task, err error) {
	if t.afterRun != nil {
		t.afterRun(ctx, task, err)
	}
}

func joinTaskCallbacks(callbacks ...[]TaskCallback) []TaskCallback {
	var ret []TaskCallback
	for _, callbackList := range callbacks {
		for _, callback := range callbackList {
			if callback != nil {
				ret = append(ret, callback)
			}
		}
	}
	return ret
}

// taskRunCallback signals that the task can handle callback execution itself.
type taskRunCallback interface {
	Task
	runWithCallbacks(ctx context.Context, callbacks ...TaskCallback) error
}

// serviceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type serviceTask struct {
	svc     Service
	isStart bool
}

var _ ServiceTask = (*serviceTask)(nil)

func (s *serviceTask) Service() Service {
	return s.svc
}

func (s *serviceTask) IsStart() bool {
	return s.isStart
}

func (s *serviceTask) Run(ctx context.Context) error {
	if s.isStart {
		return s.svc.Start(ctx)
	}
	return s.svc.Stop(ctx)
}

// multipleTask runs multiple tasks in parallel, wrapped in a single Task.
type multipleTask struct {
	tasks    []taskWrapper
	resolved resolved
}

var _ taskRunCallback = (*multipleTask)(nil)

func newMultipleTask(tasks ...taskWrapper) Task {
	return &multipleTask{
		tasks:    tasks,
		resolved: newResolved(),
	}
}

func (t *multipleTask) Run(ctx context.Context) error {
	return t.runWithCallbacks(ctx)
}

func (t *multipleTask) runWithCallbacks(ctx context.Context, callbacks ...TaskCallback) error {
	allErr := newMultiErrorBuilder()

	var wg sync.WaitGroup
	for _, st := range t.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.run(ctx, callbacks...)
			if err != nil {
				allErr.add(err)
			}
		}()
	}

	wg.Wait()

	return allErr.build()
}

type taskWithCallback struct {
	task     Task
	callback TaskCallback
}

var _ Task = (*taskWithCallback)(nil)
var _ WrappedTask = (*taskWithCallback)(nil)

func (t *taskWithCallback) WrappedTask() Task {
	return t.task
}

func (t *taskWithCallback) Run(ctx context.Context) error {
	return runTask(ctx, t.task, t.callback)
}

// type serviceWithCallback struct {
// 	svc      Service
// 	callback TaskCallback
// }
//
// var _ Service = (*serviceWithCallback)(nil)
//
// func (s *serviceWithCallback) Start(ctx context.Context) error {
// 	return s.svc.Start(ctx)
// }
//
// func (s *serviceWithCallback) Stop(ctx context.Context) error {
// 	return s.svc.Stop(ctx)
// }

type serviceFunc struct {
	start Task
	stop  Task
}

func (sf *serviceFunc) Start(ctx context.Context) error {
	if sf.start == nil {
		return nil
	}
	return sf.start.Run(ctx)
}

func (sf *serviceFunc) Stop(ctx context.Context) error {
	if sf.stop == nil {
		return nil
	}
	return sf.stop.Run(ctx)
}

type multipleTaskBuilder struct {
	stopManualTask func(task StopTask)
	stopTask       func(task Task)
}

func (m *multipleTaskBuilder) StopManualTask(task StopTask) {
	m.stopManualTask(task)
}

func (m *multipleTaskBuilder) StopTask(task Task) {
	m.stopTask(task)
}

func (m *multipleTaskBuilder) StopTaskFunc(task TaskFunc) {
	m.stopTask(task)
}

type wrappedTask struct {
	task    Task
	handler func(ctx context.Context, task Task) error
}

var _ WrappedTask = (*wrappedTask)(nil)

func (t *wrappedTask) Run(ctx context.Context) error {
	if t.handler == nil {
		return t.task.Run(ctx)
	}
	return t.handler(ctx, t.task)
}

func (t *wrappedTask) WrappedTask() Task {
	return t.task
}

type wrappedService struct {
	svc          Service
	startHandler func(ctx context.Context, svc Service) error
	stopHandler  func(ctx context.Context, svc Service) error
}

var _ Service = (*wrappedService)(nil)

// var _ WrappedService = (*wrappedService)(nil)

func (t *wrappedService) Start(ctx context.Context) error {
	if t.startHandler == nil {
		return t.svc.Start(ctx)
	}
	return t.startHandler(ctx, t.svc)
}

func (t *wrappedService) Stop(ctx context.Context) error {
	if t.stopHandler == nil {
		return t.svc.Stop(ctx)
	}
	return t.stopHandler(ctx, t.svc)
}

func (t *wrappedService) WrappedService() Service {
	return t.svc
}
