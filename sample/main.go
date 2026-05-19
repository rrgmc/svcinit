package main

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
