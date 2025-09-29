package health_http

import "net/http"

type HTTPHandlerWrapper struct {
	handler       http.Handler
	healthHandler *Handler
}

var _ http.Handler = (*HTTPHandlerWrapper)(nil)

func NewHTTPHandlerWrapper(handler http.Handler, healthHandler *Handler) *HTTPHandlerWrapper {
	ret := &HTTPHandlerWrapper{
		handler:       handler,
		healthHandler: healthHandler,
	}
	return ret
}

func (h *HTTPHandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
