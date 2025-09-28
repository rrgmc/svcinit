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
        // initialization in 2 stages. Initialization is done in stage order, and shutdown in reverse stage order.
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
```

## Real world example

This example starts an HTTP and a (simulated) messaging listeners which are the core function of the service.
The service will have telemetry, a health HTTP server listening in a different port, and will follow the Kubernetes
pattern of having startup, liveness and readiness probes with the correct states during the initialization.

There is step-by-step description of the complete process after the source code.

```go
// TODO
```

- Start `management` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `telemetry`
    - `health service`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `health service`
    - `Timeout 100ms` - (waits 100ms and exits, a debugging tool)
    - `Signals [interrupt interrupt terminated]` - (waits until an OS signal is received)
- Start `initialize` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - opens the DB connection.
- Start `ready` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `health server started probe` - signals the startup and readiness probe that the service is started. 
- Start `service` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
    - `Messaging service`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `HTTP service`
    - `Messaging service`
- **Wait until the `start` step of any task returns (with an error or nil)**.
- The first task `start` step to return in this example is `Timeout 100ms`, with the error `timed out`.
- Cancel the context sent to all tasks' `start` step which have the `WithCancelContext(true)` option set, 
  using this `timed out` error that was returned (in this example, only `Timeout 100ms` 
  and `Signals [interrupt interrupt terminated]`).
- A context based on the root context (NOT the one sent to the tasks, that was just cancelled) with a deadline of
  20 seconds, is created and will be sent to all stopping jobs.
- Stop `service` stage:
  - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
    - `Messaging service`
    - `telemetry flush` - flushes the pending telemetry to avoid losing it in case the service is killed.
    - `health server terminating probe` - signals the readiness probe that the service is terminating.
- Stop `management` stage:
    - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
        - `health service`
- **Wait until the `start` step of ALL tasks return, or for the shutdown timeout.**
- Teardown `initialize` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - closes the DB connection.
- Teardown `management` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `telemetry`

- The `Run` method will return the error `timed out`.

## Author

Rangel Reale (rangelreale@gmail.com)
