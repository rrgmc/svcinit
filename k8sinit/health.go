package k8sinit

import (
	"context"

	"github.com/rrgmc/svcinit/v3"
)

type HealthHandler interface {
	ServiceStarted()
	ServiceTerminating()
}

type HealthHandlerTask interface {
	HealthHandler
	svcinit.Task
}

func (m *Manager) initHealth() error {
	if m.healthHandler == nil {
		m.healthHandler = &noopHealthHandler{}
	}

	if m.healthTask != nil {
		// health server must be the first to start and last to stop.
		m.AddTask(StageManagement, m.healthTask)
	}

	// the "ready" stage is executed after all initialization already happened. It is used to signal the
	// startup probes that the service is ready.
	m.AddTask(StageReady, svcinit.BuildTask(
		svcinit.WithSetup(func(ctx context.Context) error {
			m.manager.Logger().DebugContext(ctx, "service started, signaling probes")
			m.healthHandler.ServiceStarted()
			return nil
		}),
		svcinit.WithName("health handler: started probe"),
	))

	// add a task in the "service" stage, so the stop step is called in parallel with the service stopping ones.
	// This tasks signals the probes that the service is terminating.
	m.AddTask(StageService, svcinit.BuildTask(
		svcinit.WithStop(func(ctx context.Context) error {
			m.manager.Logger().DebugContext(ctx, "service terminating, signaling probes")
			m.healthHandler.ServiceTerminating()
			return nil
		}),
		svcinit.WithName("health server: terminating probe"),
	))

	return nil
}

type noopHealthHandler struct {
}

func (h *noopHealthHandler) ServiceStarted()     {}
func (h *noopHealthHandler) ServiceTerminating() {}
