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
func (s *SvcInit) StartTask(start Task, options ...TaskOption) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    start,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartTaskFunc executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartTaskFunc(start TaskFunc, options ...TaskOption) StartTaskCmd {
	return s.StartTask(start, options...)
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
func (s *SvcInit) StopTask(task Task, options ...TaskOption) {
	if ps, ok := task.(pendingStopTask); ok {
		ps.setResolved()
	}
	s.cleanup = append(s.cleanup, newTaskWrapper(task, withTaskWrapperTaskOptions(options...)))
}

// StopTaskFunc adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTaskFunc(fn TaskFunc, options ...TaskOption) {
	s.StopTask(fn, options...)
}

// StopMultipleTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopMultipleTasks(fn ...Task) {
	s.StopTask(NewMultipleTask(fn...))
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTask(fn Task) {
	s.autoCleanup = append(s.autoCleanup, newTaskWrapper(fn))
}

// AutoStopTaskFunc adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTaskFunc(fn TaskFunc) {
	s.AutoStopTask(fn)
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
func (s StartTaskCmd) ManualStopCancelTask(stop Task) Task {
	return s.stopCancel(stop)
}

// ManualStopCancelTaskFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopCancelTaskFunc(stop TaskFunc) Task {
	return s.ManualStopCancelTask(stop)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStop(stop Task) Task {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return s.s.addPendingStopTask(stop)
}

// ManualStopFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopFunc(stop TaskFunc) Task {
	return s.ManualStop(stop)
}

func (s StartTaskCmd) stopCancel(stop Task) Task {
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
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartServiceCmd) AutoStop() {
	s.resolved.setResolved()
	s.s.addTask(s.s.unorderedCancelCtx, ServiceAsTask(s.svc, true))
	s.s.AutoStopTask(ServiceAsTask(s.svc, false))
}

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStopCancel() Task {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, ServiceAsTask(s.svc, true))
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) error {
		cancel(ErrExit)
		return s.svc.Stop(ctx)
	}))
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStop() Task {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, ServiceAsTask(s.svc, true))
	return s.s.addPendingStopTask(TaskFunc(func(ctx context.Context) error {
		return s.svc.Stop(ctx)
	}))
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

// addTask adds a task to be started.
// If checkFinished is true, a context will be returned that will be done when the task finishes executing.
// This is used to make the stop task wait the start task finish.
func (s *SvcInit) addTask(ctx context.Context, task Task, options ...TaskOption) {
	s.tasks = append(s.tasks, newTaskWrapper(task,
		withTaskWrapperContext(ctx),
		withTaskWrapperTaskOptions(options...)))
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
