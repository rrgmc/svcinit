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

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTask(fn StopTask) {
	if ps, ok := fn.(pendingStopTask); ok {
		ps.setResolved()
	}
	s.cleanup = append(s.cleanup, fn.Stop)
}

// StopTaskFunc adds a shutdown task. The shutdown will be done in the order they are added.
func (s *SvcInit) StopTaskFunc(fn Task) {
	s.cleanup = append(s.cleanup, fn)
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *SvcInit) AutoStopTask(fn Task) {
	s.autoCleanup = append(s.autoCleanup, fn)
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
func (s StartTaskCmd) ManualStopCancel() StopTask {
	return s.stopCancel(nil)
}

// ManualStopFuncCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStopFuncCancel(stop Task) StopTask {
	return s.stopCancel(stop)
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) ManualStop(stop Task) StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		return stop(ctx)
	})
}

func (s StartTaskCmd) stopCancel(stop Task) StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.start)
	return s.s.addPendingStopTask(func(ctx context.Context) (err error) {
		cancel(ErrExit)
		if stop != nil {
			err = stop(ctx)
		}
		return
	})
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
	s.s.addTask(s.s.unorderedCancelCtx, s.svc.Start)
	s.s.AutoStopTask(s.svc.Stop)
}

// ManualStopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStopCancel() StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.svc.Start)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		cancel(ErrExit)
		return s.svc.Stop(ctx)
	})
}

// ManualStop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) ManualStop() StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.svc.Start)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		return s.svc.Stop(ctx)
	})
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
	return pendingStopTaskImpl{
		stopTask: task,
		resolved: newResolved(),
	}
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

func (p pendingStopTaskImpl) Stop(ctx context.Context) error {
	return p.stopTask(ctx)
}

func (p pendingStopTaskImpl) isResolved() bool {
	return p.resolved.isResolved()
}

func (p pendingStopTaskImpl) setResolved() {
	p.resolved.setResolved()
}
