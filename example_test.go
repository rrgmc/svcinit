package svcinit_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/rrgmc/svcinit/v3"
)

// healthService implements an HTTP server used to serve health probes.
type healthService struct {
	server *http.Server
}

func newHealthService() *healthService {
	return &healthService{
		server: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			Addr: ":8081",
		},
	}
}

func (s *healthService) Start(ctx context.Context) error {
	s.server.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	return s.server.ListenAndServe()
}

func (s *healthService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func ExampleManager() {
	ctx := context.Background()

	// core HTTP server
	var httpServer *http.Server

	sinit, err := svcinit.New(
		// initialization in 3 stages. Initialization is done in stage order, and shutdown in reverse stage order.
		// all tasks added to the same stage are started/stopped in parallel.
		svcinit.WithStages(svcinit.StageDefault, "manage", "service"),
		// use a context with a 20-second cancellation during shutdown.
		svcinit.WithShutdownTimeout(20*time.Second),
		// some tasks may not check context cancellation, set enforce to true to give up waiting after the shutdown timeout.
		// The default is true.
		svcinit.WithEnforceShutdownTimeout(true),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// add a task to start health HTTP server before the service, and stop it after.
	sinit.AddTask("manage", svcinit.BuildDataTask[*healthService](
		// the "BuildDataTask" setup callback returns an instance that is sent to all following steps.
		func(ctx context.Context) (*healthService, error) {
			return newHealthService(), nil
		},
		svcinit.WithDataStart(func(ctx context.Context, data *healthService) error {
			return data.Start(ctx)
		}),
		svcinit.WithDataStop(func(ctx context.Context, data *healthService) error {
			return data.Stop(ctx)
		}),
	))

	// add a task to start the core HTTP server.
	sinit.
		AddTask("service", svcinit.BuildTask(
			svcinit.WithSetup(func(ctx context.Context) error {
				// initialize the service in the setup step.
				// as this may take some time in bigger services, initializing here allows other tasks to initialize
				// at the same time.
				httpServer = &http.Server{
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusOK)
					}),
					Addr: ":8080",
				}
				return nil
			}),
			svcinit.WithStart(func(ctx context.Context) error {
				httpServer.BaseContext = func(net.Listener) context.Context {
					return ctx
				}
				return httpServer.ListenAndServe()
			}),
			// stop the service. By default, the context is NOT cancelled, this method must arrange for the start
			// function to end.
			// Use [svcinit.WithCancelContext] if you want the context to be cancelled automatically after the
			// first task finishes.
			svcinit.WithStop(func(ctx context.Context) error {
				return httpServer.Shutdown(ctx)
			}),
		),
		// svcinit.WithCancelContext(true), // would cancel the "WithStart" context before calling "WithStop".
		)

	// shutdown on OS signal.
	sinit.AddTask(svcinit.StageDefault, svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

	// sleep 100ms and shutdown.
	sinit.AddTask(svcinit.StageDefault, svcinit.TimeoutTask(100*time.Millisecond,
		svcinit.WithTimeoutTaskError(errors.New("timed out"))))

	err = sinit.Run(ctx)
	if err != nil {
		fmt.Println("err:", err)
	}

	// Output: err: timed out
}
