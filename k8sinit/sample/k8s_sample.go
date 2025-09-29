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

	sinit, err := k8sinit.New(
		k8sinit.WithHealthHandlerTask(health_http.NewServer()),
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
			mux.Handle("GET /test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World, test"))
			}))
			mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Hello World"))
			}))

			return &http.Server{
				Handler: mux,
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

	return sinit.Run(ctx)
}
