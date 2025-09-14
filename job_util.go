package svcinit

func (s *SvcInit) taskFromStopTask(task StopTask) Task {
	if ps, ok := task.(*pendingStopTask); ok {
		ps.setResolved()
	}
	return task.stopTask()
}

type pendingItem interface {
	isResolved() bool
}

type pendingStopTask struct {
	task     Task
	resolved resolved
}

var _ StopTask = (*pendingStopTask)(nil)

func newPendingStopTask(stopTask Task) *pendingStopTask {
	return &pendingStopTask{
		task:     stopTask,
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
