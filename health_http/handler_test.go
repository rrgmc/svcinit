package health_http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"
)

func TestHandler(t *testing.T) {
	h := NewHandler(WithStartupProbe(true))
	mux := http.NewServeMux()
	h.Register(mux)

	check := func(path string, expected int) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, expected, w.Code, path)
	}

	check("/startup", http.StatusPreconditionFailed)
	check("/healthz", http.StatusOK)
	check("/ready", http.StatusPreconditionFailed)

	h.ServiceStarted(t.Context())
	check("/startup", http.StatusOK)
	check("/healthz", http.StatusOK)
	check("/ready", http.StatusOK)

	h.ServiceTerminating(t.Context())
	check("/startup", http.StatusOK)
	check("/healthz", http.StatusOK)
	check("/ready", http.StatusServiceUnavailable)
}
