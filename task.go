package svcinit

import (
	"context"
)

type Task interface {
	Run(ctx context.Context) error
}

type TaskFunc func(ctx context.Context) error

func (fn TaskFunc) Run(ctx context.Context) error {
	return fn(ctx)
}

// StopFuture is a stop task to be scheduled using [Manager.StopFuture].
type StopFuture interface {
	stopTask() Task
}

// TaskWithID is a task which has an ID.
type TaskWithID interface {
	Task
	TaskID() any
}

// WrappedTask is a task which was wrapped from one [Task]s.
type WrappedTask interface {
	Task
	WrappedTask() Task
}

// WrappedService is a service which was wrapped from one [Service]s.
type WrappedService interface {
	Service
	WrappedService() Service
}

// Service is task with start and stop methods.
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ServiceWithID is a Service that has an ID.
type ServiceWithID interface {
	Service
	ServiceID() any
}

// ServiceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type ServiceTask interface {
	Task
	Service() Service
	IsStart() bool
}

// ServiceFunc returns a Service from start and stop tasks.
// The passed tasks will NOT be returned in task callbacks, it is an internal detail only.
func ServiceFunc(start, stop func(ctx context.Context) error) Service {
	return &serviceFunc{start: start, stop: stop}
}

// TaskCallback is called before and after the task is run.
// Tasks derived from services will call using  a ServiceTask.
// Callbacks ALWAYS receives unwrapped tasks (with UnwrapTask).
type TaskCallback interface {
	BeforeRun(ctx context.Context, task Task, isStart bool)
	AfterRun(ctx context.Context, task Task, isStart bool, err error)
}

// TaskCallbackFunc is called before and after the task is run.
func TaskCallbackFunc(beforeRun func(ctx context.Context, task Task, isStart bool),
	afterRun func(ctx context.Context, task Task, isStart bool, err error)) TaskCallback {
	return taskCallbackFunc{
		beforeRun: beforeRun,
		afterRun:  afterRun,
	}
}

// ServiceAsTask creates and adapter from a service method to a task.
func ServiceAsTask(svc Service, isStart bool) Task {
	ret := &serviceTask{svc: svc, isStart: isStart}
	if sid, ok := svc.(ServiceWithID); ok {
		return &serviceTaskWithID{serviceTask: ret, id: sid.ServiceID()}
	}
	return ret
}

// ServiceAsTasks creates and adapter from a service method to stop and start tasks.
func ServiceAsTasks(svc Service) (start, stop Task) {
	return ServiceAsTask(svc, true), ServiceAsTask(svc, false)
}

// WrapTaskWithID wraps a Task as a TaskWithID.
// Note: it DOES NOT implements WrappedTask.
func WrapTaskWithID(id any, task Task) *WrappedTaskWithID {
	return &WrappedTaskWithID{task: task, id: id}
}

// WrapServiceWithID wraps a Service as a ServiceWithID.
// Note: it DOES NOT implements WrappedService.
func WrapServiceWithID(id any, svc Service) *WrappedServiceWithID {
	return &WrappedServiceWithID{svc: svc, id: id}
}

type MultipleTaskBuilder interface {
	StopTask(task Task)
	StopFuture(task StopFuture)
}

// NewMultipleTask creates a Task from multiple tasks. The tasks will be run in parallel.
func NewMultipleTask(tasks ...Task) Task {
	var t []taskWrapper
	for _, task := range tasks {
		t = append(t, newTaskWrapper(nil, task))
	}
	return newMultipleTask(t...)
}

// WrapTask wraps a task in a WrappedTask, allowing the handler to be customized.
func WrapTask(task Task, options ...WrapTaskOption) Task {
	if task == nil {
		return task
	}
	ret := &wrappedTask{
		task: task,
	}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type WrapTaskOption func(task *wrappedTask)

// WithWrapTaskHandler sets an optional handler for the task.
func WithWrapTaskHandler(handler func(ctx context.Context, task Task) error) WrapTaskOption {
	return func(task *wrappedTask) {
		task.handler = handler
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

// WrapService wraps a service in a WrappedService, allowing the handler to be customized.
func WrapService(service Service, options ...WrapServiceOption) Service {
	if service == nil {
		return service
	}
	ret := &wrappedService{
		svc: service,
	}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type WrapServiceOption func(service *wrappedService)

// WithWrapServiceStartHandler sets an optional start handler for the service.
func WithWrapServiceStartHandler(handler func(ctx context.Context, service Service) error) WrapServiceOption {
	return func(service *wrappedService) {
		service.startHandler = handler
	}
}

// WithWrapServiceStopHandler sets an optional start handler for the service.
func WithWrapServiceStopHandler(handler func(ctx context.Context, service Service) error) WrapServiceOption {
	return func(service *wrappedService) {
		service.stopHandler = handler
	}
}

// UnwrapService unwraps WrappedService from services.
func UnwrapService(svc Service) Service {
	for {
		if svc == nil {
			return nil
		}
		if tc, ok := svc.(WrappedService); ok {
			svc = tc.WrappedService()
		} else {
			return svc
		}
	}
}
