package k8sinit

import (
	"context"

	"github.com/rrgmc/svcinit/v3"
)

type TelemetryHandler interface {
	Flush(ctx context.Context) error
}

type TelemetryHandlerTask interface {
	TelemetryHandler
	svcinit.Task
}

func (m *Manager) initTelemetry() error {
	if m.telemetryHandler == nil {
		m.telemetryHandler = &noopTelemetryHandler{}
	}

	if m.telemetryTask != nil {
		m.AddTask(StageManagement, m.telemetryTask)
	}

	// flush the metrics as fast as possible on SIGTERM.
	m.AddTask(StageService, svcinit.BuildTask(
		// flush the current metrics as fast a possible.
		// We may not have enough time if the shutdown takes too long.
		svcinit.WithStop(m.telemetryHandler.Flush),
		svcinit.WithName("telemetry: flush"),
	))

	return nil
}

// internal

type noopTelemetryHandler struct {
}

func (h *noopTelemetryHandler) Flush(ctx context.Context) error {
	return nil
}
