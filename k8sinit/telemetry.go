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

// BuildTelemetryHandler builds a TelemetryHandler from callback functions.
func BuildTelemetryHandler(options ...BuildTelemetryHandlerOption) TelemetryHandler {
	ret := &buildTelemetryHandler{}
	for _, option := range options {
		option(ret)
	}
	return ret
}

type BuildTelemetryHandlerOption func(*buildTelemetryHandler)

func WithTelemetryHandlerFlushTelemetry(f svcinit.TaskBuildFunc) BuildTelemetryHandlerOption {
	return func(build *buildTelemetryHandler) {
		build.flushTelemetry = f
	}
}

// internal

type buildTelemetryHandler struct {
	flushTelemetry svcinit.TaskBuildFunc
}

var _ TelemetryHandler = (*buildTelemetryHandler)(nil)

func (t *buildTelemetryHandler) FlushTelemetry(ctx context.Context) error {
	if t.flushTelemetry == nil {
		return nil
	}
	return t.flushTelemetry(ctx)
}

type noopTelemetryHandler struct {
}

func (h *noopTelemetryHandler) FlushTelemetry(context.Context) error {
	return nil
}

type privateBaseOverloadedTask[T svcinit.Task] = svcinit.BaseOverloadedTask[T]
