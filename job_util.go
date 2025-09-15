package svcinit

import "slices"

func (s *SvcInit) taskFromStopTask(task StopFuture) taskWrapper {
	var optns []TaskOption
	if ps, ok := task.(*pendingStopTask); ok {
		ps.setResolved()
		optns = slices.Clone(ps.options)
	}
	return newStopTaskWrapper(task.stopTask(), optns...)
}

type pendingItem interface {
	isResolved() bool
}

type pendingStopTask struct {
	task     Task
	options  []TaskOption
	resolved resolved
}

var _ StopFuture = (*pendingStopTask)(nil)

func newPendingStopTask(stopTask Task, options ...TaskOption) *pendingStopTask {
	return &pendingStopTask{
		task:     stopTask,
		options:  options,
		resolved: newResolved(),
	}
}

func (p *pendingStopTask) stopTask() Task {
	return p.task
}

func (p *pendingStopTask) isResolved() bool {
	return p.resolved.isResolved()
}

func (p *pendingStopTask) setResolved() {
	p.resolved.setResolved()
}
