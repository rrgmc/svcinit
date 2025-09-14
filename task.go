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

type StopTask interface {
	stopTask() Task
}

// WrappedTask is a task which was wrapped from one [Task]s.
type WrappedTask interface {
	Task
	WrappedTask() Task
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

// TaskCallback is called before and after the task is run.
type TaskCallback interface {
	BeforeRun(ctx context.Context, task Task)
	AfterRun(ctx context.Context, task Task, err error)
}

// TaskCallbackFunc is called before and after the task is run.
func TaskCallbackFunc(beforeRun func(ctx context.Context, task Task),
	afterRun func(ctx context.Context, task Task, err error)) TaskCallback {
	return taskCallbackFunc{
		beforeRun: beforeRun,
		afterRun:  afterRun,
	}
}

// ServiceAsTask creates and adapter from a service method to a task.
func ServiceAsTask(svc Service, isStart bool) Task {
	return &serviceTask{svc: svc, isStart: isStart}
}

// ServiceAsTasks creates and adapter from a service method to stop and start tasks.
func ServiceAsTasks(svc Service) (start, stop Task) {
	return ServiceAsTask(svc, true), ServiceAsTask(svc, false)
}

type MultipleTaskBuilder interface {
	StopManualTask(task StopTask)
	StopTask(task Task)
	StopTaskFunc(task TaskFunc)
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
