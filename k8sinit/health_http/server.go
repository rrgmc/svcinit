package health_http

import (
	"context"
	"net"
	"net/http"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/k8sinit"
)

type Server struct {
	server             *http.Server
	handlerOptions     []HandlerOption
	handler            *Handler
	address            string
	httpServerProvider func(ctx context.Context, address string) (*http.Server, error)
	taskName           string
}

func NewServer(options ...ServerOption) *Server {
	ret := &Server{
		address:  ":6060",
		taskName: "health handler",
	}
	for _, option := range options {
		option.applyServerOption(ret)
	}
	ret.handler = NewHandler(ret.handlerOptions...)
	return ret
}

var _ svcinit.Task = (*Server)(nil)
var _ svcinit.TaskName = (*Server)(nil)
var _ k8sinit.HealthHandler = (*Server)(nil)

func (h *Server) ServiceStarted() {
	h.handler.ServiceStarted()
}

func (h *Server) ServiceTerminating() {
	h.handler.ServiceTerminating()
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
				Addr: ":6060",
			}
		}
		mux := http.NewServeMux()
		h.handler.Register(mux)
		h.server.Handler = mux
	case svcinit.StepStart:
		h.server.BaseContext = func(net.Listener) context.Context {
			return ctx
		}
		return h.server.ListenAndServe()
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

func WithServerAddress(address string) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.address = address
		},
	}
}

func WithServerProvider(provider func(ctx context.Context, address string) (*http.Server, error)) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.httpServerProvider = provider
		},
	}
}

func WithServerTaskName(name string) ServerOption {
	return &optionImpl{
		serverOpt: func(server *Server) {
			server.taskName = name
		},
	}
}
