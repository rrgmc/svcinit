package main

import (
	"context"
	"net/http"
	"os"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/k8sinit"
	"github.com/rrgmc/svcinit/v3/k8sinit/health_http"
)

// runSingleHTTP uses the same HTTP server for both health and the service itself.
func runSingleHTTP(ctx context.Context) error {
	logger := defaultLogger(os.Stdout)

	healthHandler := health_http.NewHandler(health_http.WithStartupProbe(true))
	httpHandlerWrapper := health_http.NewWrapper(healthHandler)

	sinit, err := k8sinit.New(
		k8sinit.WithHealthHandler(healthHandler),
		k8sinit.WithManagerOptions(
			svcinit.WithLogger(logger),
		),
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
