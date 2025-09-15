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

	"github.com/rrgmc/svcinit"
)

// healthService wraps an HTTP service in a [svcinit.Service] interface.
type healthService struct {
	server *http.Server
}

var _ svcinit.Service = (*healthService)(nil)

func (s *healthService) Start(ctx context.Context) error {
	s.server.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	return s.server.ListenAndServe()
}

func (s *healthService) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func newHealthService() *healthService {
	// create health HTTP server
	healthHTTPServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Addr: ":8081",
	}
	return &healthService{healthHTTPServer}
}

func ExampleSvcInit() {
	ctx := context.Background()

	// create health HTTP server
	healthHTTPServer := newHealthService()

	// create core HTTP server
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Addr: ":8080",
	}

	sinit := svcinit.New(ctx)

	// start health HTTP server as a service using manual stop ordering.
	// it is only started on the Run call.
	healthStop := sinit.
		StartService(healthHTTPServer).
		// stop the service using the Stop call WITHOUT cancelling the Start context.
		Stop()

	// start core HTTP server using manual stop ordering.
	// uses the task method instead of the service call. In the end it is the same thing, but the Service interface
	// can be implemented and reused.
	// it is only started on the Run call.
	httpStop := sinit.
		Start(svcinit.TaskFunc(func(ctx context.Context) error {
			httpServer.BaseContext = func(net.Listener) context.Context {
				return ctx
			}
			return httpServer.ListenAndServe()
		})).
		// stop the service using the Stop call WITHOUT cancelling the Start context.
		Stop(svcinit.TaskFunc(func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		}))

	// start a dummy task where the stop order doesn't matter.
	// unordered tasks are stopped in parallel.
	sinit.
		Start(svcinit.TaskFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
			}
			return nil
		})).
		AutoStop() // cancel the context of the start function on shutdown.

	// shutdown on OS signal.
	// it is only started on the Run call.
	sinit.Execute(svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

	// sleep 10 seconds and shutdown.
	// it is only started on the Run call.
	sinit.Execute(svcinit.TimeoutTask(1*time.Second, errors.New("timed out")))

	// add manual stops. They will be stopped in the added order.

	// stop HTTP server before health server
	sinit.StopTask(httpStop)
	sinit.StopTask(healthStop)

	err := sinit.Run()
	if err != nil {
		// err will be "timed out"
		fmt.Println("err:", err)
	}

	// Output: err: timed out
}
