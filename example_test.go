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

func ExampleSvcInit() {
	ctx := context.Background()

	// create core HTTP server
	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Addr: ":8080",
	}

	// create health HTTP server
	healthHTTPServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Addr: ":8081",
	}

	sinit := svcinit.New(ctx)

	// start core HTTP server using manual stop ordering.
	// it is only started on the Run call.
	httpStop := sinit.
		StartService(svcinit.ServiceTaskFunc(func(ctx context.Context) error {
			httpServer.BaseContext = func(net.Listener) context.Context {
				return ctx
			}
			return httpServer.ListenAndServe()
		}, func(ctx context.Context) error {
			return httpServer.Shutdown(ctx)
		})).
		ManualStop() // stop the service using the StopTask call WITHOUT cancelling the Start context.

	// start health HTTP server using manual stop ordering.
	// uses the task method instead of the service call. In the end it is the same thing, but the Service interface
	// can be implemented and reused.
	// it is only started on the Run call.
	healthStop := sinit.
		StartTaskFunc(func(ctx context.Context) error {
			healthHTTPServer.BaseContext = func(net.Listener) context.Context {
				return ctx
			}
			return healthHTTPServer.ListenAndServe()
		}).
		// stop the service using the StopTask call WITHOUT cancelling the Start context.
		ManualStopFunc(func(ctx context.Context) error {
			return healthHTTPServer.Shutdown(ctx)
		})

	// start a dummy task where the stop order doesn't matter.
	// unordered tasks are stopped in parallel.
	sinit.
		StartTaskFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
			}
			return nil
		}).
		AutoStop() // cancel the context of the start function on shutdown.

	// shutdown on OS signal.
	// it is only started on the Run call.
	sinit.ExecuteTask(svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

	// sleep 10 seconds and shutdown.
	// it is only started on the Run call.
	sinit.ExecuteTask(svcinit.TimeoutTask(1*time.Second, errors.New("timed out")))

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
