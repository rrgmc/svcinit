package k8sinit

import (
	"context"
	"net/http"
)

type HealthService interface {
	Handler() http.Handler
	ServiceStarted()
	ServiceTerminating()
}

type HealthMode int

const (
	HealthModeNone HealthMode = iota
	HealthModeHTTPServer
	HealthModeHTTPHandler
)

// options

func WithHealthMode(mode HealthMode) Option {
	return func(o *Manager) {
		o.healthOptions.mode = mode
	}
}

func WithHealthHTTPAddress(httpAddress string) Option {
	return func(o *Manager) {
		o.healthOptions.httpAddress = httpAddress
	}
}

func WithHealthHTTPServerProvider(provider func(ctx context.Context, address string) (*http.Server, error)) Option {
	return func(o *Manager) {
		o.healthOptions.httpServerProvider = provider
	}
}

func WithHealthStartupProbePath(path string) Option {
	return func(o *Manager) {
		o.healthOptions.startupProbePath = path
	}
}

func WithHealthLivenessProbePath(path string) Option {
	return func(o *Manager) {
		o.healthOptions.livenessProbePath = path
	}
}

func WithHealthReadinessProbePath(path string) Option {
	return func(o *Manager) {
		o.healthOptions.readinessProbePath = path
	}
}

type healthOptions struct {
	mode               HealthMode
	httpAddress        string
	httpServerProvider func(ctx context.Context, address string) (*http.Server, error)
	startupProbePath   string // /startup
	livenessProbePath  string // /healthz
	readinessProbePath string // /ready
}
