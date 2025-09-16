package svcinit

import (
	"context"
)

// Execute executes the passed task which don't need a stop task.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) Execute(task Task, options ...TaskOption) {
	s.addTask(s.unorderedCancelCtx, task, options...)
}

// Start executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) Start(task Task, options ...TaskOption) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    task,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartService executes a service task and allows the shutdown method to be customized.
// A service is a task with Start and Stop methods.
// At least one method of StartServiceCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartService(svc Service, options ...TaskOption) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// Stop adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) Stop(task Task, options ...TaskOption) {
	s.cleanup = append(s.cleanup, newStopTaskWrapper(task, options...))
}

// StopMultiple adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopMultiple(f func(MultipleTaskBuilder)) {
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
		s.Stop(newMultipleTask(multiTasks...))
	}
}

// StopFuture adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopFuture(task StopFuture) {
	s.cleanup = append(s.cleanup, s.taskFromStopFuture(task))
}

// StopFutureMultiple adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopFutureMultiple(tasks ...StopFuture) {
	s.StopMultiple(func(builder MultipleTaskBuilder) {
		for _, task := range tasks {
			builder.StopFuture(task)
		}
	})
}

// AutoStop adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStop(task Task, options ...TaskOption) {
	s.autoCleanup = append(s.autoCleanup, newStopTaskWrapper(task, options...))
}

type StartTaskCmd struct {
	s        *SvcInit
	start    Task
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStop() {
	s.resolved.setResolved()
	s.s.addTask(s.s.unorderedCancelCtx, s.start, s.options...)
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) FutureStop(stop Task, stopOptions ...FutureStopOption) StopFuture {
	return s.doStop(stop, stopOptions...)
}

// FutureStopContext returns a StopFuture to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopFuture must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) FutureStopContext(stopOptions ...TaskOption) StopFuture {
	return s.doStop(nil, castFutureStopOptions(stopOptions)...)
}

func (s StartTaskCmd) doStop(stop Task, stopOptions ...FutureStopOption) StopFuture {
	optns := parseFutureStopOptions(stopOptions...)

	s.resolved.setResolved()
	ctx := s.s.ctx
	var cancel context.CancelCauseFunc
	if optns.futureStopOptions.cancelContext {
		ctx, cancel = context.WithCancelCause(s.s.ctx)
	}
	s.s.addTask(ctx, s.start, s.options...)
	var pendingStopTask Task
	if !optns.futureStopOptions.cancelContext {
		pendingStopTask = stop
	} else if stop == nil {
		pendingStopTask = TaskFunc(func(ctx context.Context) error {
			cancel(ErrExit)
			return nil
		})
	} else {
		pendingStopTask = WrapTask(stop, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return s.s.addPendingStopTask(pendingStopTask, optns.options...)
}

func (s StartTaskCmd) isResolved() bool {
	return s.resolved.isResolved()
}

type StartServiceCmd struct {
	s        *SvcInit
	svc      Service
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartServiceCmd) AutoStop() {
	s.resolved.setResolved()
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(s.s.unorderedCancelCtx, startTask, s.options...)
	s.s.autoCleanup = append(s.s.autoCleanup, newStopTaskWrapper(stopTask, s.options...))
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [SvcInit.Stop].
func (s StartServiceCmd) FutureStop(stopOptions ...FutureStopOption) StopFuture {
	optns := parseFutureStopOptions(stopOptions...)
	pendingStopOptions := s.options // use same options as start
	if optns.options != nil {
		// use custom stop options
		pendingStopOptions = optns.options
	}

	s.resolved.setResolved()
	ctx := s.s.ctx
	var cancel context.CancelCauseFunc
	if optns.futureStopOptions.cancelContext {
		ctx, cancel = context.WithCancelCause(s.s.ctx)
	}
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(ctx, startTask, s.options...)
	var pendingStopTask Task
	if !optns.futureStopOptions.cancelContext {
		pendingStopTask = stopTask
	} else {
		pendingStopTask = WrapTask(stopTask, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return s.s.addPendingStopTask(pendingStopTask, pendingStopOptions...)
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

// addTask adds a task to be started.
func (s *SvcInit) addTask(ctx context.Context, task Task, options ...TaskOption) {
	s.tasks = append(s.tasks, newTaskWrapper(ctx, task, options...))
}
