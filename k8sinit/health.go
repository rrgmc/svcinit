package k8sinit

type HealthHandler interface {
	ServiceStarted()
	ServiceTerminating()
}
