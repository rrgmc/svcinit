# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit/v3?status.png)](https://godoc.org/github.com/rrgmc/svcinit/v3)

`svcinit` is an initialization system for Go services.

It manages starting and stopping tasks (like a web server), initialization order, correct context handling, free of
race conditions and goroutine safe.

It is NOT some kind of dependency injection or application framework like Uber's FX, it could be seen like a more
advanced version of [github.com/oklog/run](https://github.com/oklog/run).

The library makes it easy to follow common service initialization patterns, like making sure things start in a
defined order, correctly doing startup, liveness and readiness probes, context cancellation where the shutdown context 
is not the same as the startup one (otherwise shutdown tasks would also be cancelled), using resolvable futures to 
provide data to dependent tasks, and more.

## Install

```shell
go get github.com/rrgmc/svcinit/v3
```

## Table of Contents

- [Features](#features)
- [Task type](#task-type)
- [Example](#example)
- [Real-world example](#real-world-example)
- [Real-world example - Kubernetes](#real-world-example---kubernetes)

## Features

- stages for managing start/stop ordering. The next stage is only initialized once the previous one was fully started.
- `start`, `stop`, `setup` and `teardown` task steps.
- start tasks can stop with or without context cancellation.
- `setup` and `teardown` steps to perform task initialization and finalization. Initialization is done in a goroutine,
  so a health service can correctly manage a startup probe.
- keeps track of all steps executed, so each step is guaranteed to be called at most once, and any initialization error
  just calls the stopping steps of what was effectively started.
- ensures no race condition, like tasks finishing before all initialization was done.
- "futures" to manage task dependencies.
- possibility of the `stop` step directly managing it's `start` step, like canceling its context and waiting for its
  completion.
- callbacks for all events that happens during execution. 
- the application execution error result will be the error returned by the first task `start` step to finish.
- specific implementation using Kubernetes initialization patterns.

## Task type

```go
type Step int

const (
    StepSetup Step = iota
    StepStart
    StepStop
    StepTeardown
)

type Task interface {
    Run(ctx context.Context, step Step) error
}
```

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

This example starts an HTTP server and a (simulated) messaging listener which are the core function of the service.
The service will have telemetry, a health HTTP server listening in a different port, and will follow the Kubernetes
pattern of having startup, liveness and readiness probes with the correct states during the initialization.

Full source code in the [sample](sample/) folder.

There is step-by-step description of the complete process after the source code.

```go
import (
    "context"
    "database/sql"
    "fmt"
    "os"
    "syscall"
    "time"

    "github.com/rrgmc/svcinit/v3"
)

const (
    StageManagement = "management" // 1st stage: initialize telemetry, health server and signal handling
    StageInitialize = "initialize" // 2nd stage: initialize data, like DB connections
    StageReady      = "ready"      // 3rd stage: signals probes that the service has completely started
    StageService    = "service"    // 4th state: initialize services
)

var allStages = []string{StageManagement, StageInitialize, StageReady, StageService}

//
// Health webservice
//

type HealthService interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ServiceStarted()        // signal the startup / readiness probe that the service is ready
    ServiceTerminating()    // signal the readiness probe that the service is terminating and not ready
    AddDBHealth(db *sql.DB) // add the DB connection to be checked in the readiness probe
}

//
// HTTP webservice
//

type HTTPService interface {
    svcinit.Service // has "Start(ctx) error" and "Stop(ctx) error" methods.
}

//
// Messaging service
//
// Simulates a messaging service receiving and processing messages.
// This specific sample uses a TCP listener for the simulation.
//

type MessagingService interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

func main() {
    ctx := context.Background()
    if err := run(ctx); err != nil {
        fmt.Println(err)
    }
}

func run(ctx context.Context) error {
    logger := defaultLogger(os.Stdout)

    sinit, err := svcinit.New(
        svcinit.WithLogger(logger),
        // initialization in 4 stages. Initialization is done in stage order, and shutdown in reverse stage order.
        // all tasks added to the same stage are started/stopped in parallel.
        svcinit.WithStages(allStages...),
        // use a context with a 20-second cancellation during shutdown.
        svcinit.WithShutdownTimeout(20*time.Second),
        // some tasks may not check context cancellation, set enforce to true to give up waiting after the shutdown timeout.
        // The default is true.
        svcinit.WithEnforceShutdownTimeout(true),
    )
    if err != nil {
        return err
    }

    //
    // OpenTelemetry
    //

    // initialize and close OpenTelemetry.
    sinit.AddTask(StageManagement, svcinit.BuildTask(
        svcinit.WithSetup(func(ctx context.Context) error {
            // TODO: OpenTelemetry initialization
            return nil
        }),
        svcinit.WithTeardown(func(ctx context.Context) error {
            // TODO: OpenTelemetry closing/flushing
            return nil
        }),
        svcinit.WithName("telemetry"),
    ))

    // flush the metrics as fast as possible on SIGTERM.
    sinit.AddTask(StageService, svcinit.BuildTask(
        svcinit.WithStop(func(ctx context.Context) error {
            // TODO: flush the current metrics as fast a possible.
            // We may not have enough time if the shutdown takes too long, so do it as early as possible.
            return nil
        }),
        svcinit.WithName("telemetry flush"),
    ))

    //
    // Health service
    //

    // health server must be the first to start and last to stop.
    // created as a future task so it can be accessed by other tasks.
    // other tasks can wait for it to become available.
    healthTask := svcinit.NewTaskFuture[HealthService](
        func(ctx context.Context) (HealthService, error) {
            return NewHealthServiceImpl(), nil
        },
        svcinit.WithDataStart(func(ctx context.Context, service HealthService) error {
            return service.Start(ctx)
        }),
        svcinit.WithDataStop(func(ctx context.Context, service HealthService) error {
            return service.Stop(ctx)
        }),
        svcinit.WithDataName[HealthService]("health service"),
    )
    sinit.AddTask(StageManagement, healthTask)

    // the "ready" stage is executed after all initialization already happened. It is used to signal the
    // startup probes that the service is ready.
    sinit.AddTask(StageReady, svcinit.BuildTask(
        svcinit.WithSetup(func(ctx context.Context) error {
            healthServer, err := healthTask.Value() // get health server from future
            if err != nil {
                return fmt.Errorf("error getting health server: %w", err)
            }
            logger.DebugContext(ctx, "service started, signaling probes")
            healthServer.ServiceStarted()
            return nil
        }),
        svcinit.WithName("health server started probe"),
    ))

    // add a task in the "service" stage, so the stop step is called in parallel with the service stopping ones.
    // This tasks signals the probes that the service is terminating.
    sinit.AddTask(StageService, svcinit.BuildTask(
        svcinit.WithStop(func(ctx context.Context) error {
            healthServer, err := healthTask.Value() // get health server from future
            if err != nil {
                return fmt.Errorf("error getting health server: %s", err)
            }
            logger.DebugContext(ctx, "service terminating, signaling probes")
            healthServer.ServiceTerminating()
            return nil
        }),
        svcinit.WithName("health server terminating probe"),
    ))

    //
    // initialize data to be used by the service, like database and cache connections.
    // A TaskFuture is a Task and a Future at the same time, where the task resolves the future.
    // Following tasks can wait on this future to get the initialized data.
    //
    type initTaskData struct {
        db *sql.DB
    }
    initTask := svcinit.NewTaskFuture[*initTaskData](
        func(ctx context.Context) (data *initTaskData, err error) {
            data = &initTaskData{}

            logger.InfoContext(ctx, "connecting to database")
            // ret.db, err = sql.Open("pgx", "dburl")
            data.db = &sql.DB{}
            if err != nil {
                return nil, err
            }

            // send the initialized DB connection to the health service to be used by the readiness probe.
            healthServer, err := healthTask.Value() // get the health server from the Future.
            if err != nil {
                return nil, err
            }
            healthServer.AddDBHealth(data.db)

            logger.InfoContext(ctx, "data initialization finished")
            return
        },
        svcinit.WithDataTeardown(func(ctx context.Context, data *initTaskData) error {
            logger.InfoContext(ctx, "closing database connection")
            // return data.db.Close()
            return nil
        }),
        svcinit.WithDataName[*initTaskData]("init data"),
    )
    sinit.AddTask(StageInitialize, initTask)

    //
    // initialize and start the HTTP service.
    //
    sinit.AddTask(StageService, svcinit.BuildDataTask[svcinit.Task](
        func(ctx context.Context) (svcinit.Task, error) {
            // using the WithDataParentFromSetup parameter, returning a [svcinit.Task] from this "setup" step
            // sets it as the parent task, and all of its steps are added to this one.
            // This makes Start and Stop be called automatically.
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return svcinit.ServiceAsTask(NewHTTPServiceImpl(initData.db)), nil
        },
        svcinit.WithDataParentFromSetup[svcinit.Task](true),
        svcinit.WithDataName[svcinit.Task]("HTTP service"),
    ))

    //
    // initialize and start the messaging service.
    //
    sinit.AddTask(StageService, svcinit.BuildDataTask[MessagingService](
        func(ctx context.Context) (MessagingService, error) {
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return NewMessagingServiceImpl(logger, initData.db), nil
        },
        svcinit.WithDataStart(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            return service.Start(ctx)
        }),
        svcinit.WithDataStop(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            err := service.Stop(ctx)
            if err != nil {
                return err
            }

            // the stop method of the TCP listener do not wait until the connection is shutdown to return.
            // Using the [svcinit.WithStartStepManager] task option, we have access to an interface from the context
            // that we can use to cancel the "start" step context and/or wait for its completion.
            ssm := svcinit.StartStepManagerFromContext(ctx)

            // we could also cancel the context of the "start" step manually. As the Go TCP listener don't have
            // context cancellation, it wouldn't do anything in this case.
            // Note that the [svcinit.StartStepManager] context cancellation is not the same as the main/root context
            // cancellation, this is a context exclusive for this interaction.
            // // ssm.ContextCancel(context.Canceled)

            select {
            case <-ctx.Done():
            case <-ssm.Finished():
                // will be signaled when the "start" step of this task ends.
                // "ssm.FinishedErr()" will contain the error that was returned from it.
            }
            return nil
        }),
        svcinit.WithDataName[MessagingService]("Messaging service"),
    ), svcinit.WithStartStepManager())

    //
    // Signal handling
    //
    sinit.AddTask(StageManagement, svcinit.SignalTask(os.Interrupt, syscall.SIGINT, syscall.SIGTERM))

    // //
    // // debug step: sleep 100ms and shutdown.
    // //
    // sinit.AddTask(StageManagement, svcinit.TimeoutTask(100*time.Millisecond,
    // 	svcinit.WithTimeoutTaskError(errors.New("timed out"))))

    //
    // start execution
    //
    return sinit.Run(ctx)
}
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


## Real world example - Kubernetes

The `github.com/rrgmc/svcinit/v3/k8sinit` package contains a Kubernetes service initialization pattern, which is the
same as the above real world example.

Full example source code in the [k8sinit/sample](k8sinit/sample/) folder.

Here is the implementation of the same service above using this package:

```go
import (
    "context"
    "database/sql"
    "fmt"
    "os"

    "github.com/rrgmc/svcinit/v3"
    "github.com/rrgmc/svcinit/v3/health_http"
    "github.com/rrgmc/svcinit/v3/k8sinit"
)

//
// Health webservice
//

type HealthService interface {
    AddDBHealth(db *sql.DB) // add the DB connection to be checked in the readiness probe
}

//
// HTTP webservice
//

type HTTPService interface {
    svcinit.Service // has "Start(ctx) error" and "Stop(ctx) error" methods.
}

//
// Messaging service
//
// Simulates a messaging service receiving and processing messages.
// This specific sample uses a TCP listener for the simulation.
//

type MessagingService interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

func main() {
    ctx := context.Background()
    if err := run(ctx); err != nil {
        fmt.Println(err)
    }
}

func run(ctx context.Context) error {
    logger := defaultLogger(os.Stdout)

    // healthService is created in advance because it supports setting a DB instance for the readiness probe to use.
    // Otherwise, [health_http.WithProbeHandler] would not need to be added, a default implementation would be used.
    healthService := NewHealthServiceImpl()

    sinit, err := k8sinit.New(
        k8sinit.WithLogger(defaultLogger(os.Stdout)),
        k8sinit.WithHealthHandlerTask(health_http.NewServer(
            health_http.WithProbeHandler(healthService),
        )),
    )
    if err != nil {
        return err
    }

    //
    // initialize data to be used by the service, like database and cache connections.
    // A TaskFuture is a Task and a Future at the same time, where the task resolves the future.
    // Following tasks can wait on this future to get the initialized data.
    //
    type initTaskData struct {
        db *sql.DB
    }
    initTask := svcinit.NewTaskFuture[*initTaskData](
        func(ctx context.Context) (data *initTaskData, err error) {
            data = &initTaskData{}

            logger.InfoContext(ctx, "connecting to database")
            // ret.db, err = sql.Open("pgx", "dburl")
            data.db = &sql.DB{}
            if err != nil {
                return nil, err
            }

            // send the initialized DB connection to the health service to be used by the readiness probe.
            healthService.AddDBHealth(data.db)

            logger.InfoContext(ctx, "data initialization finished")
            return
        },
        svcinit.WithDataTeardown(func(ctx context.Context, data *initTaskData) error {
            logger.InfoContext(ctx, "closing database connection")
            // return data.db.Close()
            return nil
        }),
        svcinit.WithDataName[*initTaskData]("init data"),
    )
    sinit.AddTask(k8sinit.StageInitialize, initTask)

    //
    // initialize and start the HTTP service.
    //
    sinit.AddTask(k8sinit.StageService, svcinit.BuildDataTask[svcinit.Task](
        func(ctx context.Context) (svcinit.Task, error) {
            // using the WithDataParentFromSetup parameter, returning a [svcinit.Task] from this "setup" step
            // sets it as the parent task, and all of its steps are added to this one.
            // This makes Start and Stop be called automatically.
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return svcinit.ServiceAsTask(NewHTTPServiceImpl(initData.db)), nil
        },
        svcinit.WithDataParentFromSetup[svcinit.Task](true),
        svcinit.WithDataName[svcinit.Task]("HTTP service"),
    ))

    //
    // initialize and start the messaging service.
    //
    sinit.AddTask(k8sinit.StageService, svcinit.BuildDataTask[MessagingService](
        func(ctx context.Context) (MessagingService, error) {
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return NewMessagingServiceImpl(logger, initData.db), nil
        },
        svcinit.WithDataStart(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            return service.Start(ctx)
        }),
        svcinit.WithDataStop(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            err := service.Stop(ctx)
            if err != nil {
                return err
            }

            // the stop method of the TCP listener do not wait until the connection is shutdown to return.
            // Using the [svcinit.WithStartStepManager] task option, we have access to an interface from the context
            // that we can use to cancel the "start" step context and/or wait for its completion.
            ssm := svcinit.StartStepManagerFromContext(ctx)

            // we could also cancel the context of the "start" step manually. As the Go TCP listener don't have
            // context cancellation, it wouldn't do anything in this case.
            // Note that the [svcinit.StartStepManager] context cancellation is not the same as the main/root context
            // cancellation, this is a context exclusive for this interaction.
            // // ssm.ContextCancel(context.Canceled)

            select {
            case <-ctx.Done():
            case <-ssm.Finished():
                // will be signaled when the "start" step of this task ends.
                // "ssm.FinishedErr()" will contain the error that was returned from it.
            }
            return nil
        }),
        svcinit.WithDataName[MessagingService]("Messaging service"),
    ), svcinit.WithStartStepManager())

    // //
    // // debug step: sleep 100ms and shutdown.
    // //
    // sinit.AddTask(k8sinit.StageManagement, svcinit.TimeoutTask(100*time.Millisecond,
    // 	svcinit.WithTimeoutTaskError(errors.New("timed out"))))

    //
    // start execution
    //
    return sinit.Run(ctx)
}
```


## Author

Rangel Reale (rangelreale@gmail.com)
