# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit/v3?status.png)](https://godoc.org/github.com/rrgmc/svcinit/v3)

`svcinit` is an initialization system for Go services.

It manages starting and stopping tasks (like a web server), initialization order, correct context handling, without
race conditions and goroutine-safe.

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
- `start` steps can stop with or without context cancellation.
- `setup` and `teardown` steps to perform task initialization and finalization. Initialization is done in a goroutine,
  so for example a health service can correctly manage a startup probe.
- keeps track of all steps executed, so each step is guaranteed to be called at most once, and any initialization error
  just calls the stopping steps of what was effectively started.
- ensures no race conditions, like tasks finishing before all initialization was done.
- "futures" to manage task dependencies.
- possibility of the `stop` step directly managing it's `start` step, like canceling its context and waiting for its
  completion.
- callbacks for all events that happens during execution. 
- the application execution error result will be the error returned by the first `start` step that finishes.
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
    "github.com/rrgmc/svcinit/v3/instancetask"
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

    sinit, err := svcinit.New(
        // initialization in 2 stages. Initialization is done in stage order, and shutdown in reverse stage order.
        // all tasks added to the same stage are started/stopped in parallel.
        svcinit.WithStages("manage", "service"),
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
    sinit.AddTask("manage", instancetask.Build[*healthService](
        // the "BuildDataTask" setup callback returns an instance that is sent to all following steps.
        func(ctx context.Context) (*healthService, error) {
            return newHealthService(), nil
        },
        instancetask.WithStart(func(ctx context.Context, service *healthService) error {
            return service.Start(ctx)
        }),
        instancetask.WithStop(func(ctx context.Context, service *healthService) error {
            return service.Stop(ctx)
        }),
    ))

    // add a task to start the core HTTP server.
    sinit.AddTask("service", instancetask.Build[*http.Server](
        func(ctx context.Context) (*http.Server, error) {
            // initialize the service in the setup step.
            // as this may take some time in bigger services, initializing here allows other tasks to initialize
            // at the same time.
            server := &http.Server{
                Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    w.WriteHeader(http.StatusOK)
                }),
                Addr: ":8080",
            }
            return server, nil
        },
        instancetask.WithStart(func(ctx context.Context, service *http.Server) error {
            service.BaseContext = func(net.Listener) context.Context {
                return ctx
            }
            return service.ListenAndServe()
        }),
        // stop the service. By default, the context is NOT cancelled, this method must arrange for the start
        // function to end.
        instancetask.WithStop(func(ctx context.Context, service *http.Server) error {
            return service.Shutdown(ctx)
        }),
    ))

    // shutdown on OS signal.
    sinit.AddTask("manage", svcinit.SignalTask(os.Interrupt, syscall.SIGTERM))

    // sleep 100ms and shutdown.
    sinit.AddTask("manage", svcinit.TimeoutTask(100*time.Millisecond,
        svcinit.WithTimeoutTaskError(errors.New("timed out"))))

    err = sinit.Run(ctx)
    if err != nil {
        fmt.Println("err:", err)
    }

    // Output: err: timed out
}
```

## Real world example

This example starts an HTTP server, which is the core function of the service.
The service will have telemetry, and a health HTTP server listening in a different port.

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
    "github.com/rrgmc/svcinit/v3/futuretask"
    "github.com/rrgmc/svcinit/v3/instancetask"
)

const (
    StageManagement = "management" // 1st stage: initialize telemetry, health server and signal handling
    StageInitialize = "initialize" // 2nd stage: initialize data, like DB connections
    StageService    = "service"    // 3rd state: initialize services
)

var allStages = []string{StageManagement, StageInitialize, StageService}

//
// Health webservice
// (implements svcinit.Service)
//

type HealthService interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

//
// HTTP webservice
// (implements svcinit.Service)
//

type HTTPService interface {
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
        // initialization in 3 stages. Initialization is done in stage order, and shutdown in reverse stage order.
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
    sinit.AddTask(StageManagement, instancetask.Build[HealthService](
        func(ctx context.Context) (HealthService, error) {
            return NewHealthServiceImpl(), nil
        },
        instancetask.WithStart(func(ctx context.Context, service HealthService) error {
            return service.Start(ctx)
        }),
        instancetask.WithStop(func(ctx context.Context, service HealthService) error {
            return service.Stop(ctx)
        }),
        instancetask.WithName[HealthService]("health service"),
    ))

    //
    // initialize data to be used by the service, like database and cache connections.
    // A TaskFuture is a Task and a Future at the same time, where the task resolves the future.
    // Following tasks can wait on this future to get the initialized data.
    //
    type initTaskData struct {
        db *sql.DB
    }
    initTask := svcinit.ManagerInitAddTask(sinit, StageInitialize, func() svcinit.TaskFuture[*initTaskData] {
        return futuretask.New[*initTaskData](
            func(ctx context.Context) (data *initTaskData, err error) {
                data = &initTaskData{}

                logger.InfoContext(ctx, "connecting to database")
                // ret.db, err = sql.Open("pgx", "dburl")
                data.db = &sql.DB{}
                if err != nil {
                    return nil, err
                }

                logger.InfoContext(ctx, "data initialization finished")
                return
            },
            instancetask.WithTeardown(func(ctx context.Context, data *initTaskData) error {
                logger.InfoContext(ctx, "closing database connection")
                // return data.db.Close()
                return nil
            }),
            instancetask.WithName[*initTaskData]("init data"),
        )
    })

    //
    // initialize and start the HTTP service.
    //
    sinit.AddTask(StageService, instancetask.Provider(
        func(ctx context.Context) (svcinit.Task, error) {
            // get the init value from the future declared above.
            initData, err := initTask.Value()
            if err != nil {
                return nil, err
            }
            // Provide the task to be executed.
            // svcinit.ServiceAsTask wraps a svcinit.Service (Start() and Stop()) into an svcinit.Task.
            return svcinit.ServiceAsTask(NewHTTPServiceImpl(initData.db)), nil
        },
        instancetask.WithName[svcinit.Task]("HTTP service"),
    ))

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
    - `health`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `health`
    - `timeout` - (waits 100ms and exits, a debugging tool)
    - `signals` - (waits until an OS signal is received)
- Start `initialize` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - opens the DB connection.
- Start `service` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `HTTP service`
- **Wait until the `start` step of any task returns (with an error or nil)**.
- The first `start` step to return in this example will be `timeout`, with the error `timed out`.
- Cancel the context sent to all `start` steps which have the `WithCancelContext(true)` option set,
  using this `timed out` error that was returned (in this example, only `timeout` and `signals`).
- A context based on the root context (NOT the one sent to the tasks, that was just canceled) with a deadline of
  20 seconds, is created and will be sent to all `stop` and `teardown` steps.
- Stop `service` stage:
  - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
    - `telemetry: flush` - flushes the pending telemetry to avoid losing it in case the service is killed.
- Stop `management` stage:
  - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
    - `health service`
- **Wait until the `start` step of ALL tasks return, or the shutdown deadline ends.**
- Teardown `initialize` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - closes the DB connection.
- Teardown `management` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `telemetry`
- The `Run` method will return the error `timed out`.

## Complex Real world example

This example starts an HTTP server and a (simulated) messaging listener which are the core function of the service.
The service will have telemetry, a health HTTP server listening in a different port, and will follow the Kubernetes
pattern of having startup, liveness and readiness probes with the correct states at all times.

Full source code in the [samplecomplex](sample/samplecomplex) folder.

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
    "github.com/rrgmc/svcinit/v3/futuretask"
    "github.com/rrgmc/svcinit/v3/instancetask"
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
    healthTask := futuretask.New[HealthService](
        func(ctx context.Context) (HealthService, error) {
            return NewHealthServiceImpl(), nil
        },
        instancetask.WithStart(func(ctx context.Context, service HealthService) error {
            return service.Start(ctx)
        }),
        instancetask.WithStop(func(ctx context.Context, service HealthService) error {
            return service.Stop(ctx)
        }),
        instancetask.WithName[HealthService]("health service"),
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
    initTask := svcinit.ManagerInitAddTask(sinit, StageInitialize, func() svcinit.TaskFuture[*initTaskData] {
        return futuretask.New[*initTaskData](
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
            instancetask.WithTeardown(func(ctx context.Context, data *initTaskData) error {
                logger.InfoContext(ctx, "closing database connection")
                // return data.db.Close()
                return nil
            }),
            instancetask.WithName[*initTaskData]("init data"),
        )
    })

    //
    // initialize and start the HTTP service.
    //
    sinit.AddTask(StageService, instancetask.Provider(
        func(ctx context.Context) (svcinit.Task, error) {
            // get the init value from the future declared above.
            initData, err := initTask.Value()
            if err != nil {
                return nil, err
            }
            // Provide the task to be executed.
            return svcinit.ServiceAsTask(NewHTTPServiceImpl(initData.db)), nil
        },
        instancetask.WithName[svcinit.Task]("HTTP service"),
    ))

    //
    // initialize and start the messaging service.
    //
    sinit.AddTask(StageService, instancetask.Build[MessagingService](
        func(ctx context.Context) (MessagingService, error) {
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return NewMessagingServiceImpl(logger, initData.db), nil
        },
        instancetask.WithStart(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            return service.Start(ctx)
        }),
        instancetask.WithStop(func(ctx context.Context, service MessagingService) error {
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
        instancetask.WithName[MessagingService]("Messaging service"),
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
    - `health`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `health`
    - `timeout` - (waits 100ms and exits, a debugging tool)
    - `signals` - (waits until an OS signal is received)
- Start `initialize` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - opens the DB connection.
- Start `ready` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `health: started probe` - signals the startup and readiness probe that the service is started. 
- Start `service` stage:
  - run the `setup` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
    - `Messaging service`
  - run the `start` step of these tasks in parallel but DON'T wait for their completion. They are expected to block
    until some condition makes then exit.
    - `HTTP service`
    - `Messaging service`
- **Wait until the `start` step of any task returns (with an error or nil)**.
- The first `start` step to return in this example will be `timeout`, with the error `timed out`.
- Cancel the context sent to all `start` steps which have the `WithCancelContext(true)` option set, 
  using this `timed out` error that was returned (in this example, only `timeout` and `signals`).
- A context based on the root context (NOT the one sent to the tasks, that was just cancelled) with a deadline of
  20 seconds, is created and will be sent to all `stop` and `teardown` steps.
- Stop `service` stage:
  - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
    - `HTTP service`
    - `Messaging service`
    - `telemetry: flush` - flushes the pending telemetry to avoid losing it in case the service is killed.
    - `health: terminating probe` - signals the readiness probe that the service is terminating.
- Stop `management` stage:
  - run the `stop` step of these tasks in parallel and wait for the completion of all of them:
    - `health service`
- **Wait until the `start` step of ALL tasks return, or the shutdown deadline ends.**
- Teardown `initialize` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `init data` - closes the DB connection.
- Teardown `management` stage:
  - run the `teardown` step of these tasks in parallel and wait for the completion of all of them:
    - `telemetry`
- The `Run` method will return the error `timed out`.

## Real world example - Kubernetes

The `github.com/rrgmc/svcinit/v3/k8sinit` package contains a Kubernetes service initialization pattern, which is an
abstraction of same thing the above real world example does.

Full example source code in the [k8sinit/sample](k8sinit/sample/) folder.

Here is the implementation of the same service above using this package:

```go
import (
    "context"
    "database/sql"
    "fmt"
    "os"

    "github.com/rrgmc/svcinit/v3"
    "github.com/rrgmc/svcinit/v3/futuretask"
    "github.com/rrgmc/svcinit/v3/health_http"
    "github.com/rrgmc/svcinit/v3/instancetask"
    "github.com/rrgmc/svcinit/v3/k8sinit"
)

