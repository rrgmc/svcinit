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

// StopTask is a task meant for stopping other tasks.
type StopTask interface {
	Task
	Stop(ctx context.Context) error
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

// ServiceStartTask adapts a Service start method to a Task.
func ServiceStartTask(svc Service) Task {
	return TaskFunc(func(ctx context.Context) error {
		return svc.Start(ctx)
	})
}

// ServiceStopTask adapts a Service stop method to a Task.
func ServiceStopTask(svc Service) Task {
	return TaskFunc(func(ctx context.Context) error {
		return svc.Stop(ctx)
	})
}

type StopTaskFunc func(ctx context.Context) error

func (sf StopTaskFunc) Stop(ctx context.Context) error {
	return sf(ctx)
}

func (sf StopTaskFunc) Run(ctx context.Context) error {
	return sf.Stop(ctx)
}

type ParallelStopTask struct {
	tasks    []StopTask
	resolved resolved
}

var _ pendingStopTask = (*ParallelStopTask)(nil)

func NewParallelStopTask(tasks ...StopTask) StopTask {
	return &ParallelStopTask{
		tasks:    tasks,
		resolved: newResolved(),
	}
}

func (t *ParallelStopTask) Run(ctx context.Context) error {
	return t.Stop(ctx)
}

func (t *ParallelStopTask) Stop(ctx context.Context) error {
	var m sync.Mutex
	var allErr []error

	var wg sync.WaitGroup
	for _, st := range t.tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := st.Stop(ctx)
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
