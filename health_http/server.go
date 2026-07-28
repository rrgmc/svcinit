package health_http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/rrgmc/svcinit/v3"
)

type Server struct {
	server             *http.Server
	handlerOptions     []HandlerOption
	handler            *Handler
	address            string
	httpServerProvider func(ctx context.Context, address string) (*http.Server, error)
	taskName           string
}

// NewServer creates a separate http server for using as a health handler.
// It implements [svcinit.HealthHandler] and [svcinit.Task], so besides being used as a [svcinit.HealthHandler],
// it must also be started as a [svcinit.Task].
func NewServer(options ...ServerOption) *Server {
	ret := &Server{
		address:  ":6060",
		taskName: "health",
	}
	for _, option := range options {
		option.applyServerOption(ret)
	}
	ret.handler = NewHandler(ret.handlerOptions...)
	return ret
}

var _ svcinit.Task = (*Server)(nil)
var _ svcinit.TaskName = (*Server)(nil)
var _ svcinit.HealthHandler = (*Server)(nil)

func (h *Server) ServiceStarted(ctx context.Context) {
	h.handler.ServiceStarted(ctx)
}

func (h *Server) ServiceTerminating(ctx context.Context) {
	h.handler.ServiceTerminating(ctx)
}

func (h *Server) Run(ctx context.Context, step svcinit.Step) (err error) {
	switch step {
	case svcinit.StepSetup:
		if h.httpServerProvider != nil {
			h.server, err = h.httpServerProvider(ctx, h.address)
			if err != nil {
				return err
			}
		} else {
			h.server = &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				Addr:              h.address,
				ReadHeaderTimeout: 5 * time.Second,
			}
		}
		mux := http.NewServeMux()
		h.handler.Register(mux)
		h.server.Handler = mux
	case svcinit.StepStart:
		h.server.BaseContext = func(net.Listener) context.Context {
			return ctx
		}
		if err := h.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case svcinit.StepStop:
		return h.server.Shutdown(ctx)
	default:
	}
	return nil
}

func (h *Server) TaskName() string {
	return h.taskName
}

// options

// WithServerAddress sets the address the server will listen on. The default is ":6060".
// Ignored if [WithServerProvider] is used: the provider is then responsible for the server's address.
func WithServerAddress(address string) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.address = address
		},
	}
}

// WithServerProvider sets a custom function to create the underlying [http.Server]. Any Handler already
// set on the returned server is discarded: [Server] always installs its own mux with the health probe
// routes as the server's Handler.
func WithServerProvider(provider func(ctx context.Context, address string) (*http.Server, error)) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.httpServerProvider = provider
		},
	}
}

// WithServerTaskName sets the [svcinit.TaskName] returned for this task. The default is "health".
func WithServerTaskName(name string) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.taskName = name
		},
	}
}
