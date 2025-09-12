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

type ParallelStopTask struct {
	tasks    []Task
	resolved resolved
}

var _ pendingStopTask = (*ParallelStopTask)(nil)

func NewParallelStopTask(tasks ...Task) Task {
	return &ParallelStopTask{
		tasks:    tasks,
		resolved: newResolved(),
	}
}

func (t *ParallelStopTask) Run(ctx context.Context) error {
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

func (t *ParallelStopTask) isResolved() bool {
	return t.resolved.isResolved()
}

func (t *ParallelStopTask) setResolved() {
	for _, st := range t.tasks {
		if ps, ok := st.(pendingStopTask); ok {
			ps.setResolved()
		}
	}
	t.resolved.setResolved()
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
