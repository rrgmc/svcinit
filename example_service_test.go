package svcinit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
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
// Simulates a messaging service receiving and forwarding messages.
//

type MessagingService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

func Example() {
	ctx := context.Background()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sinit, err := svcinit.New(
		svcinit.WithStages(allStages...),
		svcinit.WithShutdownTimeout(20*time.Second),
		svcinit.WithEnforceShutdownTimeout(true),
	)
	if err != nil {
		fmt.Println(err)
		return
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
			// ret.db, err = sql.Open("pgx", "dburl")
			ret.db = &sql.DB{}
			if err != nil {
				return nil, err
			}

			// send the initialized DB connection to the health service to be used by the readiness probe.
			healthServer.AddDBHealth(ret.db)

			return
		},
		svcinit.WithDataTeardown(func(ctx context.Context, data *initTaskData) error {
			// return data.db.Close()
			return nil
		}),
		svcinit.WithDataName[*initTaskData]("initialization"),
	)
	sinit.AddTask(StageInitialize, initTask)

	//
	// initialize and start the HTTP service.
	//
	sinit.AddTask(StageService, svcinit.BuildDataTask[svcinit.Task](
		func(ctx context.Context) (svcinit.Task, error) {
			// using WithDataParentFromSetup, returning a [svcinit.Task] from this "setup" step sets it as the parent
			// task, and all of its steps are added to this one.
			// This makes Start and Stop be called automatically.
			initData, err := initTask.Value()
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
			initData, err := initTask.Value()
			if err != nil {
				return nil, err
			}
			return NewMessagingServiceImpl(logger, initData.db), nil
		},
		svcinit.WithDataStart(func(ctx context.Context, data MessagingService) error {
			return data.Start(ctx)
		}),
		svcinit.WithDataStop(func(ctx context.Context, data MessagingService) error {
			return data.Stop(ctx)
		}),
		svcinit.WithDataName[MessagingService]("Messaging service"),
	))

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

	err = sinit.Run(ctx)
	if err != nil {
		fmt.Println(err)
	}

	// Output: timed out
}

//
// Health webservice
//

type HealthServiceImpl struct {
	server *http.Server
}

func NewHealthServiceImpl() *HealthServiceImpl {
	return &HealthServiceImpl{
		server: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			Addr: ":6060",
		},
	}
}

func (s *HealthServiceImpl) Start(ctx context.Context) error {
	s.server.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	return s.server.ListenAndServe()
}

func (s *HealthServiceImpl) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HealthServiceImpl) AddDBHealth(db *sql.DB) {
	// add the DB connection to be checked in the readiness probe...
}

func (s *HealthServiceImpl) ServiceStarted() {
	// signal the startup / readiness probe that the service is ready...
}

func (s *HealthServiceImpl) ServiceTerminating() {
	// signal the readiness probe that the service is terminating and not ready...
}

//
// HTTP webservice
//

type HTTPServiceImpl struct {
	server *http.Server
	db     *sql.DB
}

func NewHTTPServiceImpl(db *sql.DB) *HTTPServiceImpl {
	return &HTTPServiceImpl{
		server: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			Addr: ":8080",
		},
		db: db,
	}
}

func (s *HTTPServiceImpl) Start(ctx context.Context) error {
	s.server.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	return s.server.ListenAndServe()
}

func (s *HTTPServiceImpl) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

//
// Messaging service
//
// Simulates a messaging service receiving and forwarding messages.
//

type MessagingServiceImpl struct {
	logger    *slog.Logger
	server    net.Listener
	db        *sql.DB
	isStopped atomic.Bool
}

func NewMessagingServiceImpl(logger *slog.Logger, db *sql.DB) *MessagingServiceImpl {
	return &MessagingServiceImpl{
		logger: logger,
		db:     db,
	}
}

func (s *MessagingServiceImpl) Start(ctx context.Context) error {
	var err error
	s.server, err = net.Listen("tcp", ":9900")
	if err != nil {
		return err
	}

	for {
		conn, err := s.server.Accept()
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to accept connection", "error", err)
			if s.isStopped.Load() {
				return err
			}
			continue
		}
		go func(c net.Conn) {
			_ = c.Close()
		}(conn)
	}
}

func (s *MessagingServiceImpl) Stop(ctx context.Context) error {
	s.isStopped.Store(true)
	return s.server.Close()
}
