package k8sinit

import (
	"context"
	"fmt"

	"github.com/rrgmc/svcinit/v3"
)

type TelemetryHandler interface {
	FlushTelemetry(ctx context.Context) error
}

type TelemetryHandlerTask interface {
	TelemetryHandler
	svcinit.Task
}

// SetTelemetryHandler sets a telemetry handler. The task options will be set to all internal tasks.
func (m *Manager) SetTelemetryHandler(handler TelemetryHandler, options ...svcinit.TaskOption) {
	if handler == nil {
		m.manager.AddInitError(fmt.Errorf("%w: telemetry handler cannot be nil", svcinit.ErrInitialization))
		return
	}
	if m.telemetryHandler != nil {
		m.manager.AddInitError(fmt.Errorf("%w: telemetry handler was already set", svcinit.ErrAlreadyInitialized))
		return
	}
	m.telemetryHandler = handler

	// flush the metrics as fast as possible on SIGTERM.
	m.AddTask(StageService, svcinit.BuildTask(
		// flush the current metrics as fast a possible.
		// We may not have enough time if the shutdown takes too long.
		svcinit.WithStop(m.TelemetryHandler().FlushTelemetry),
		svcinit.WithName(TaskNameTelemetryFlush),
	), options...)
}

// SetTelemetryHandlerTask uses the same instance on both SetTelemetryHandler and SetTelemetryTask.
func (m *Manager) SetTelemetryHandlerTask(handlerTask TelemetryHandlerTask, options ...svcinit.TaskOption) {
	if m.telemetryHandler != nil || m.telemetryTask != nil {
		m.manager.AddInitError(fmt.Errorf("%w: telemetry handler and/or task was already set", svcinit.ErrAlreadyInitialized))
		return
	}
	m.SetTelemetryHandler(handlerTask, options...)
	m.SetTelemetryTask(handlerTask, options...)
}

// SetTelemetryTask sets the telemetry task. It will be added to the "management" stage.
// The passed task options will be set to this task.
func (m *Manager) SetTelemetryTask(task svcinit.Task, options ...svcinit.TaskOption) {
	if m.telemetryTask != nil {
		m.manager.AddInitError(fmt.Errorf("%w: telemetry task was already set", svcinit.ErrAlreadyInitialized))
		return
	}
	m.telemetryTask = task

	// telemetry server must be the first to start and last to stop.
	m.AddTask(StageManagement, task, options...)
}

func (m *Manager) initRunTelemetry() {
	if m.telemetryHandler == nil {
		m.telemetryHandler = &noopTelemetryHandler{}
	}
}

func BuildTelemetryHandlerTask[T any](task svcinit.TaskWithData[T], options ...BuildTelemetryHandlerTaskOption[T]) TelemetryHandlerTask {
	ret := &taskBuildTelemetryHandler[T]{
		privateBaseOverloadedTask: &svcinit.BaseOverloadedTask[svcinit.TaskWithData[T]]{
			Task: task,
		},
	}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type BuildTelemetryHandlerTaskOption[T any] func(*taskBuildTelemetryHandler[T])

func WithDataFlushTelemetry[T any](f svcinit.TaskBuildDataFunc[T]) BuildTelemetryHandlerTaskOption[T] {
	return func(build *taskBuildTelemetryHandler[T]) {
		build.flushTelemetry = f
	}
}

// internal

type taskBuildTelemetryHandler[T any] struct {
	*privateBaseOverloadedTask[svcinit.TaskWithData[T]]
	flushTelemetry svcinit.TaskBuildDataFunc[T]
}

var _ svcinit.Task = (*taskBuildTelemetryHandler[svcinit.Task])(nil)
var _ TelemetryHandler = (*taskBuildTelemetryHandler[svcinit.Task])(nil)

func (t *taskBuildTelemetryHandler[T]) Run(ctx context.Context, step svcinit.Step) error {
	return t.Task.Run(ctx, step)
}

func (t *taskBuildTelemetryHandler[T]) FlushTelemetry(ctx context.Context) error {
	if t.flushTelemetry == nil {
		return nil
	}
	data, err := t.Task.TaskData()
	if err != nil {
		return err
	}
	return t.flushTelemetry(ctx, data)
}

type noopTelemetryHandler struct {
}

func (h *noopTelemetryHandler) FlushTelemetry(context.Context) error {
	return nil
}

type privateBaseOverloadedTask[T svcinit.Task] = svcinit.BaseOverloadedTask[T]
