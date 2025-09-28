package k8sinit

// options

type HealthOptions func(*healthOptions)

func WithHealthStartupProbePath(path string) HealthOptions {
	return func(h *healthOptions) {
		h.startupProbePath = path
	}
}

func WithHealthLivenessProbePath(path string) HealthOptions {
	return func(h *healthOptions) {
		h.livenessProbePath = path
	}
}

func WithHealthReadinessProbePath(path string) HealthOptions {
	return func(h *healthOptions) {
		h.readinessProbePath = path
	}
}

type healthOptions struct {
	startupProbePath   string // /startup
	livenessProbePath  string // /healthz
	readinessProbePath string // /ready
}
