package health_http

import (
	"net/http"
	"sync/atomic"

	"github.com/rrgmc/svcinit/v3"
)

type Probe int

const (
	ProbeStartup Probe = iota
	ProbeLiveness
	ProbeReadiness
)

type Status struct {
	IsStarted     bool
	IsTerminating bool
}

type ProbeHandler interface {
	ServeHTTP(Probe, Status, http.ResponseWriter, *http.Request)
}

type ProbeHandlerFunc func(Probe, Status, http.ResponseWriter, *http.Request)

func (f ProbeHandlerFunc) ServeHTTP(probe Probe, status Status, w http.ResponseWriter, r *http.Request) {
	f(probe, status, w, r)
}

// Handler is a container for HTTP health handlers.
// The handlers must be added to some HTTP server, using Register or other custom method.
type Handler struct {
	StartupHandler   http.Handler
	LivenessHandler  http.Handler
	ReadinessHandler http.Handler

	StartupProbePath   string
	LivenessProbePath  string
	ReadinessProbePath string

	startupProbe             bool
	probeHandler             ProbeHandler
	isStarted, isTerminating atomic.Bool
}

var _ svcinit.HealthHandler = (*Handler)(nil)

func NewHandler(options ...HandlerOption) *Handler {
	ret := &Handler{
		StartupProbePath:   "/startup",
		LivenessProbePath:  "/healthz",
		ReadinessProbePath: "/ready",
	}
	for _, option := range options {
		option.applyHandlerOption(ret)
	}
	if ret.probeHandler == nil {
		ret.probeHandler = ProbeHandlerFunc(DefaultProbeHandler)
	}
	ret.init()
	return ret
}

// ServiceStarted signals the handler that the service has started.
func (h *Handler) ServiceStarted() {
	h.isStarted.Store(true)
}

// ServiceTerminating signals the handler that the service is terminating.
func (h *Handler) ServiceTerminating() {
	h.isTerminating.Store(true)
}

// IsStarted returns whether ServiceStarted was called.
func (h *Handler) IsStarted() bool {
	return h.isStarted.Load()
}

// IsTerminating returns whether ServiceTerminating was called.
func (h *Handler) IsTerminating() bool {
	return h.isTerminating.Load()
}

// Register adds the health handlers to an [http.ServeMux].
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET "+h.StartupProbePath, h.StartupHandler)
	mux.Handle("GET "+h.LivenessProbePath, h.LivenessHandler)
	mux.Handle("GET "+h.ReadinessProbePath, h.ReadinessHandler)
}

// WithStartupProbe sets whether to check for startup initialization.
// If this is true, until [Handler.ServiceStarted] is called:
// - the startup probe will fail.
// - the readiness probe will fail.
// The default is false, which is the same as if [Handler.ServiceStarted] was already called.
func WithStartupProbe(startupProbe bool) HandlerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.handlerOptions = append(server.handlerOptions, WithStartupProbe(startupProbe))
		},
		handlerOpt: func(handler *Handler) {
			handler.startupProbe = startupProbe
		},
	}
}

// WithStartupProbePath sets the HTTP startup probe path.
// The default is "/startup".
func WithStartupProbePath(path string) HandlerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.handlerOptions = append(server.handlerOptions, WithStartupProbePath(path))
		},
		handlerOpt: func(handler *Handler) {
			handler.StartupProbePath = path
		},
	}
}

// WithLivenessProbePath sets the HTTP liveness probe path.
// The default is "/healthz".
func WithLivenessProbePath(path string) HandlerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.handlerOptions = append(server.handlerOptions, WithLivenessProbePath(path))
		},
		handlerOpt: func(handler *Handler) {
			handler.LivenessProbePath = path
		},
	}
}

// WithReadinessProbePath sets the HTTP readiness probe path.
// The default is "/ready".
func WithReadinessProbePath(path string) HandlerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.handlerOptions = append(server.handlerOptions, WithReadinessProbePath(path))
		},
		handlerOpt: func(handler *Handler) {
			handler.ReadinessProbePath = path
		},
	}
}

// WithProbeHandler sets the handler to use for probe HTTP responses.
// If not set, DefaultProbeHandler will be used.
func WithProbeHandler(h ProbeHandler) HandlerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.handlerOptions = append(server.handlerOptions, WithProbeHandler(h))
		},
		handlerOpt: func(handler *Handler) {
			handler.probeHandler = h
		},
	}
}

// internal

func (h *Handler) init() {
	if !h.startupProbe {
		h.isStarted.Store(true)
	}
	h.StartupHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.probeHandler.ServeHTTP(ProbeStartup, Status{
			IsStarted:     h.isStarted.Load(),
			IsTerminating: h.isTerminating.Load(),
		}, w, r)
	})
	h.LivenessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.probeHandler.ServeHTTP(ProbeLiveness, Status{
			IsStarted:     h.isStarted.Load(),
			IsTerminating: h.isTerminating.Load(),
		}, w, r)
	})
	h.ReadinessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.probeHandler.ServeHTTP(ProbeReadiness, Status{
			IsStarted:     h.isStarted.Load(),
			IsTerminating: h.isTerminating.Load(),
		}, w, r)
	})
}

func DefaultProbeHandler(probe Probe, status Status, w http.ResponseWriter, r *http.Request) {
	if probe == ProbeLiveness {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !status.IsStarted {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte("service not ready"))
		return
	}
	if probe == ProbeReadiness {
		if status.IsTerminating {
			w.WriteHeader(499) // https://www.webfx.com/web-development/glossary/http-status-codes/what-is-a-499-status-code/
			_, _ = w.Write([]byte("service shutting down"))
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}
