package health_http

type HealthHandler interface {
	ServiceStarted()
	ServiceTerminating()
}
