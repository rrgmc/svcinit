package svcinit

import "context"

type StartTaskCmd struct {
	s        *Manager
	start    Task
	preStop  valuePtr[Task]
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStop(stop Task) {
	stopTask := s.createStopTask(s.s.unorderedCancelCtx, stop, false, WithCancelContext(true))
	s.s.AutoStopTask(stopTask)
}

// AutoStopContext schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartTaskCmd) AutoStopContext() {
	s.addStartTask(s.s.unorderedCancelCtx)
}

// PreStop adds a pre-stop task.
func (s StartTaskCmd) PreStop(preStop Task) StartTaskCmd {
	s.preStop.Set(preStop)
	return s
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// If stop is nil, it will be replaced by NullTask.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartTaskCmd) FutureStop(stop Task, stopOptions ...StopOption) StopFuture {
	return s.createStopFuture(stop, false, stopOptions...)
}

// FutureStopContext returns a StopFuture to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartTaskCmd) FutureStopContext() StopFuture {
	return s.createStopFuture(nil, true, WithCancelContext(true))
}

func (s StartTaskCmd) createStopFuture(stop Task, isAutoStop bool, options ...StopOption) StopFuture {
	stopTask := s.createStopTask(s.s.ctx, stop, isAutoStop, options...)
	return s.s.addPendingStopTask(stopTask, s.options...)
}

func (s StartTaskCmd) addStartTask(ctx context.Context) {
	s.resolved.setResolved()
	s.s.addTask(ctx, s.start, s.options...)
	if !s.preStop.IsNil() {
		s.s.PreStopTask(s.preStop.Get(), s.options...)
	}
}

func (s StartTaskCmd) createStopTask(ctx context.Context, stop Task, isAutoStop bool, options ...StopOption) Task {
	var optns stopOptions
	for _, opt := range options {
		opt(&optns)
	}

	s.resolved.setResolved()

	var cancel context.CancelCauseFunc
	if optns.cancelContext {
		ctx, cancel = context.WithCancelCause(s.s.ctx)
	}
	s.addStartTask(ctx)
	var stopTask Task
	if stop == nil {
		if !isAutoStop {
			return stop
		}
		// TODO: maybe a deadlock if !optns.cancelContext?
		stopTaskFunc := func(ctx context.Context) error {
			if optns.cancelContext {
				cancel(ErrExit)
			}
			return nil
		}
		if tid, ok := s.start.(TaskWithID); ok {
			stopTask = TaskFuncWithID(tid.TaskID(), stopTaskFunc)
		} else {
			stopTask = TaskFunc(stopTaskFunc)
		}
	} else if !optns.cancelContext {
		stopTask = stop
	} else {
		stopTask = WrapTask(stop, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return stopTask
}

func (s StartTaskCmd) isResolved() bool {
	return s.resolved.isResolved()
}
