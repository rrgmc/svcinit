package svcinit

import "context"

type StartServiceCmd struct {
	s        *Manager
	svc      Service
	options  []TaskOption
	resolved resolved
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s StartServiceCmd) AutoStop() {
	startTask, preStopTask, stopTask := serviceAsTasks(s.svc)
	s.addStartTask(s.s.unorderedCancelCtx, startTask, preStopTask)
	s.s.stopTasks = append(s.s.stopTasks, s.s.newStopTaskWrapper(stopTask, s.options...))
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s StartServiceCmd) FutureStop(options ...StopOption) StopFuture {
	var optns stopOptions
	for _, opt := range options {
		opt(&optns)
	}

	ctx := s.s.ctx
	var cancel context.CancelCauseFunc
	if optns.cancelContext {
		ctx, cancel = context.WithCancelCause(ctx)
	}
	startTask, preStopTask, stopTask := serviceAsTasks(s.svc)
	s.addStartTask(ctx, startTask, preStopTask)
	var pendingStopTask Task
	if stopTask == nil {
		return nil
	} else if !optns.cancelContext {
		pendingStopTask = stopTask
	} else {
		pendingStopTask = WrapTask(stopTask, WithWrapTaskHandler(func(ctx context.Context, task Task) error {
			cancel(ErrExit)
			return task.Run(ctx)
		}))
	}
	return s.s.addPendingStopTask(pendingStopTask, s.options...)
}

// FutureStopContext calls FutureStop with WithCancelContext(true).
// It is a convenience to make it similar to the [Manager.StartTask] one.
func (s StartServiceCmd) FutureStopContext(options ...StopOption) StopFuture {
	return s.FutureStop(append(options, WithCancelContext(true))...)
}

func (s StartServiceCmd) addStartTask(ctx context.Context, startTask Task, preStopTask Task) {
	s.resolved.setResolved()
	s.s.addTask(ctx, startTask, s.options...)
	s.s.PreStopTask(preStopTask, s.options...)
}

func (s StartServiceCmd) isResolved() bool {
	return s.resolved.isResolved()
}
