package svcinit

import "slices"

func (s *SvcInit) taskFromStopFuture(task StopFuture) taskWrapper {
	var optns []TaskOption
	if ps, ok := task.(*pendingStopFuture); ok {
		ps.setResolved()
		optns = slices.Clone(ps.options)
	}
	return newStopTaskWrapper(task.stopTask(), optns...)
}

type pendingItem interface {
	isResolved() bool
}

type pendingStopFuture struct {
	task     Task
	options  []TaskOption
	resolved resolved
}

var _ StopFuture = (*pendingStopFuture)(nil)

func newPendingStopTask(stopTask Task, options ...TaskOption) *pendingStopFuture {
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
