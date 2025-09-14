package svcinit

import (
	"context"
	"errors"
	"sync"
)

type Task interface {
	Run(ctx context.Context) error
}

type TaskFunc func(ctx context.Context) error

func (fn TaskFunc) Run(ctx context.Context) error {
	return fn(ctx)
}

type StopTask interface {
	isStopTask()
}

// WrappedTask is a task which was wrapped from one [Task]s.
type WrappedTask interface {
	Task
	WrappedTask() Task
}

// WrappedTasks is a task which was wrapped from one or more [Task]s.
type WrappedTasks interface {
	Task
	WrappedTasks() []Task
}

// Service is task with start and stop methods.
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ServiceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type ServiceTask interface {
	Task
	Service() Service
	IsStart() bool
}

// ServiceFunc returns a Service from start and stop tasks.
func ServiceFunc(start, stop Task) Service {
	return &serviceFunc{start: start, stop: stop}
}

// ServiceTaskFunc returns a Service from start and stop tasks.
func ServiceTaskFunc(start, stop TaskFunc) Service {
	return ServiceFunc(start, stop)
}

type ServiceToTask interface {
	Service
	ToTask(isStart bool) Task
}

// ServiceAsTask creates and adapter from a service method to a task.
func ServiceAsTask(svc Service, isStart bool) Task {
	if stt, ok := svc.(ServiceToTask); ok {
		return stt.ToTask(isStart)
	}
	return &serviceTask{svc: svc, isStart: isStart}
}

// ServiceAsTasks creates and adapter from a service method to stop and start tasks.
func ServiceAsTasks(svc Service) (start, stop Task) {
	return ServiceAsTask(svc, true), ServiceAsTask(svc, false)
}

// ServiceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type serviceTask struct {
	svc     Service
	isStart bool
}

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

type MultipleTaskBuilder interface {
	StopManualTask(task StopTask)
	StopTask(task Task)
	StopTaskFunc(task TaskFunc)
}

// multipleTask runs multiple tasks in parallel, wrapped in a single Task.
type multipleTask struct {
	tasks    []Task
	resolved resolved
}

var _ WrappedTasks = (*multipleTask)(nil)

func NewMultipleTask(tasks ...Task) Task {
	return newMultipleTask(tasks...)
}

func newMultipleTask(tasks ...Task) Task {
	return &multipleTask{
		tasks:    tasks,
		resolved: newResolved(),
	}
}

func (t *multipleTask) WrappedTasks() []Task {
	return t.tasks
}

func (t *multipleTask) Run(ctx context.Context) error {
	allErr := newMultiErrorBuilder()

	var wg sync.WaitGroup
	for _, st := range t.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.Run(ctx)
			if err != nil {
				allErr.add(err)
			}
		}()
	}

	wg.Wait()

	return allErr.build()
}

// func (t *multipleTask) isResolved() bool {
// 	return t.resolved.isResolved()
// }
//
// func (t *multipleTask) setResolved() {
// 	for _, st := range t.tasks {
// 		if ps, ok := st.(*pendingStopTask); ok {
// 			ps.setResolved()
// 		}
// 	}
// 	t.resolved.setResolved()
// }

// TaskWithCallback wraps a service with a callback to be called before and after it runs.
func TaskWithCallback(task Task, callback TaskCallback) Task {
	if task == nil || callback == nil {
		return task
	}
	return &taskWithCallback{
		task:     task,
		callback: callback,
	}
}

// TaskFuncWithCallback wraps a task with a callback to be called before and after it runs.
func TaskFuncWithCallback(task TaskFunc, callback TaskCallback) Task {
	return TaskWithCallback(task, callback)
}

// ServiceWithCallback wraps a Service with a callback to be called before and after it runs.
func ServiceWithCallback(service Service, callback TaskCallback) Service {
	return &serviceWithCallback{
		svc:      service,
		callback: callback,
	}
}

// UnwrapTask unwraps WrappedTask from tasks.
func UnwrapTask(task Task) Task {
	for {
		if task == nil {
			return nil
		}
		if tc, ok := task.(WrappedTask); ok {
			task = tc.WrappedTask()
		} else {
			return task
		}
	}
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
	if t.callback != nil {
		t.callback.BeforeRun(ctx, t.task)
	}
	err := t.task.Run(ctx)
	if t.callback != nil {
		t.callback.AfterRun(ctx, t.task, err)
	}
	return err
}

type serviceWithCallback struct {
	svc      Service
	callback TaskCallback
}

var _ Service = (*serviceWithCallback)(nil)

func (s *serviceWithCallback) Start(ctx context.Context) error {
	return errors.New("this should never run")
	// return s.svc.Start(ctx)
}

func (s *serviceWithCallback) Stop(ctx context.Context) error {
	return errors.New("this should never run")
	// return s.svc.Stop(ctx)
}

func (s *serviceWithCallback) ToTask(isStart bool) (tt Task) {
	if stt, ok := s.svc.(ServiceToTask); ok {
		tt = stt.ToTask(isStart)
	} else {
		tt = &serviceTask{
			svc:     s.svc,
			isStart: isStart,
		}
	}
	if s.callback != nil {
		tt = TaskWithCallback(tt, s.callback)
	}
	return
}

// taskFromCallback unwraps taskWithCallback from tasks.
func taskFromCallback(task Task) Task {
	for {
		if tc, ok := task.(*taskWithCallback); ok {
			task = tc.task
		} else {
			return task
		}
	}
}

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
