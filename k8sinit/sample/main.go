package main

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

	// healthHelper is created in advance because it supports setting a DB instance for the readiness probe to use.
	// Otherwise, [health_http.WithProbeHandler] would not need to be added, a default implementation would be used.
	healthHelper := NewHealthHelperImpl()

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
	sinit.SetTelemetryHandler(NewTelemetryHandlerImpl())

	//
	// Health service
	//

	//
	// set a health handler which is also a task to be started/stopped.
	//
	sinit.SetHealthHandlerTask(health_http.NewServer(
		health_http.WithStartupProbe(true), // fails startup and readiness probes until service is started.
		health_http.WithProbeHandler(healthHelper),
		health_http.WithServerTaskName(k8sinit.TaskNameHealth),
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
			healthHelper.AddDBHealth(data.db)

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
