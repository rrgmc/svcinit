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

func (h *HealthHTTPHandler) IsStarted() bool {
	return h.isStarted.Load()
}

func (h *HealthHTTPHandler) IsTerminating() bool {
	return h.isTerminating.Load()
}

type HealthHTTPHandlerWrapper struct {
	handler       http.Handler
	healthHandler *HealthHTTPHandler
	httpOptions   healthHTTPOptions
}

var _ http.Handler = (*HealthHTTPHandlerWrapper)(nil)

func NewHealthHTTPHandlerWrapper(handler http.Handler, healthHandler *HealthHTTPHandler,
	options ...HealthHTTPOption) *HealthHTTPHandlerWrapper {
	ret := &HealthHTTPHandlerWrapper{
		handler:       handler,
		healthHandler: healthHandler,
		httpOptions:   newHealthHTTPOptions(),
	}
	for _, option := range options {
		option.applyHealthHTTPOption(&ret.httpOptions)
	}
	return ret
}

func (h *HealthHTTPHandlerWrapper) Register(mux *http.ServeMux) {
	mux.Handle("GET "+h.httpOptions.startupProbePath, h.healthHandler.StartupHandler)
	mux.Handle("GET "+h.httpOptions.livenessProbePath, h.healthHandler.LivenessHandler)
	mux.Handle("GET "+h.httpOptions.readinessProbePath, h.healthHandler.ReadinessHandler)
}

func (h *HealthHTTPHandlerWrapper) Paths() (startupProbePath string, livenessProbePath string,
	readinessProbePath string) {
	return h.httpOptions.startupProbePath, h.httpOptions.livenessProbePath, h.httpOptions.readinessProbePath
}

func (h *HealthHTTPHandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if r.URL.Path == h.httpOptions.startupProbePath {
			h.healthHandler.StartupHandler.ServeHTTP(w, r)
			return
		} else if r.URL.Path == h.httpOptions.livenessProbePath {
			h.healthHandler.LivenessHandler.ServeHTTP(w, r)
			return
		} else if r.URL.Path == h.httpOptions.readinessProbePath {
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

func WithHealthStartupProbePath(path string) HealthHTTPOption {
	return &healthHTTPServerOption{
		httpOption: func(o *healthHTTPOptions) {
			o.startupProbePath = path
		},
		serverOption: func(o *HealthHTTPServer) {
			o.httpOptions.startupProbePath = path
		},
	}
}

func WithHealthLivenessProbePath(path string) HealthHTTPOption {
	return &healthHTTPServerOption{
		httpOption: func(o *healthHTTPOptions) {
			o.livenessProbePath = path
		},
		serverOption: func(o *HealthHTTPServer) {
			o.httpOptions.livenessProbePath = path
		},
	}
}

func WithHealthReadinessProbePath(path string) HealthHTTPOption {
	return &healthHTTPServerOption{
		httpOption: func(o *healthHTTPOptions) {
			o.readinessProbePath = path
		},
		serverOption: func(o *HealthHTTPServer) {
			o.httpOptions.readinessProbePath = path
		},
	}
}

type HealthHTTPBaseOption interface {
	applyHealthHTTPOption(*healthHTTPOptions)
}

type HealthHTTPOption interface {
	HealthHTTPBaseOption
	HealthHTTPServerOption
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
