# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit?status.png)](https://godoc.org/github.com/rrgmc/svcinit)

## Features

- ordered and unordered stop callbacks.
- stop tasks can work with or without context cancellation.
- ensures no race condition if any starting job returns before all jobs initialized.
- checks all added tasks are properly initialized and all ordered stop tasks are set, ensuring all created tasks always executes. 

## Example

```go
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

func (s *healthService) RunService(ctx context.Context, stage svcinit.Stage) error {
    switch stage {
    case svcinit.StageStart:
        s.server.BaseContext = func(net.Listener) context.Context {
            return ctx
        }
        return s.server.ListenAndServe()
    case svcinit.StageStop:
        return s.server.Shutdown(ctx)
    default:
    }
    return nil
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

func ExampleManager() {
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
        // stop the service using the [svcinit.Manager.FutureStop] call WITHOUT cancelling the start context.
        FutureStop()

    // start core HTTP server using manual stop ordering.
    // uses the task method instead of the service call. In the end it is the same thing, but the Service interface
    // can be implemented and reused.
    // it is only started on the Run call.
    httpStop := sinit.
        StartTask(svcinit.TaskFunc(func(ctx context.Context) error {
            httpServer.BaseContext = func(net.Listener) context.Context {
                return ctx
            }
            return httpServer.ListenAndServe()
        })).
        // called just before shutdown starts.
        // This could be used to make the readiness probe to fail during shutdown.
        PreStop(svcinit.TaskFunc(func(ctx context.Context) error {
            return nil
        })).
        // stop the service using the [svcinit.Manager.FutureStop] call WITHOUT cancelling the start context.
        FutureStop(svcinit.TaskFunc(func(ctx context.Context) error {
            return httpServer.Shutdown(ctx)
        }))

    // start a dummy task where the stop order doesn't matter.
    // unordered tasks are stopped in parallel.
    sinit.
        StartTask(svcinit.TaskFunc(func(ctx context.Context) error {
            select {
            case <-ctx.Done():
            }
            return nil
        })).
        AutoStopContext() // cancel the context of the start function on shutdown.

    // shutdown on OS signal.
    // it is only started on the Run call.
    sinit.ExecuteTask(svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

    // sleep 10 seconds and shutdown.
    // it is only started on the Run call.
    sinit.ExecuteTask(svcinit.TimeoutTask(1*time.Second, errors.New("timed out")))

    // add manual stops. They will be stopped in the added order.

    // stop HTTP server before health server
    sinit.StopFuture(httpStop)
    sinit.StopFuture(healthStop)

    err := sinit.Run()
    if err != nil {
        // err will be "timed out"
        fmt.Println("err:", err)
    }

    // Output: err: timed out
}
```

## Author

Rangel Reale (rangelreale@gmail.com)
