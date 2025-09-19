package svcinit

import (
	"context"
	"slices"
)

func (s *Manager) taskFromStartFuture(task StartFuture) taskWrapper {
	if task == nil {
		return s.newTaskWrapper(nil, nil)
	}
	var optns []TaskOption
	if ps, ok := task.(*pendingStartFuture); ok {
		ps.setResolved()
		optns = slices.Clone(ps.options)
	}
	return s.newTaskWrapper(nil, task.startTask(), optns...)
}

func (s *Manager) taskFromStopFuture(task StopFuture) taskWrapper {
	if task == nil {
		return s.newStopTaskWrapper(nil)
	}
	var optns []TaskOption
	if ps, ok := task.(*pendingStopFuture); ok {
		ps.setResolved()
		optns = slices.Clone(ps.options)
	}
	return s.newStopTaskWrapper(task.stopTask(), optns...)
}

type pendingItem interface {
	isResolved() bool
}

type pendingStartFuture struct {
	ctx      context.Context
	task     Task
	options  []TaskOption
	resolved resolved
}

var _ StartFuture = (*pendingStartFuture)(nil)

func newPendingStartFuture(ctx context.Context, startTask Task, options ...TaskOption) *pendingStartFuture {
	return &pendingStartFuture{
		ctx:      ctx,
		task:     startTask,
		options:  options,
		resolved: newResolved(),
	}
}

func (p *pendingStartFuture) startTask() Task {
	return p.task
}

func (p *pendingStartFuture) isResolved() bool {
	return p.resolved.isResolved()
}

func (p *pendingStartFuture) setResolved() {
	p.resolved.setResolved()
}

type pendingStopFuture struct {
	task     Task
	options  []TaskOption
	resolved resolved
}

var _ StopFuture = (*pendingStopFuture)(nil)

func newPendingStopFuture(stopTask Task, options ...TaskOption) *pendingStopFuture {
	return &pendingStopFuture{
		task:     stopTask,
		options:  options,
		resolved: newResolved(),
	}
}

func (p *pendingStopFuture) stopTask() Task {
	return p.task
}

func (p *pendingStopFuture) isResolved() bool {
	return p.resolved.isResolved()
}

func (p *pendingStopFuture) setResolved() {
	p.resolved.setResolved()
}

func serviceAsTasks(svc Service) (start, preStop, stop Task) {
	if svc == nil {
		return nil, nil, nil
	}
	return ServiceAsTask(svc, StageStart),
		ServiceAsTask(svc, StagePreStop),
		ServiceAsTask(svc, StageStop)
}
