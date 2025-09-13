package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTask(task Task, options ...TaskOption) {
	s.addTask(s.unorderedCancelCtx, parseTaskOptions(task, options...))
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
		start:    parseTaskOptions(task, options...),
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
		svc:      parseServiceTaskOptions(svc, options...),
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTask(task Task) {
	if ps, ok := task.(pendingStopTask); ok {
		ps.setResolved()
	}
	s.cleanup = append(s.cleanup, task)
}

// StopTaskFunc adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTaskFunc(task TaskFunc) {
	s.StopTask(task)
}

// StopMultipleTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopMultipleTasks(tasks ...Task) {
	s.StopTask(NewMultipleTask(tasks...))
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTask(task Task) {
	s.autoCleanup = append(s.autoCleanup, task)
}

// AutoStopTaskFunc adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTaskFunc(task TaskFunc) {
	s.AutoStopTask(task)
}

type StartTaskCmd struct {
	s        *SvcInit
	start    Task
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStop() {
	s.resolved.setResolved()
	s.s.addTask(s.s.unorderedCancelCtx, s.start)
}

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancel() Task {
	return s.stopCancel(nil)
}

// ManualStopCancelTask returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancelTask(stop Task, options ...TaskOption) Task {
	return s.stopCancel(stop, options...)
}

// ManualStopCancelTaskFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancelTaskFunc(stop TaskFunc, options ...TaskOption) Task {
	return s.ManualStopCancelTask(stop, options...)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStop(stop Task, options ...TaskOption) Task {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return s.s.addPendingStopTask(parseTaskOptions(stop))
}

// ManualStopFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopFunc(stop TaskFunc, options ...TaskOption) Task {
	return s.ManualStop(stop, options...)
}

func (s StartTaskCmd) stopCancel(stop Task, options ...TaskOption) Task {
	if stop != nil {
		stop = parseTaskOptions(stop, options...)
	}
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.start)
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) (err error) {
		cancel(ErrExit)
		if stop != nil {
			err = stop.Run(ctx)
		}
		return
	}))
}

func (s StartTaskCmd) isResolved() bool {
	return s.resolved.isResolved()
}

type StartServiceCmd struct {
	s        *SvcInit
	svc      Service
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartServiceCmd) AutoStop() {
	s.resolved.setResolved()
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(s.s.unorderedCancelCtx, startTask)
	s.s.AutoStopTask(stopTask)
}

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStopCancel() Task {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(ctx, startTask)
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) error {
		cancel(ErrExit)
		return stopTask.Run(ctx)
	}))
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStop() Task {
	s.resolved.setResolved()
	startTask, stopTask := ServiceAsTasks(s.svc)
	s.s.addTask(s.s.ctx, startTask)
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) error {
		return stopTask.Run(ctx)
	}))
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

// addTask adds a task to be started.
// If checkFinished is true, a context will be returned that will be done when the task finishes executing.
// This is used to make the stop task wait the start task finish.
func (s *SvcInit) addTask(ctx context.Context, fn Task) {
	s.tasks = append(s.tasks, taskWrapper{
		ctx:  ctx,
		task: fn,
	})
}

type pendingItem interface {
	isResolved() bool
}

func newPendingStopTask(task Task) pendingStopTask {
	return newPendingStopTaskImpl(task)
}

type pendingStopTask interface {
	Task
	pendingItem
	setResolved()
}

type pendingStopTaskImpl struct {
	stopTask Task
	resolved resolved
}

var _ WrappedTask = (*pendingStopTaskImpl)(nil)

func newPendingStopTaskImpl(stopTask Task) pendingStopTaskImpl {
	return pendingStopTaskImpl{
		stopTask: stopTask,
		resolved: newResolved(),
	}
}

func (p pendingStopTaskImpl) WrappedTasks() []Task {
	return []Task{p.stopTask}
}

func (p pendingStopTaskImpl) Stop(ctx context.Context) error {
	return p.stopTask.Run(ctx)
}

func (p pendingStopTaskImpl) Run(ctx context.Context) error {
	return p.Stop(ctx)
}

func (p pendingStopTaskImpl) isResolved() bool {
	return p.resolved.isResolved()
}

func (p pendingStopTaskImpl) setResolved() {
	p.resolved.setResolved()
}
