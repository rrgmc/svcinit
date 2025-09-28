package k8sinit

type HealthMode int

const (
	HealthModeNone HealthMode = iota
	HealthModeHTTPServer
	HealthModeHTTPHandler
)

// options

type HealthOptions func(*healthOptions)

func WithHealthMode(mode HealthMode) HealthOptions {
	return func(o *healthOptions) {
		o.mode = mode
	}
}

func WithHealthStartupProbePath(path string) HealthOptions {
	return func(o *healthOptions) {
		o.startupProbePath = path
	}
}

func WithHealthLivenessProbePath(path string) HealthOptions {
	return func(o *healthOptions) {
		o.livenessProbePath = path
	}
}

func WithHealthReadinessProbePath(path string) HealthOptions {
	return func(o *healthOptions) {
		o.readinessProbePath = path
	}
}

type healthOptions struct {
	mode               HealthMode
	startupProbePath   string // /startup
	livenessProbePath  string // /healthz
	readinessProbePath string // /ready
}
