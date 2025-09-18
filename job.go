package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task which don't need a stop task.
// The context passed to the task will be canceled on stop.
// It is equivalent to "StartTask().AutoStop()".
// The task is only executed at the Run call.
func (s *Manager) ExecuteTask(task Task, options ...TaskOption) {
	s.addTask(s.unorderedCancelCtx, task, options...)
}

// StartTask executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *Manager) StartTask(task Task, options ...TaskOption) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    task,
		options:  options,
		preStop:  newValuePtr[Task](),
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartService executes a service task and allows the shutdown method to be customized.
// A service is a task with StartTask and StopTask methods.
// At least one method of StartServiceCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *Manager) StartService(svc Service, options ...TaskOption) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// PreStopTask adds a pre-stop task. They will be called in parallel, before stop tasks starts.
func (s *Manager) PreStopTask(task Task, options ...TaskOption) {
	s.preStopTasks = append(s.preStopTasks, newStopTaskWrapper(task, options...))
}

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *Manager) StopTask(task Task, options ...TaskOption) {
	s.stopTasksOrdered = append(s.stopTasksOrdered, newStopTaskWrapper(task, options...))
}

// StopMultipleTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *Manager) StopMultipleTasks(f func(MultipleTaskBuilder)) {
	var multiTasks []taskWrapper
	mtb := &multipleTaskBuilder{
		stopFuture: func(task StopFuture) {
			multiTasks = append(multiTasks, s.taskFromStopFuture(task))
		},
		stop: func(task Task) {
			multiTasks = append(multiTasks, newStopTaskWrapper(task))
		},
	}
	f(mtb)
	if len(multiTasks) > 0 {
		s.StopTask(newMultipleTask(multiTasks...))
	}
}

// StopFuture adds a shutdown task. The shutdown will be done in the order they are added.
func (s *Manager) StopFuture(task StopFuture) {
	s.stopTasksOrdered = append(s.stopTasksOrdered, s.taskFromStopFuture(task))
}

// StopFutureMultiple adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *Manager) StopFutureMultiple(tasks ...StopFuture) {
	s.StopMultipleTasks(func(builder MultipleTaskBuilder) {
		for _, task := range tasks {
			builder.StopFuture(task)
		}
	})
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *Manager) AutoStopTask(task Task, options ...TaskOption) {
	s.stopTasks = append(s.stopTasks, newStopTaskWrapper(task, options...))
}

type StartTaskCmd struct {
	s        *Manager
	start    Task
	preStop  valuePtr[Task]
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStop(stop Task) {
	stopTask := s.createStopTask(s.s.unorderedCancelCtx, stop, WithCancelContext(true))
	s.s.AutoStopTask(stopTask)
}

// AutoStopContext schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStopContext() {
	s.addStartTask(s.s.unorderedCancelCtx)
}

func (s StartTaskCmd) PreStop(preStop Task) StartTaskCmd {
	s.preStop.Set(preStop)
	return s
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartTaskCmd) FutureStop(stop Task, stopOptions ...StopOption) StopFuture {
	return s.createStopFuture(stop, stopOptions...)
}

// FutureStopContext returns a StopFuture to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartTaskCmd) FutureStopContext() StopFuture {
	return s.createStopFuture(nil, WithCancelContext(true))
}

func (s StartTaskCmd) createStopFuture(stop Task, options ...StopOption) StopFuture {
	stopTask := s.createStopTask(s.s.ctx, stop, options...)
	return s.s.addPendingStopTask(stopTask, s.options...)
}

func (s StartTaskCmd) addStartTask(ctx context.Context) {
	s.resolved.setResolved()
	s.s.addTask(ctx, s.start, s.options...)
	if !s.preStop.IsNil() {
		s.s.PreStopTask(s.preStop.Get(), s.options...)
	}
}

func (s StartTaskCmd) createStopTask(ctx context.Context, stop Task, options ...StopOption) Task {
	var optns stopOptions
	for _, opt := range options {
		opt(&optns)
	}

	s.resolved.setResolved()

	var cancel context.CancelCauseFunc
	if optns.cancelContext {
		ctx, cancel = context.WithCancelCause(s.s.ctx)
	}
	s.addStartTask(ctx)
	var stopTask Task
	if stop == nil {
		// TODO: maybe a deadlock if !optns.cancelContext?
		stopTaskFunc := func(ctx context.Context) error {
			cancel(ErrExit)
			return nil
		}
		if tid, ok := s.start.(TaskWithID); ok {
			stopTask = TaskFuncWithID(tid.TaskID(), stopTaskFunc)
		} else {
			stopTask = TaskFunc(stopTaskFunc)
		}
	} else if !optns.cancelContext {
		stopTask = stop
	} else {
		stopTask = WrapTask(stop, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return stopTask
}

func (s StartTaskCmd) isResolved() bool {
	return s.resolved.isResolved()
}

type StartServiceCmd struct {
	s        *Manager
	svc      Service
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartServiceCmd) AutoStop() {
	startTask, preStopTask, stopTask := ServiceAsTasks(s.svc)
	s.addStartTask(s.s.unorderedCancelCtx, startTask, preStopTask)
	s.s.stopTasks = append(s.s.stopTasks, newStopTaskWrapper(stopTask, s.options...))
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartServiceCmd) FutureStop(options ...StopOption) StopFuture {
	var optns stopOptions
	for _, opt := range options {
		opt(&optns)
	}

	ctx := s.s.ctx
	var cancel context.CancelCauseFunc
	if optns.cancelContext {
		ctx, cancel = context.WithCancelCause(ctx)
	}
	startTask, preStopTask, stopTask := ServiceAsTasks(s.svc)
	s.addStartTask(ctx, startTask, preStopTask)
	var pendingStopTask Task
	if !optns.cancelContext {
		pendingStopTask = stopTask
	} else {
		pendingStopTask = WrapTask(stopTask, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return s.s.addPendingStopTask(pendingStopTask, s.options...)
}

// FutureStopContext calls FutureStop with WithCancelContext(true).
// It is a convenience to make it similar to the [Manager.StartTask] one.
func (s StartServiceCmd) FutureStopContext(options ...StopOption) StopFuture {
	return s.FutureStop(append(options, WithCancelContext(true))...)
}

func (s StartServiceCmd) addStartTask(ctx context.Context, startTask Task, preStopTask Task) {
	s.resolved.setResolved()
	s.s.addTask(ctx, startTask, s.options...)
	s.s.PreStopTask(preStopTask, s.options...)
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

// addTask adds a task to be started.
func (s *Manager) addTask(ctx context.Context, task Task, options ...TaskOption) {
	s.tasks = append(s.tasks, newTaskWrapper(ctx, task, options...))
}
