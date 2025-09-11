package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled on stop.
// The task is only executed at the Run call.
func (s *SvcInit) ExecuteTask(fn Task) {
	_ = s.addTask(s.serviceCancelCtx, fn, false)
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
// A service is a task with Start and Stop methods.
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
	_ = s.s.addTask(s.s.serviceCancelCtx, s.start, false)
}

// StopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) StopCancel() StopTask {
	return s.stopCancel(nil)
}

// StopFuncCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) StopFuncCancel(stop Task) StopTask {
	return s.stopCancel(stop)
}

// Stop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartTaskCmd) Stop(stop Task) StopTask {
	s.resolved.setResolved()
	finishedCtx := s.s.addTask(s.s.ctx, s.start, true)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		err := stop(ctx)
		finishedWait(ctx, finishedCtx)
		return err
	})
}

func (s StartTaskCmd) stopCancel(stop Task) StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	finishedCtx := s.s.addTask(ctx, s.start, true)
	return s.s.addPendingStopTask(func(ctx context.Context) (err error) {
		cancel(ErrExit)
		if stop != nil {
			err = stop(ctx)
		}
		finishedWait(ctx, finishedCtx)
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
	_ = s.s.addTask(s.s.serviceCancelCtx, s.svc.Start, false)
	s.s.AutoStopTask(s.svc.Stop)
}

// StopCancel returns a StopTask to be stopped when the order matters.
// The context passed to the task will be canceled BEFORE calling the stop task.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) StopCancel() StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	finishedCtx := s.s.addTask(ctx, s.svc.Start, true)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		cancel(ErrExit)
		err := s.svc.Stop(ctx)
		finishedWait(ctx, finishedCtx)
		return err
	})
}

// Stop returns a StopTask to be stopped when the order matters.
// The context passed to the task will NOT be canceled.
// The returned StopTask must be added in order to [SvcInit.StopTask].
func (s StartServiceCmd) Stop() StopTask {
	s.resolved.setResolved()
	finishedCtx := s.s.addTask(s.s.ctx, s.svc.Start, true)
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		err := s.svc.Stop(ctx)
		finishedWait(ctx, finishedCtx)
		return err
	})
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

func (s *SvcInit) addTask(ctx context.Context, fn Task, checkFinished bool) (finishedCtx context.Context) {
	task := taskWrapper{
		ctx:  ctx,
		task: fn,
	}
	if checkFinished {
		task.taskFinishedCtx, task.taskFinished = context.WithCancel(s.ctx)
	}
	s.tasks = append(s.tasks, task)
	return task.taskFinishedCtx
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

func finishedWait(ctx, finishedCtx context.Context) {
	select {
	case <-finishedCtx.Done():
	case <-ctx.Done():
	}
}
