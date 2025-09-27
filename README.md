# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit/v3?status.png)](https://godoc.org/github.com/rrgmc/svcinit/v3)

## Features

- manages start/stop ordering using stages.
- start tasks can stop with or without context cancellation.
- ensures no race condition if any starting job returns before all jobs initialized.

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

    "github.com/rrgmc/svcinit/v3"
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
        svcinit.WithStages(svcinit.StageDefault, "manage", "service"),
        // use a context with a 20-second cancellation during shutdown.
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
    sinit.AddTask("manage", healthHTTPServer)

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
```

## Real world example

- Start `management` stage:
  - run the `setup` step in parallel of these tasks and wait for the completion of all of them:
    - `telemetry`
    - `health service`
  - run the `start` step in parallel of these tasks but DON'T wait for their completion. They are expected to block
    until some condition makes then exit. To avoid some possible race condition, this step waits until all goroutines 
    START running to continue.
    - `Timeout 100ms` (waits 100ms and exits, a debugging tool)
    - `health service`
    - `Signals [interrupt interrupt terminated]` (waits until an OS signal is received)
- Start `initialize` stage:
  - run the `setup` step in parallel of these tasks and wait for the completion of all of them:
    - `init data`
- Start `ready` stage:
  - run the `setup` step in parallel of these tasks and wait for the completion of all of them:
    - `health server started probe` - signals the startup and readiness probe that the service is started. 
- Start `service` stage:
  - run the `setup` step in parallel of these tasks and wait for the completion of all of them:
    - `Messaging service`
    - `HTTP service`
  - run the `start` step in parallel of these tasks but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `Messaging service`
    - `HTTP service`
- Waits until the `start` step of any tasks returns an error (or nil).

- The first task to return in this example is `Timeout 100ms`, with the error `timed out`.
- Cancel the context sent to all services which have the `WithCancelContext(true)` option set using this `timed out` 
  error that was returned (in this example, only the `Timeout 100ms` and `Signals [interrupt interrupt terminated]` tasks).
- A context based on the root context (NOT the one sent to the tasks, that was just cancelled) with a deadline of
  20 seconds, is created and will be sent to all stopping jobs.

- Stop `service` stage:
  - run the `stop` step in parallel of these tasks and wait for the completion of all of them:
    - `HTTP service`
    - `Messaging service`
    - `telemetry flush` - flushes the pending telemetry to avoid losing it in case the service is killed.
    - `health server terminating probe` - signals the readiness probe that the service is terminating.
- Shutdown `management` stage:
    - run the `stop` step in parallel of these tasks and wait for the completion of all of them:
        - `health service`

- Wait until all tasks' `start` step returns or for the shutdown timeout.

- Teardown `initialize` stage:
  - run the `teardown` step in parallel of these tasks and wait for the completion of all of them:
    - `init data` - closes the DB connection.
- Teardown `management` stage:
  - run the `teardown` step in parallel of these tasks and wait for the completion of all of them:
    - `telemetry`

- The `Run` method will return the error `timed out`.

## Author

Rangel Reale (rangelreale@gmail.com)
