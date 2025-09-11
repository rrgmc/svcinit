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
	s.addPending(cmd)
	return cmd
}

func (s *SvcInit) StartService(svc Service) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		resolved: newResolved(),
	}
	s.addPending(cmd)
	return cmd
}

func (s *SvcInit) StopTask(fn Task) {
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

func (s StartTaskCmd) CtxStop() (stopFn Task) {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, s.start)
	return func(_ context.Context) error {
		cancel(ErrExit)
		return nil
	}
}

func (s StartTaskCmd) Stop(stop Task) (stopFn Task) {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, s.start)
	return stop
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

func (s StartServiceCmd) CtxStop() (stopFn Task) {
	s.resolved.setResolved()
	ctx, cancel := context.WithCancelCause(s.s.ctx)
	s.s.addTask(ctx, func(ctx context.Context) error {
		return s.svc.Start(ctx)
	})
	return func(_ context.Context) error {
		cancel(ErrExit)
		return s.svc.Stop(ctx)
	}
}

func (s StartServiceCmd) Stop() (stopFn Task) {
	s.resolved.setResolved()
	s.s.addTask(s.s.ctx, func(ctx context.Context) error {
		return s.svc.Start(ctx)
	})
	return func(ctx context.Context) error {
		return s.svc.Stop(ctx)
	}
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

type pendingStart interface {
	isResolved() bool
}
