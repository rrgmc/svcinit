package k8sinit

import (
	"net/http"
	"sync/atomic"
)

type HealthHTTPHandler struct {
	StartupHandler   http.Handler
	LivenessHandler  http.Handler
	ReadinessHandler http.Handler

	StartupProbePath   string
	LivenessProbePath  string
	ReadinessProbePath string

	startupProbe             bool
	isStarted, isTerminating atomic.Bool
}

var _ HealthHandler = (*HealthHTTPHandler)(nil)

func NewHealthHTTPHandler(options ...HealthHTTPHandlerOption) *HealthHTTPHandler {
	ret := &HealthHTTPHandler{
		StartupProbePath:   "/startup",
		LivenessProbePath:  "/healthz",
		ReadinessProbePath: "/ready",
	}
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

func (h *HealthHTTPHandler) IsStarted() bool {
	return h.isStarted.Load()
}

func (h *HealthHTTPHandler) IsTerminating() bool {
	return h.isTerminating.Load()
}

func (h *HealthHTTPHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET "+h.StartupProbePath, h.StartupHandler)
	mux.Handle("GET "+h.LivenessProbePath, h.LivenessHandler)
	mux.Handle("GET "+h.ReadinessProbePath, h.ReadinessHandler)
}

type HealthHTTPHandlerWrapper struct {
	handler       http.Handler
	healthHandler *HealthHTTPHandler
}

var _ http.Handler = (*HealthHTTPHandlerWrapper)(nil)

func NewHealthHTTPHandlerWrapper(handler http.Handler, healthHandler *HealthHTTPHandler) *HealthHTTPHandlerWrapper {
	ret := &HealthHTTPHandlerWrapper{
		handler:       handler,
		healthHandler: healthHandler,
	}
	return ret
}

func (h *HealthHTTPHandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if r.URL.Path == h.healthHandler.StartupProbePath {
			h.healthHandler.StartupHandler.ServeHTTP(w, r)
			return
		} else if r.URL.Path == h.healthHandler.LivenessProbePath {
			h.healthHandler.LivenessHandler.ServeHTTP(w, r)
			return
		} else if r.URL.Path == h.healthHandler.ReadinessProbePath {
			h.healthHandler.ReadinessHandler.ServeHTTP(w, r)
			return
		}
	}
	if !h.healthHandler.IsStarted() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.handler.ServeHTTP(w, r)
}

func WithHealthStartupProbePath(path string) HealthHTTPHandlerOption {
	return func(h *HealthHTTPHandler) {
		h.StartupProbePath = path
	}
}

func WithHealthLivenessProbePath(path string) HealthHTTPHandlerOption {
	return func(h *HealthHTTPHandler) {
		h.LivenessProbePath = path
	}
}

func WithHealthReadinessProbePath(path string) HealthHTTPHandlerOption {
	return func(h *HealthHTTPHandler) {
		h.ReadinessProbePath = path
	}
}

// internal

func (h *HealthHTTPHandler) initHandlers() {
	if !h.startupProbe {
		h.isStarted.Store(true)
	}
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
