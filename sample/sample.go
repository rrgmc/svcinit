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
	AddDBHealth(db *sql.DB) // add the DB connection to be checked in the readiness probe...
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
		svcinit.WithStages(allStages...),
		svcinit.WithShutdownTimeout(20*time.Second),
		svcinit.WithEnforceShutdownTimeout(true),
		svcinit.WithLogger(logger),
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
			// OpenTelemetry initialization...
			return nil
		}),
		svcinit.WithTeardown(func(ctx context.Context) error {
			// OpenTelemetry closing/flushing...
			return nil
		}),
		svcinit.WithName("telemetry"),
	))

	// flush the metrics as fast as possible on SIGTERM.
	sinit.AddTask(StageService, svcinit.BuildTask(
		svcinit.WithStop(func(ctx context.Context) error {
			// flush the current metrics as fast a possible.
			// We may not have enough time if the shutdown takes too long.
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
		svcinit.WithDataStart(func(ctx context.Context, data HealthService) error {
			return data.Start(ctx)
		}),
		svcinit.WithDataStop(func(ctx context.Context, data HealthService) error {
			return data.Stop(ctx)
		}),
		svcinit.WithDataName[HealthService]("health service"),
	)
	sinit.AddTask(StageManagement, healthTask)

	sinit.AddTask(StageReady, svcinit.BuildTask(
		svcinit.WithSetup(func(ctx context.Context) error {
			healthServer, err := healthTask.Value()
			if err != nil {
				return fmt.Errorf("error getting health server: %w", err)
			}
			logger.DebugContext(ctx, "service started, signaling probes")
			healthServer.ServiceStarted()
			return nil
		}),
		svcinit.WithName("health server started probe"),
	))

	sinit.AddTask(StageService, svcinit.BuildTask(
		svcinit.WithStop(func(ctx context.Context) error {
			healthServer, err := healthTask.Value()
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
	//
	type initTaskData struct {
		db *sql.DB
	}
	initTask := svcinit.NewTaskFuture[*initTaskData](
		func(ctx context.Context) (ret *initTaskData, err error) {
			// get the health server from the Future.
			healthServer, err := healthTask.Value()
			if err != nil {
				return nil, err
			}

			ret = &initTaskData{}

			logger.InfoContext(ctx, "connecting to database")
			// ret.db, err = sql.Open("pgx", "dburl")
			ret.db = &sql.DB{}
			if err != nil {
				return nil, err
			}

			// send the initialized DB connection to the health service to be used by the readiness probe.
			healthServer.AddDBHealth(ret.db)

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

			if !ssm.CanFinished() {
				// if we can't wait for any reason, just return.
				return nil
			}
			select {
			case <-ctx.Done():
			case <-ssm.Finished(): // will be signaled when the "start" step of this task ends.
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
