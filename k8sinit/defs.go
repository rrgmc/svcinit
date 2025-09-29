package k8sinit

const (
	StageManagement = "management" // 1st stage: initialize telemetry, health server and signal handling
	StageInitialize = "initialize" // 2nd stage: initialize data, like DB connections
	StageReady      = "ready"      // 3rd stage: signals probes that the service has completely started
	StageService    = "service"    // 4th state: initialize services
)

var allStages = []string{StageManagement, StageInitialize, StageReady, StageService}

func AllStages() []string {
	return allStages
}

// task names

const (
	TaskNameHealth                 = "health"
	TaskNameHealthStartedProbe     = "health: started probe"
	TaskNameHealthTerminatingProbe = "health: terminating probe"
	TaskNameTelemetry              = "telemetry"
	TaskNameTelemetryFlush         = "telemetry: flush"
)
