package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
)

//
// Health webservice
//

type HealthServiceImpl struct {
	server *http.Server
}

var _ HealthService = (*HealthServiceImpl)(nil)

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

var _ HTTPService = (*HTTPServiceImpl)(nil)

func NewHTTPServiceImpl(db *sql.DB) *HTTPServiceImpl {
	mux := http.NewServeMux()
	mux.Handle("GET /test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello World, test"))
	}))
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello World"))
	}))

	return &HTTPServiceImpl{
		server: &http.Server{
			Handler: mux,
			Addr:    ":8080",
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

var _ MessagingService = (*MessagingServiceImpl)(nil)

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
			if s.isStopped.Load() {
				return nil
			}
			s.logger.ErrorContext(ctx, "failed to accept connection", "error", err)
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
