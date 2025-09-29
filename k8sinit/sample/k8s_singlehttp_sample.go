package main

import (
	"context"
	"net/http"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/health_http"
	"github.com/rrgmc/svcinit/v3/k8sinit"
)

// runSingleHTTP uses the same HTTP server for both health and the service itself.
func runSingleHTTP(ctx context.Context) error {
	// handler for the health endpoints
	healthHandler := health_http.NewHandler(
		health_http.WithStartupProbe(true), // fails startup and readiness probes until service is started.
	)
	// HTTP handler wrapper which handles the health requests, and forward the other to the real handler.
	// The real handler will be set in a following step.
	httpHandlerWrapper := health_http.NewWrapper(healthHandler)

	sinit, err := k8sinit.New(
		k8sinit.WithHealthHandler(healthHandler),
	)
	if err != nil {
		return err
	}

	// start the main HTTP server in the management stage, which is where the health service must run.
	sinit.AddTask(k8sinit.StageManagement, svcinit.BuildDataTask[*http.Server](
		func(ctx context.Context) (*http.Server, error) {
			mux := http.NewServeMux()
			healthHandler.Register(mux)
			return &http.Server{
				Handler: httpHandlerWrapper,
				Addr:    ":8080",
			}, nil
		},
		svcinit.WithDataStart(func(ctx context.Context, service *http.Server) error {
			return service.ListenAndServe()
		}),
		svcinit.WithDataStop(func(ctx context.Context, service *http.Server) error {
			return service.Shutdown(ctx)
		}),
	))

	//
	// initialize and start the HTTP service.
	// It will set the real HTTP handler to the health handler wrapped one. It will handle the health endpoints,
	// and forward the other requests to this handler.
	//
	sinit.AddTask(k8sinit.StageService, svcinit.BuildTask(
		svcinit.WithSetup(func(ctx context.Context) error {
			mux := http.NewServeMux()
			mux.Handle("GET /test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World, test"))
			}))
			mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World"))
			}))
			// set the real HTTP handler mux.
			httpHandlerWrapper.SetHTTPHandler(mux)
			return nil
		}),
	))

	return sinit.Run(ctx)
}
