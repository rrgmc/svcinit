package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTask(fn Task) {
	s.addTask(s.unorderedCancelCtx, fn)
}

// ExecuteTaskFunc executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTaskFunc(fn TaskFunc) {
	s.ExecuteTask(fn)
}

// StartTask executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartTask(start Task) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    start,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartTaskFunc executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartTaskFunc(start TaskFunc) StartTaskCmd {
	return s.StartTask(start)
}

// StartService executes a service task and allows the shutdown method to be customized.
// A service is a task with Start and ManualStop methods.
// At least one method of StartServiceCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *SvcInit) StartService(svc Service) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// Stop adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) Stop(fn StopTask) {
	if ps, ok := fn.(pendingStopTask); ok {
		ps.setResolved()
	}
	s.cleanup = append(s.cleanup, fn)
}

// StopParallel adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *SvcInit) StopParallel(fn ...StopTask) {
	s.Stop(NewParallelStopTask(fn...))
}

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTask(fn Task) {
	s.cleanup = append(s.cleanup, fn)
}

// StopTaskFunc adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTaskFunc(fn TaskFunc) {
	s.StopTask(fn)
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTask(fn Task) {
	s.autoCleanup = append(s.autoCleanup, fn)
}

// AutoStopTaskFunc adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTaskFunc(fn TaskFunc) {
	s.AutoStopTask(fn)
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
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) ManualStopCancel() StopTask {
	return s.stopCancel(nil)
}

// ManualStopCancelTask returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) ManualStopCancelTask(stop Task) StopTask {
	return s.stopCancel(stop)
}

// ManualStopCancelTaskFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) ManualStopCancelTaskFunc(stop TaskFunc) StopTask {
	return s.ManualStopCancelTask(stop)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) ManualStop(stop Task) StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return s.s.addPendingStopTask(stop)
}

// ManualStopFunc returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartTaskCmd) ManualStopFunc(stop TaskFunc) StopTask {
	return s.ManualStop(stop)
}

func (s StartTaskCmd) stopCancel(stop Task) StopTask {
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
	s.s.addTask(s.s.unorderedCancelCtx, ServiceAsTask(s.svc, true))
	s.s.AutoStopTask(ServiceAsTask(s.svc, false))
}

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartServiceCmd) ManualStopCancel() StopTask {
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
// The returned StopTask must be added in order to [SvcInit.Stop].
func (s StartServiceCmd) ManualStop() StopTask {
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
func (s *SvcInit) addTask(ctx context.Context, fn Task) {
	s.tasks = append(s.tasks, taskWrapper{
		ctx:  ctx,
		task: fn,
	})
}

type pendingTask interface {
	isResolved() bool
}

func newPendingStopTask(task Task) pendingStopTask {
	return newPendingStopTaskImpl(task)
}

type pendingStopTask interface {
	StopTask
	isResolved() bool
	setResolved()
}

type pendingStopTaskImpl struct {
	stopTask Task
	resolved resolved
}

func newPendingStopTaskImpl(stopTask Task) pendingStopTaskImpl {
	return pendingStopTaskImpl{
		stopTask: stopTask,
		resolved: newResolved(),
	}
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
