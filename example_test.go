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

	"github.com/rrgmc/svcinit/v2"
)

// healthService wraps an HTTP service in a [svcinit.Task] interface.
type healthService struct {
	server *http.Server
}

var _ svcinit.Task = (*healthService)(nil)

func (s *healthService) Run(ctx context.Context, step svcinit.Step) error {
	switch step {
	case svcinit.StepSetup:
		// initialize the service in the setup step.
		s.server = &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			Addr: ":8081",
		}
	case svcinit.StepStart:
		s.server.BaseContext = func(net.Listener) context.Context {
			return ctx
		}
		return s.server.ListenAndServe()
	case svcinit.StepStop:
		return s.server.Shutdown(ctx)
	case svcinit.StepPreStop:
		// called just before shutdown starts.
		// This could be used to make the readiness probe to fail during shutdown for example.
	default:
	}
	return nil
}

func ExampleManager() {
	ctx := context.Background()

	// create health HTTP server
	healthHTTPServer := &healthService{}

	// create core HTTP server
	var httpServer *http.Server

	sinit, err := svcinit.New(
		// initialization in 2 stages. Initialization is done in stage order, and shutdown in reverse stage order.
		// all tasks added to the same stage are started/stopped in parallel.
		svcinit.WithStages("manage", "service"),
		// default stage if the task don't set one.
		svcinit.WithDefaultStage("manage"),
		// use a context with a 10-second cancellation in the stop tasks.
		svcinit.WithShutdownTimeout(10*time.Second),
		// some tasks may not check context cancellation, set enforce to true to give up waiting after the shutdown timeout.
		// The default is true.
		svcinit.WithEnforceShutdownTimeout(true),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// add a task to start health HTTP server before the service, and stop it after.
	sinit.AddTask(healthHTTPServer)

	// add a task to start the core HTTP server.
	sinit.
		AddTask(svcinit.BuildTask(
			svcinit.WithSetup(func(ctx context.Context) error {
				// initialize the service in the setup step.
				// as this may take some time in bigger services, initializing here allows other tasks to setup
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
			svcinit.WithStage("service"), // will run in the "service" stage.
			// svcinit.WithCancelContext(true), // would cancel the "WithStart" context before calling "WithStop".
		)

	// shutdown on OS signal.
	sinit.AddTask(svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

	// sleep 1 second and shutdown.
	sinit.AddTask(svcinit.TimeoutTask(1*time.Second,
		svcinit.WithTimeoutTaskError(errors.New("timed out"))))

	err = sinit.Run(ctx)
	if err != nil {
		fmt.Println("err:", err)
	}

	// Output: err: timed out
}
