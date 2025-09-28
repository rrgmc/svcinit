package k8sinit

import (
	"context"
	"net"
	"net/http"

	"github.com/rrgmc/svcinit/v3"
)

type HealthHTTPServer struct {
	server             *http.Server
	handlerOptions     []HealthHTTPHandlerOption
	handler            *HealthHTTPHandler
	address            string
	httpServerProvider func(ctx context.Context, address string) (*http.Server, error)
	taskName           string
}

func NewHealthHTTPServer(options ...HealthHTTPServerOption) *HealthHTTPServer {
	ret := &HealthHTTPServer{
		address:  ":6060",
		taskName: "health handler",
	}
	for _, option := range options {
		option(ret)
	}
	ret.handler = NewHealthHTTPHandler(ret.handlerOptions...)
	return ret
}

var _ svcinit.Task = (*HealthHTTPServer)(nil)
var _ svcinit.TaskName = (*HealthHTTPServer)(nil)
var _ HealthHandler = (*HealthHTTPServer)(nil)

func (h *HealthHTTPServer) ServiceStarted() {
	h.handler.ServiceStarted()
}

func (h *HealthHTTPServer) ServiceTerminating() {
	h.handler.ServiceTerminating()
}

func (h *HealthHTTPServer) Run(ctx context.Context, step svcinit.Step) (err error) {
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
		mux.Handle("GET "+h.handler.StartupProbePath, h.handler.StartupHandler)
		mux.Handle("GET "+h.handler.LivenessProbePath, h.handler.LivenessHandler)
		mux.Handle("GET "+h.handler.ReadinessProbePath, h.handler.ReadinessHandler)
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

func (h *HealthHTTPServer) TaskName() string {
	return h.taskName
}

// options

type HealthHTTPServerOption func(*HealthHTTPServer)

func WithHealthHTTPServerAddress(address string) HealthHTTPServerOption {
	return func(h *HealthHTTPServer) {
		h.address = address
	}
}

func WithHealthHTTPServerProvider(provider func(ctx context.Context, address string) (*http.Server, error)) HealthHTTPServerOption {
	return func(h *HealthHTTPServer) {
		h.httpServerProvider = provider
	}
}

func WithHealthHTTPServerTaskName(name string) HealthHTTPServerOption {
	return func(h *HealthHTTPServer) {
		h.taskName = name
	}
}

func WithHealthHTTPServerHandlerOptions(options ...HealthHTTPHandlerOption) HealthHTTPServerOption {
	return func(h *HealthHTTPServer) {
		h.handlerOptions = options
	}
}
