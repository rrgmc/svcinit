package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTask(task Task, options ...TaskOption) {
	s.addTask(s.unorderedCancelCtx, task, options...)
}

// ExecuteTaskFunc executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTaskFunc(task TaskFunc, options ...TaskOption) {
	s.ExecuteTask(task, options...)
}

// StartTask executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartTask(task Task, options ...TaskOption) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    task,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartTaskFunc executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartTaskFunc(task TaskFunc, options ...TaskOption) StartTaskCmd {
	return s.StartTask(task, options...)
}

// StartService executes a service task and allows the shutdown method to be customized.
// A service is a task with Start and ManualStop methods.
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

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTask(task Task) {
	s.cleanup = append(s.cleanup, newStopTaskWrapper(task))
}

// StopTaskFunc adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTaskFunc(task TaskFunc) {
	s.StopTask(task)
}

// StopManualTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopManualTask(task StopTask) {
	s.cleanup = append(s.cleanup, s.taskFromStopTask(task))
}

// StopMultipleManualTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopMultipleManualTasks(tasks ...StopTask) {
	s.StopMultipleTasks(func(builder MultipleTaskBuilder) {
		for _, task := range tasks {
			builder.StopManualTask(task)
		}
	})
}

// StopMultipleTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopMultipleTasks(f func(MultipleTaskBuilder)) {
	var multiTasks []taskWrapper
	mtb := &multipleTaskBuilder{
		stopManualTask: func(task StopTask) {
			multiTasks = append(multiTasks, s.taskFromStopTask(task))
		},
		stopTask: func(task Task) {
			multiTasks = append(multiTasks, newStopTaskWrapper(task))
		},
	}
	f(mtb)
	if len(multiTasks) > 0 {
		s.StopTask(newMultipleTask(multiTasks...))
	}
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTask(task Task) {
	s.autoCleanup = append(s.autoCleanup, newStopTaskWrapper(task))
}

// AutoStopTaskFunc adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTaskFunc(task TaskFunc) {
	s.AutoStopTask(task)
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

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancel() StopTask {
	return s.stopCancel(nil)
}

// ManualStopCancelTask returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancelTask(stop Task, options ...TaskOption) StopTask {
	return s.stopCancel(stop, options...)
}

// ManualStopCancelTaskFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancelTaskFunc(stop TaskFunc, options ...TaskOption) StopTask {
	return s.ManualStopCancelTask(stop, options...)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStop(stop Task, options ...TaskOption) StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start, s.options...)
	return s.s.addPendingStopTask(stop, options...)
}

// ManualStopFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopFunc(stop TaskFunc, options ...TaskOption) StopTask {
	return s.ManualStop(stop, options...)
}

func (s StartTaskCmd) stopCancel(stop Task, options ...TaskOption) StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.start, s.options...)
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) (err error) {
		cancel(ErrExit)
		if stop != nil {
			err = stop.Run(ctx)
		}
		return
	}), options...)
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

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStopCancel() StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(ctx, startTask, s.options...)
	return s.s.addPendingStopTask(WrapTask(stopTask, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
		cancel(ErrExit)
		return task.Run(ctx)
	})), s.options...)
	// return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) error {
	// 	cancel(ErrExit)
	// 	return stopTask.Run(ctx)
	// }), s.options...)
	// return s.s.addPendingStopTask(stopTask, slices.Concat(s.options, []TaskOption{
	// 	WithTaskCallback(TaskCallbackFunc(func(ctx context.Context, task Task) {
	// 		cancel(ErrExit)
	// 	}, nil)),
	// })...)
	// return s.s.addPendingStopTask(&serviceTaskWithCallback{
	// 	svc: stopTask,
	// 	callback: TaskCallbackFunc(func(ctx context.Context, task Task) {
	// 		cancel(ErrExit)
	// 	}, nil),
	// }, s.options...)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStop() StopTask {
	s.resolved.setResolved()
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(s.s.ctx, startTask, s.options...)
	return s.s.addPendingStopTask(stopTask, s.options...)
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

// addTask adds a task to be started.
func (s *SvcInit) addTask(ctx context.Context, task Task, options ...TaskOption) {
	s.tasks = append(s.tasks, newTaskWrapper(ctx, task, options...))
}
