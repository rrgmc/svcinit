package k8sinit

import (
	"context"
	"fmt"

	"github.com/rrgmc/svcinit/v3"
)

type TelemetryHandler interface {
	Flush(ctx context.Context) error
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
		svcinit.WithStop(m.TelemetryHandler().Flush),
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

// internal

type noopTelemetryHandler struct {
}

func (h *noopTelemetryHandler) Flush(ctx context.Context) error {
	return nil
}
