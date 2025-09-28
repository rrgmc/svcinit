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
	httpOptions        healthHTTPOptions
}

func NewHealthHTTPServer(options ...HealthHTTPServerOption) *HealthHTTPServer {
	ret := &HealthHTTPServer{
		address:     ":6060",
		taskName:    "health handler",
		httpOptions: newHealthHTTPOptions(),
	}
	for _, option := range options {
		option.applyHealthHTTPServerOption(ret)
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
		mux.Handle("GET "+h.httpOptions.startupProbePath, h.handler.StartupHandler)
		mux.Handle("GET "+h.httpOptions.livenessProbePath, h.handler.LivenessHandler)
		mux.Handle("GET "+h.httpOptions.readinessProbePath, h.handler.ReadinessHandler)
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

func WithHealthHTTPServerAddress(address string) HealthHTTPServerOption {
	return &healthHTTPServerOption{
		httpOption: nil,
		serverOption: func(o *HealthHTTPServer) {
			o.address = address
		},
	}
}

func WithHealthHTTPServerProvider(provider func(ctx context.Context, address string) (*http.Server, error)) HealthHTTPServerOption {
	return &healthHTTPServerOption{
		httpOption: nil,
		serverOption: func(o *HealthHTTPServer) {
			o.httpServerProvider = provider
		},
	}
}

func WithHealthHTTPServerTaskName(name string) HealthHTTPServerOption {
	return &healthHTTPServerOption{
		httpOption: nil,
		serverOption: func(o *HealthHTTPServer) {
			o.taskName = name
		},
	}
}

func WithHealthHTTPServerHandlerOptions(options ...HealthHTTPHandlerOption) HealthHTTPServerOption {
	return &healthHTTPServerOption{
		httpOption: nil,
		serverOption: func(o *HealthHTTPServer) {
			o.handlerOptions = append(o.handlerOptions, options...)
		},
	}
}

type HealthHTTPServerOption interface {
	applyHealthHTTPServerOption(*HealthHTTPServer)
}

// internal

type healthHTTPServerOption struct {
	httpOption   func(o *healthHTTPOptions)
	serverOption func(o *HealthHTTPServer)
}

func (h *healthHTTPServerOption) applyHealthHTTPOption(options *healthHTTPOptions) {
	if h.httpOption != nil {
		h.httpOption(options)
	}
}

func (h *healthHTTPServerOption) applyHealthHTTPServerOption(server *HealthHTTPServer) {
	if h.serverOption != nil {
		h.serverOption(server)
	}
}

type healthHTTPOptions struct {
	startupProbePath   string // /startup
	livenessProbePath  string // /healthz
	readinessProbePath string // /ready
}

func newHealthHTTPOptions() healthHTTPOptions {
	return healthHTTPOptions{
		startupProbePath:   "/startup",
		livenessProbePath:  "/healthz",
		readinessProbePath: "/ready",
	}
}
