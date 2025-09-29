package health_http

import "net/http"

type Wrapper struct {
	httpHandler   http.Handler
	healthHandler *Handler
}

var _ http.Handler = (*Wrapper)(nil)

func NewWrapper(handler http.Handler, options ...HandlerOption) *Wrapper {
	ret := &Wrapper{
		httpHandler:   handler,
		healthHandler: NewHandler(options...),
	}
	return ret
}

func (h *Wrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	h.httpHandler.ServeHTTP(w, r)
}
