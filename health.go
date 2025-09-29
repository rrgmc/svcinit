package svcinit

// HealthHandler is a health handler definition that allows implementing probes.
type HealthHandler interface {
	ServiceStarted()
	ServiceTerminating()
}
