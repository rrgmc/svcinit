package k8sinit

import (
	"net/http"
	"sync/atomic"
)

type HealthHTTPHandler struct {
	StartupHandler   http.Handler
	LivenessHandler  http.Handler
	ReadinessHandler http.Handler

	startupProbe             bool
	isStarted, isTerminating atomic.Bool
}

var _ HealthHandler = (*HealthHTTPHandler)(nil)

func NewHealthHTTPHandler(options ...HealthHTTPHandlerOption) *HealthHTTPHandler {
	ret := &HealthHTTPHandler{}
	for _, option := range options {
		option(ret)
	}
	ret.initHandlers()
	return ret
}

func (h *HealthHTTPHandler) ServiceStarted() {
	h.isStarted.Store(true)
}

func (h *HealthHTTPHandler) ServiceTerminating() {
	h.isTerminating.Store(true)
}

func (h *HealthHTTPHandler) initHandlers() {
	h.StartupHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.startupProbe && !h.isStarted.Load() {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h.LivenessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h.ReadinessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.startupProbe && !h.isStarted.Load() {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if h.isTerminating.Load() {
			w.WriteHeader(499) // https://www.webfx.com/web-development/glossary/http-status-codes/what-is-a-499-status-code/
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type HealthHTTPHandlerOption func(*HealthHTTPHandler)

func WithHealthHTTPHandlerStartupProbe(startupProbe bool) HealthHTTPHandlerOption {
	return func(h *HealthHTTPHandler) {
		h.startupProbe = startupProbe
	}
}
