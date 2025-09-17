package svcinit

import (
	"context"
)

type Stage int

const (
	StageStart Stage = iota
	StageStop
	StagePreStop
)

type Step int

const (
	StepBefore Step = iota
	StepAfter
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

// TaskFuncWithID returns a TaskWithID from a TaskFunc.
func TaskFuncWithID(id any, fn TaskFunc) TaskWithID {
	return &WrappedTaskWithID{
		task: fn,
		id:   id,
	}
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

// Service is task with multiple stages.
type Service interface {
	RunService(ctx context.Context, stage Stage) error
}

// ServiceFunc is a functional implementation of Service.
type ServiceFunc func(ctx context.Context, stage Stage) error

func (f ServiceFunc) RunService(ctx context.Context, stage Stage) error {
	return f(ctx, stage)
}

// ServiceWithID is a Service that has an ID.
type ServiceWithID interface {
	Service
	ServiceID() any
}

// ServiceFuncWithID returns a ServiceWithID from a ServiceFunc.
func ServiceFuncWithID(id any, svc ServiceFunc) ServiceWithID {
	return &WrappedServiceWithID{
		svc: svc,
		id:  id,
	}
}

// ServiceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type ServiceTask interface {
	Task
	Service() Service
	Stage() Stage
}

// TaskCallback is called before and after the task is run.
// Tasks derived from services will call using  a ServiceTask.
// Callbacks ALWAYS receives unwrapped tasks (with UnwrapTask).
// The err parameter is only set if Step == StepAfter.
type TaskCallback interface {
	Callback(ctx context.Context, task Task, stage Stage, step Step, err error)
}

type TaskCallbackFunc func(ctx context.Context, task Task, stage Stage, step Step, err error)

func (f TaskCallbackFunc) Callback(ctx context.Context, task Task, stage Stage, step Step, err error) {
	f(ctx, task, stage, step, err)
}

// ServiceAsTask creates and adapter from a service method to a task.
func ServiceAsTask(svc Service, stage Stage) Task {
	ret := &serviceTask{svc: svc, stage: stage}
	if sid, ok := svc.(ServiceWithID); ok {
		return &serviceTaskWithID{serviceTask: ret, id: sid.ServiceID()}
	}
	return ret
}

// ServiceAsTasks creates and adapter from a service method to stop and start tasks.
func ServiceAsTasks(svc Service) (start, preStop, stop Task) {
	return ServiceAsTask(svc, StageStart),
		ServiceAsTask(svc, StagePreStop),
		ServiceAsTask(svc, StageStop)
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
// If service or handler is nil, the unmodified service will be returned.
func WrapService(service Service, handler func(ctx context.Context, svc Service, stage Stage) error) Service {
	if service == nil || handler == nil {
		return service
	}
	return &wrappedService{
		svc:     service,
		handler: handler,
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
