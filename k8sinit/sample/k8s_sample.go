package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/k8sinit"
	"github.com/rrgmc/svcinit/v3/k8sinit/health_http"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Println(err)
	}
}

func run(ctx context.Context) error {
	logger := defaultLogger(os.Stdout)

	healthHandler := health_http.NewHandler(health_http.WithStartupProbe(true))

	sinit, err := k8sinit.New(
		// k8sinit.WithHealthHandler(health_http.NewServer()),
		k8sinit.WithHealthHandler(healthHandler),
		k8sinit.WithManagerOptions(
			svcinit.WithLogger(logger),
		),
	)
	if err != nil {
		return err
	}

	sinit.AddTask(k8sinit.StageService, svcinit.BuildDataTask[*http.Server](
		func(ctx context.Context) (*http.Server, error) {
			mux := http.NewServeMux()
			healthHandler.Register(mux)
			mux.Handle("GET /test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World, test"))
			}))
			mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World"))
			}))

			return &http.Server{
				Handler: health_http.NewWrapper(mux, healthHandler),
				Addr:    ":8080",
			}, nil
		},
		svcinit.WithDataStart(func(ctx context.Context, data *http.Server) error {
			return data.ListenAndServe()
		}),
		svcinit.WithDataStop(func(ctx context.Context, data *http.Server) error {
			return data.Shutdown(ctx)
		}),
	))

	return sinit.Run(ctx)
}
