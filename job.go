package svcinit_poc1

import (
	"context"
)

func (s *SvcInit) ExecuteTask(fn Task) {
	s.addTask(s.cancelCtx, fn)
}

func (s *SvcInit) StartTask(start Task) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    start,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

func (s *SvcInit) StartService(svc Service) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

func (s *SvcInit) StopTask(fn StopTask) {
	if ps, ok := fn.(pendingStopTask); ok {
		ps.setResolved()
	}
	s.cleanup = append(s.cleanup, fn.Stop)
}

func (s *SvcInit) StopTaskFunc(fn Task) {
	s.cleanup = append(s.cleanup, fn)
}

func (s *SvcInit) AutoStopTask(fn Task) {
	s.autoCleanup = append(s.autoCleanup, fn)
}

type StartTaskCmd struct {
	s        *SvcInit
	start    Task
	resolved resolved
}

func (s StartTaskCmd) AutoStop() {
	s.resolved.setResolved()
	s.s.addTask(s.s.cancelCtx, s.start)
}

func (s StartTaskCmd) CtxStop() StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.start)
	return s.s.addPendingStopTask(func(_ context.Context) error {
		cancel(ErrExit)
		return nil
	})
}

func (s StartTaskCmd) Stop(stop Task) StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return s.s.addPendingStopTask(stop)
}

func (s StartTaskCmd) isResolved() bool {
	return s.resolved.isResolved()
}

type StartServiceCmd struct {
	s        *SvcInit
	svc      Service
	resolved resolved
}

func (s StartServiceCmd) AutoStop() {
	s.resolved.setResolved()
	s.s.addTask(s.s.cancelCtx, func(ctx context.Context) error {
		return s.svc.Start(ctx)
	})
	s.s.AutoStopTask(func(ctx context.Context) error {
		return s.svc.Stop(ctx)
	})
}

func (s StartServiceCmd) CtxStop() StopTask {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, func(ctx context.Context) error {
		return s.svc.Start(ctx)
	})
	return s.s.addPendingStopTask(func(_ context.Context) error {
		cancel(ErrExit)
		return s.svc.Stop(ctx)
	})
}

func (s StartServiceCmd) Stop() StopTask {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, func(ctx context.Context) error {
		return s.svc.Start(ctx)
	})
	return s.s.addPendingStopTask(func(ctx context.Context) error {
		return s.svc.Stop(ctx)
	})
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}

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
