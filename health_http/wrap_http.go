package health_http

import (
	"net/http"
	"sync/atomic"
)

type HTTPWrapper struct {
	httpHandler   atomic.Pointer[http.Handler]
	healthHandler *Handler
	mux           *http.ServeMux
}

var _ http.Handler = (*HTTPWrapper)(nil)

// NewHTTPWrapper returns an http.Handler which handles the probes before calling the final http handler.
func NewHTTPWrapper(healthHandler *Handler) *HTTPWrapper {
	ret := &HTTPWrapper{
		healthHandler: healthHandler,
		mux:           http.NewServeMux(),
	}
	// reuse Handler.Register so probe routes behave identically here as they would on a mux the caller
	// registers directly (method matching, path cleaning), instead of hand-rolling divergent matching.
	healthHandler.Register(ret.mux)
	ret.mux.Handle("/", http.HandlerFunc(ret.serveApp))
	return ret
}

func (h *HTTPWrapper) SetHTTPHandler(handler http.Handler) {
	h.httpHandler.Store(&handler)
}

func (h *HTTPWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *HTTPWrapper) serveApp(w http.ResponseWriter, r *http.Request) {
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
