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

// type ServiceToTask interface {
// 	Service
// 	ToTask(isStart bool) Task
// }

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
func ServiceAsTask(svc Service, isStart bool) (st ServiceTask) {
	st = &serviceTask{svc: svc, isStart: isStart}
	if sc, ok := svc.(*serviceWithCallback); ok {
		st = &serviceTaskWithCallback{svc: st, callback: sc.callback}
	}
	return st
}

// ServiceAsTasks creates and adapter from a service method to stop and start tasks.
func ServiceAsTasks(svc Service) (start, stop ServiceTask) {
	return ServiceAsTask(svc, true), ServiceAsTask(svc, false)
}

type MultipleTaskBuilder interface {
	StopManualTask(task StopTask)
	StopTask(task Task)
	StopTaskFunc(task TaskFunc)
}

func NewMultipleTask(tasks ...Task) Task {
	var t []taskWrapper
	for _, task := range tasks {
		t = append(t, newTaskWrapper(nil, task))
	}
	return newMultipleTask(t...)
}

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
