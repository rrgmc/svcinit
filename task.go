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

// WrappedTask is a task which was wrapped from one or more [Task]s.
type WrappedTask interface {
	Task
	WrappedTasks() []Task
}

// Service is task with start and stop methods.
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ServiceFunc returns a Service from start and stop tasks.
func ServiceFunc(start, stop Task) Service {
	return &serviceFunc{start: start, stop: stop}
}

// ServiceTaskFunc returns a Service from start and stop tasks.
func ServiceTaskFunc(start, stop TaskFunc) Service {
	return ServiceFunc(start, stop)
}

// ServiceAsTask creates and adapter from a service method to a task.
func ServiceAsTask(svc Service, isStart bool) *ServiceTask {
	return &ServiceTask{svc: svc, isStart: isStart}
}

// ServiceTask is a Task implemented from a Service.
// Use Service to get the source service instance.
type ServiceTask struct {
	svc     Service
	isStart bool
}

func (s *ServiceTask) Service() Service {
	return s.svc
}

func (s *ServiceTask) IsStart() bool {
	return s.isStart
}

func (s *ServiceTask) Run(ctx context.Context) error {
	if s.isStart {
		return s.svc.Start(ctx)
	}
	return s.svc.Stop(ctx)
}

// MultipleTask runs multiple tasks in parallel, wrapped in a single Task.
type MultipleTask struct {
	tasks    []Task
	resolved resolved
}

var _ pendingStopTask = (*MultipleTask)(nil)
var _ WrappedTask = (*MultipleTask)(nil)

func NewMultipleTask(tasks ...Task) Task {
	return &MultipleTask{
		tasks:    tasks,
		resolved: newResolved(),
	}
}

func (t *MultipleTask) WrappedTasks() []Task {
	return t.tasks
}

func (t *MultipleTask) Run(ctx context.Context) error {
	var m sync.Mutex
	var allErr []error

	var wg sync.WaitGroup
	for _, st := range t.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.Run(ctx)
			if err != nil {
				m.Lock()
				allErr = append(allErr, err)
				m.Unlock()
			}
		}()
	}

	wg.Wait()

	return errors.Join(allErr...)
}

func (t *MultipleTask) isResolved() bool {
	return t.resolved.isResolved()
}

func (t *MultipleTask) setResolved() {
	for _, st := range t.tasks {
		if ps, ok := st.(pendingStopTask); ok {
			ps.setResolved()
		}
	}
	t.resolved.setResolved()
}

// TaskWithCallback wraps a task with a callback to be called before and after it runs.
func TaskWithCallback(task Task, callback TaskCallback) Task {
	if callback == nil {
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

type taskWithCallback struct {
	task     Task
	callback TaskCallback
}

func (t *taskWithCallback) WrappedTasks() []Task {
	return []Task{t.task}
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
