# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit?status.png)](https://godoc.org/github.com/rrgmc/svcinit)

## Features

- ordered and unordered shutdown callbacks.
- stop tasks can work with or without context cancellation.
- ensures no race condition if any starting job returns before all jobs initialized.
- when running ordered stop callbacks, ensures the actual starting job finished before running the next stop task, not only waiting for the stop task to finish.
- checks all added tasks are properly initialized and all ordered stop tasks are set, ensuring all created tasks always executes. 

## Example

```go
import (
    "context"
    "errors"
    "fmt"
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
        StartService(svcinit.ServiceFunc(func(ctx context.Context) error {
            fmt.Println("starting HTTP server")
            err := httpServer.ListenAndServe()
            fmt.Println("stopped HTTP server")
            return err
        }, func(ctx context.Context) error {
            fmt.Println("stopping HTTP server")
            return httpServer.Shutdown(ctx)
        })).
        Stop() // stop the service using the Stop call WITHOUT cancelling the Start context.

    // start health HTTP server using manual stop ordering.
    // it is only started on the Run call.
    healthStop := sinit.
        StartService(svcinit.ServiceFunc(func(ctx context.Context) error {
            fmt.Println("starting health HTTP server")
            err := healthHTTPServer.ListenAndServe()
            fmt.Println("stopped health HTTP server")
            return err
        }, func(ctx context.Context) error {
            fmt.Println("stopping health HTTP server")
            return healthHTTPServer.Shutdown(ctx)
        })).
        Stop() // stop the service using the Stop call WITHOUT cancelling the Start context.

    // start a dummy task where the stop order doesn't matter.
    // unordered tasks are stopped in parallel.
    sinit.
        StartTask(func(ctx context.Context) error {
            select {
            case <-ctx.Done():
            }
            return nil
        }).
        AutoStop() // cancel the context of the start function on shutdown.

    // stop tasks on OS signal.
    // it is only started on the Run call.
    sinit.ExecuteTask(svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

    // sleep 10 seconds and stops the task.
    // it is only started on the Run call.
    sinit.ExecuteTask(svcinit.TimeoutTask(2*time.Second, errors.New("timed out")))

    // add manual stops. They will be stopped in the added order.

    // stop HTTP server before health server
    sinit.StopTask(httpStop)
    sinit.StopTask(healthStop)

    err := sinit.Run()
    if err != nil {
		// err will be "timed out"
        fmt.Println("err:", err)
    }
}
```

## Author

Rangel Reale (rangelreale@gmail.com)
