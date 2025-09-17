package svcinit

import (
	"context"
	"sync"
)

// type taskCallbackFunc struct {
// 	beforeRun func(ctx context.Context, task Task, stage Stage)
// 	afterRun  func(ctx context.Context, task Task, stage Stage, err error)
// }
//
// func (t taskCallbackFunc) BeforeRun(ctx context.Context, task Task, stage Stage) {
// 	if t.beforeRun != nil {
// 		t.beforeRun(ctx, task, stage)
// 	}
// }
//
// func (t taskCallbackFunc) AfterRun(ctx context.Context, task Task, stage Stage, err error) {
// 	if t.afterRun != nil {
// 		t.afterRun(ctx, task, stage, err)
// 	}
// }

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
	runWithCallbacks(ctx context.Context, stage Stage, callbacks ...TaskCallback) error
}

// serviceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type serviceTask struct {
	svc   Service
	stage Stage
}

var _ ServiceTask = (*serviceTask)(nil)

func (s *serviceTask) Service() Service {
	return s.svc
}

func (s *serviceTask) Stage() Stage {
	return s.stage
}

func (s *serviceTask) Run(ctx context.Context) error {
	return s.svc.RunService(ctx, s.stage)
}

type serviceTaskWithID struct {
	*serviceTask
	id any
}

var _ ServiceTask = (*serviceTaskWithID)(nil)
var _ TaskWithID = (*serviceTaskWithID)(nil)

func (s *serviceTaskWithID) TaskID() any {
	return s.id
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
	return t.runWithCallbacks(ctx, StageStart)
}

func (t *multipleTask) runWithCallbacks(ctx context.Context, stage Stage, callbacks ...TaskCallback) error {
	allErr := newMultiErrorBuilder()

	var wg sync.WaitGroup
	for _, st := range t.tasks {
		wg.Go(func() {
			err := st.run(ctx, stage, callbacks...)
			if err != nil {
				allErr.add(err)
			}
		})
	}

	wg.Wait()

	return allErr.build()
}

type multipleTaskBuilder struct {
	stopFuture func(task StopFuture)
	stop       func(task Task)
}

func (m *multipleTaskBuilder) StopFuture(task StopFuture) {
	m.stopFuture(task)
}

func (m *multipleTaskBuilder) StopTask(task Task) {
	m.stop(task)
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
	svc     Service
	handler func(ctx context.Context, svc Service, stage Stage) error
}

var _ WrappedService = (*wrappedService)(nil)

func (t *wrappedService) RunService(ctx context.Context, stage Stage) error {
	if t.handler == nil {
		return t.svc.RunService(ctx, stage)
	}
	return t.handler(ctx, t.svc, stage)
}

func (t *wrappedService) WrappedService() Service {
	return t.svc
}

type WrappedTaskWithID struct {
	task Task
	id   any
}

var _ TaskWithID = (*WrappedTaskWithID)(nil)

func (t *WrappedTaskWithID) TaskID() any {
	return t.id
}

func (t *WrappedTaskWithID) Task() Task {
	return t.task
}

func (t *WrappedTaskWithID) Run(ctx context.Context) error {
	return t.task.Run(ctx)
}

type WrappedServiceWithID struct {
	svc Service
	id  any
}

var _ ServiceWithID = (*WrappedServiceWithID)(nil)

func (s *WrappedServiceWithID) Service() any {
	return s.svc
}

func (s *WrappedServiceWithID) ServiceID() any {
	return s.id
}

func (s *WrappedServiceWithID) RunService(ctx context.Context, stage Stage) error {
	return s.svc.RunService(ctx, stage)
}
