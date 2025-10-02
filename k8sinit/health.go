package k8sinit

import (
	"context"
	"fmt"

	"github.com/rrgmc/svcinit/v3"
)

// SetHealthHandler sets a health handler. The task options will be set to all internal tasks.
func (m *Manager) SetHealthHandler(handler svcinit.HealthHandler, options ...svcinit.TaskOption) {
	if handler == nil {
		m.manager.AddInitError(fmt.Errorf("%w: health handler cannot be nil", svcinit.ErrInitialization))
		return
	}
	if m.healthHandler != nil {
		m.manager.AddInitError(fmt.Errorf("%w: health handler was already set", svcinit.ErrAlreadyInitialized))
		return
	}
	m.healthHandler = handler

	// the "ready" stage is executed after all initialization already happened. It is used to signal the
	// startup probes that the service is ready.
	m.AddTask(StageReady, svcinit.BuildTask(
		svcinit.WithSetup(func(ctx context.Context) error {
			m.logger.DebugContext(ctx, "service started, signaling probes")
			m.HealthHandler().ServiceStarted(ctx)
			return nil
		}),
		svcinit.WithName(TaskNameHealthStartedProbe),
	), options...)

	// add a task in the "service" stage, so the stop step is called in parallel with the service stopping ones.
	// This tasks signals the probes that the service is terminating.
	m.AddTask(StageService, svcinit.BuildTask(
		svcinit.WithStop(func(ctx context.Context) error {
			m.logger.DebugContext(ctx, "service terminating, signaling probes")
			m.HealthHandler().ServiceTerminating(ctx)
			return nil
		}),
		svcinit.WithName(TaskNameHealthTerminatingProbe),
	), options...)
}

// SetHealthTask sets the health task. It will be added to the "management" stage.
// The passed task options will be set to this task.
func (m *Manager) SetHealthTask(task svcinit.Task, options ...svcinit.TaskOption) {
	if m.healthTask != nil {
		m.manager.AddInitError(fmt.Errorf("%w: health task was already set", svcinit.ErrAlreadyInitialized))
		return
	}
	m.healthTask = task

	// health server must be the first to start and last to stop.
	m.AddTask(StageManagement, task, options...)
}

func (m *Manager) initRunHealth() {
	if m.healthHandler == nil {
		m.healthHandler = &noopHealthHandler{}
	}
}

type noopHealthHandler struct {
}

func (h *noopHealthHandler) ServiceStarted(context.Context)     {}
func (h *noopHealthHandler) ServiceTerminating(context.Context) {}