//
// Health helper
//

type HealthHelper interface {
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

    sinit, err := k8sinit.New(
        k8sinit.WithLogger(defaultLogger(os.Stdout)),
    )
    if err != nil {
        return err
    }

    //
    // OpenTelemetry
    //

    // initialize and close OpenTelemetry.
    sinit.SetTelemetryTask(svcinit.BuildTask(
        svcinit.WithSetup(func(ctx context.Context) error {
            // TODO: OpenTelemetry initialization
            return nil
        }),
        svcinit.WithTeardown(func(ctx context.Context) error {
            // TODO: OpenTelemetry closing/flushing
            return nil
        }),
        svcinit.WithName(k8sinit.TaskNameTelemetry),
    ))
    // handle flushing metrics when service begins shutdown.
    sinit.SetTelemetryHandler(k8sinit.BuildTelemetryHandler(
        k8sinit.WithTelemetryHandlerFlushTelemetry(func(ctx context.Context) error {
            // TODO: flush metrics
            return nil
        }),
    ))

    //
    // Health service
    //

    // healthHelper is created in advance because it supports setting a DB instance for the readiness probe to use.
    // Otherwise, [health_http.WithProbeHandler] would not need to be added, a default implementation would be used.
    // It also allows customization of the probe HTTP responses.
    healthHelper := NewHealthHelperImpl()

    // set a health handler and task. [health_http.Server] supports both using the same object.
    healthTask := health_http.NewServer(
        health_http.WithStartupProbe(true), // fails startup and readiness probes until service is started.
        health_http.WithProbeHandler(healthHelper),
        health_http.WithServerTaskName(k8sinit.TaskNameHealth),
    )
    sinit.SetHealthTask(healthTask)
    sinit.SetHealthHandler(healthTask)

    //
    // initialize data to be used by the service, like database and cache connections.
    // A TaskFuture is a Task and a Future at the same time, where the task resolves the future.
    // Following tasks can wait on this future to get the initialized data.
    //
    type initTaskData struct {
        db *sql.DB
    }
    initTask := k8sinit.ManagerInitAddTask(sinit, k8sinit.StageInitialize, func() svcinit.TaskFuture[*initTaskData] {
        return futuretask.New[*initTaskData](
            func(ctx context.Context) (data *initTaskData, err error) {
                data = &initTaskData{}

                logger.InfoContext(ctx, "connecting to database")
                // ret.db, err = sql.Open("pgx", "dburl")
                data.db = &sql.DB{}
                if err != nil {
                    return nil, err
                }

                // send the initialized DB connection to the health service to be used by the readiness probe.
                healthHelper.AddDBHealth(data.db)

                logger.InfoContext(ctx, "data initialization finished")
                return
            },
            instancetask.WithTeardown(func(ctx context.Context, data *initTaskData) error {
                logger.InfoContext(ctx, "closing database connection")
                // return data.db.Close()
                return nil
            }),
            instancetask.WithName[*initTaskData]("init data"),
        )
    })

    //
    // initialize and start the HTTP service.
    //
    sinit.AddTask(k8sinit.StageService, instancetask.Provider(
        func(ctx context.Context) (svcinit.Task, error) {
            // get the init value from the future declared above.
            initData, err := initTask.Value()
            if err != nil {
                return nil, err
            }
            // Provide the task to be executed.
            return svcinit.ServiceAsTask(NewHTTPServiceImpl(initData.db)), nil
        },
        instancetask.WithName[svcinit.Task]("HTTP service"),
    ))

    //
    // initialize and start the messaging service.
    //
    sinit.AddTask(k8sinit.StageService, instancetask.Build[MessagingService](
        func(ctx context.Context) (MessagingService, error) {
            initData, err := initTask.Value() // get the init value from the future declared above.
            if err != nil {
                return nil, err
            }
            return NewMessagingServiceImpl(logger, initData.db), nil
        },
        instancetask.WithStart(func(ctx context.Context, service MessagingService) error {
            // service is the object returned from the setup step function above.
            return service.Start(ctx)
        }),
        instancetask.WithStop(func(ctx context.Context, service MessagingService) error {
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
        instancetask.WithName[MessagingService]("Messaging service"),
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
