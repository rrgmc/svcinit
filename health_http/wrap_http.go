package health_http

import (
	"net/http"
	"sync/atomic"
)

type HTTPWrapper struct {
	httpHandler   atomic.Pointer[http.Handler]
	healthHandler *Handler
}

var _ http.Handler = (*HTTPWrapper)(nil)

// NewHTTPWrapper returns an http.Handler which handles the probes before calling the final http handler.
func NewHTTPWrapper(healthHandler *Handler) *HTTPWrapper {
	ret := &HTTPWrapper{
		healthHandler: healthHandler,
	}
	return ret
}

func (h *HTTPWrapper) SetHTTPHandler(handler http.Handler) {
	h.httpHandler.Store(&handler)
}

func (h *HTTPWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte("service not ready"))
		return
	}
	if httpHandler := h.httpHandler.Load(); httpHandler != nil {
		(*httpHandler).ServeHTTP(w, r)
		return
	}
	http.NotFoundHandler().ServeHTTP(w, r)
}
