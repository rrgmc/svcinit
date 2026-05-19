package main

import (
	"context"
	"database/sql"
	"net"
	"net/http"
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
