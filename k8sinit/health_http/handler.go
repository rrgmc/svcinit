package health_http

import (
	"net/http"
	"sync/atomic"

	"github.com/rrgmc/svcinit/v3/k8sinit"
)

type Handler struct {
	StartupHandler   http.Handler
	LivenessHandler  http.Handler
	ReadinessHandler http.Handler

	StartupProbePath   string
	LivenessProbePath  string
	ReadinessProbePath string

	startupProbe             bool
	isStarted, isTerminating atomic.Bool
}

var _ k8sinit.HealthHandler = (*Handler)(nil)

func NewHandler(options ...HandlerOption) *Handler {
	ret := &Handler{
		StartupProbePath:   "/startup",
		LivenessProbePath:  "/healthz",
		ReadinessProbePath: "/ready",
	}
	for _, option := range options {
		option.applyHandlerOption(ret)
	}
	ret.init()
	return ret
}

func (h *Handler) ServiceStarted() {
	h.isStarted.Store(true)
}

func (h *Handler) ServiceTerminating() {
	h.isTerminating.Store(true)
}

func (h *Handler) IsStarted() bool {
	return h.isStarted.Load()
}

func (h *Handler) IsTerminating() bool {
	return h.isTerminating.Load()
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET "+h.StartupProbePath, h.StartupHandler)
	mux.Handle("GET "+h.LivenessProbePath, h.LivenessHandler)
	mux.Handle("GET "+h.ReadinessProbePath, h.ReadinessHandler)
}

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

// internal

func (h *Handler) init() {
	if !h.startupProbe {
		h.isStarted.Store(true)
	}
	h.StartupHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.startupProbe && !h.isStarted.Load() {
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte("service not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h.LivenessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h.ReadinessHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.startupProbe && !h.isStarted.Load() {
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte("service not ready"))
			return
		}
		if h.isTerminating.Load() {
			w.WriteHeader(499) // https://www.webfx.com/web-development/glossary/http-status-codes/what-is-a-499-status-code/
			_, _ = w.Write([]byte("service shutting down"))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
