package svcinit

import "context"

type StartTask interface {
	AutoStop(stop Task)
	AutoStopContext()
	PreStop(preStop Task) StartTask
	FutureStop(stop Task, stopOptions ...StopOption) StopFuture
	FutureStopContext() StopFuture
}

type StartFutureTask interface {
	AutoStop(stop Task) StartFuture
	AutoStopContext() StartFuture
	PreStop(preStop Task) StartFutureTask
	FutureStop(stop Task, stopOptions ...StopOption) (StartFuture, StopFuture)
	FutureStopContext() (StartFuture, StopFuture)
}

type StartTaskCmd struct {
	handler *startTaskHandler
}

func (s *Manager) newStartTaskCmd(task Task, options ...TaskOption) *StartTaskCmd {
	return &StartTaskCmd{
		handler: s.newStartTaskHandler(task, false, options...),
	}
}

func (s *StartTaskCmd) AutoStop(stop Task) {
	_ = s.handler.AutoStop(stop)
}

func (s *StartTaskCmd) AutoStopContext() {
	_ = s.handler.AutoStopContext()
}

func (s *StartTaskCmd) PreStop(preStop Task) StartTask {
	s.handler.PreStop(preStop)
	return s
}

func (s *StartTaskCmd) FutureStop(stop Task, stopOptions ...StopOption) StopFuture {
	_, stopFuture := s.handler.FutureStop(stop, stopOptions...)
	return stopFuture
}

func (s *StartTaskCmd) FutureStopContext() StopFuture {
	_, stopFuture := s.handler.FutureStopContext()
	return stopFuture
}

func (s *StartTaskCmd) isResolved() bool {
	return s.handler.isResolved()
}

type StartFutureTaskCmd struct {
	handler *startTaskHandler
}

func (s *Manager) newStartFutureTaskCmd(task Task, options ...TaskOption) *StartFutureTaskCmd {
	return &StartFutureTaskCmd{
		handler: s.newStartTaskHandler(task, true, options...),
	}
}

func (s *StartFutureTaskCmd) AutoStop(stop Task) StartFuture {
	return s.handler.AutoStop(stop)
}

func (s *StartFutureTaskCmd) AutoStopContext() StartFuture {
	return s.handler.AutoStopContext()
}

func (s *StartFutureTaskCmd) PreStop(preStop Task) StartFutureTask {
	s.handler.PreStop(preStop)
	return s
}

func (s *StartFutureTaskCmd) FutureStop(stop Task, stopOptions ...StopOption) (StartFuture, StopFuture) {
	return s.handler.FutureStop(stop, stopOptions...)
}

func (s *StartFutureTaskCmd) FutureStopContext() (StartFuture, StopFuture) {
	return s.handler.FutureStopContext()
}

func (s *StartFutureTaskCmd) isResolved() bool {
	return s.handler.isResolved()
}

type startTaskHandler struct {
	s        *Manager
	isFuture bool
	start    Task
	preStop  valuePtr[Task]
	options  []TaskOption
	resolved resolved
}

func (s *Manager) newStartTaskHandler(task Task, isFuture bool, options ...TaskOption) *startTaskHandler {
	return &startTaskHandler{
		s:        s,
		isFuture: isFuture,
		start:    task,
		options:  options,
		preStop:  newValuePtr[Task](),
		resolved: newResolved(),
	}
}

// AutoStop schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s startTaskHandler) AutoStop(stop Task) StartFuture {
	startFuture, stopTask := s.createStopTask(s.s.unorderedCancelCtx, stop, false, WithCancelContext(true))
	s.s.AutoStopTask(stopTask)
	return startFuture
}

// AutoStopContext schedules the task to be stopped when the shutdown order DOES NOT matter.
// The context passed to the task will be canceled.
func (s startTaskHandler) AutoStopContext() StartFuture {
	return s.addStartTask(s.s.unorderedCancelCtx)
}

// PreStop adds a pre-stop task.
func (s startTaskHandler) PreStop(preStop Task) {
	s.preStop.Set(preStop)
}

// FutureStop returns a StopFuture to be stopped when the order matters.
// If stop is nil, it will be replaced by NullTask.
// The context passed to the task will NOT be canceled, except if the option WithCancelContext(true) is set.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s startTaskHandler) FutureStop(stop Task, stopOptions ...StopOption) (StartFuture, StopFuture) {
	return s.createStopFuture(stop, false, stopOptions...)
}

// FutureStopContext returns a StopFuture to be stopped when the order matters.
// The context passed to the task will be canceled.
// The returned StopFuture must be added in order to [Manager.StopTask].
func (s startTaskHandler) FutureStopContext() (StartFuture, StopFuture) {
	return s.createStopFuture(nil, true, WithCancelContext(true))
}

func (s startTaskHandler) createStopFuture(stop Task, isAutoStop bool, options ...StopOption) (StartFuture, StopFuture) {
	startFuture, stopTask := s.createStopTask(s.s.ctx, stop, isAutoStop, options...)
	return startFuture, s.s.addPendingStopTask(stopTask, s.options...)
}

func (s startTaskHandler) addStartTask(ctx context.Context) (sf StartFuture) {
	if !s.isFuture {
		s.resolved.setResolved()
		s.s.addTask(ctx, s.start, s.options...)
	} else {
		sf = s.s.addPendingStartTask(ctx, s.start, s.options...)
	}
	if !s.preStop.IsNil() {
		s.s.PreStopTask(s.preStop.Get(), s.options...)
	}
	return
}

func (s startTaskHandler) createStopTask(ctx context.Context, stop Task, isAutoStop bool, options ...StopOption) (StartFuture, Task) {
	var optns stopOptions
	for _, opt := range options {
		opt(&optns)
	}

	s.resolved.setResolved()

	var cancel context.CancelCauseFunc
	if optns.cancelContext {
		ctx, cancel = context.WithCancelCause(s.s.ctx)
	}
	startFuture := s.addStartTask(ctx)
	var stopTask Task
	if stop == nil {
		if !isAutoStop {
			return startFuture, stop
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
	return startFuture, stopTask
}

func (s startTaskHandler) isResolved() bool {
	return s.resolved.isResolved()
}
