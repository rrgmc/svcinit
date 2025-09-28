package main

import (
	"context"
	"database/sql"
	"errors"
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

	//
	// debug step: sleep 100ms and shutdown.
	//
	sinit.AddTask(StageManagement, svcinit.TimeoutTask(100*time.Millisecond,
		svcinit.WithTimeoutTaskError(errors.New("timed out"))))

	//
	// start execution
	//
	return sinit.Run(ctx)
}
