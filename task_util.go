package svcinit

import (
	"context"
	"errors"
	"sync"
)

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

// multipleTask runs multiple tasks in parallel, wrapped in a single Task.
type multipleTask struct {
	tasks    []Task
	resolved resolved
}

var _ WrappedTasks = (*multipleTask)(nil)

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
