package svcinit

import (
	"context"
	"errors"
	"sync"
)

type Task func(ctx context.Context) error

// StopTask is a task meant for stopping other tasks.
type StopTask interface {
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

type StopTaskFunc func(ctx context.Context) error

func (sf StopTaskFunc) Stop(ctx context.Context) error {
	return sf(ctx)
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
	start func(ctx context.Context) error
	stop  func(ctx context.Context) error
}

func (sf *serviceFunc) Start(ctx context.Context) error {
	if sf.start == nil {
		return nil
	}
	return sf.start(ctx)
}

func (sf *serviceFunc) Stop(ctx context.Context) error {
	if sf.stop == nil {
		return nil
	}
	return sf.stop(ctx)
}
