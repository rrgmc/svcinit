package svcinit

import (
	"context"
)

// Service is an abstraction of Task as an interface, for convenience.
// Use ServiceAsTask do the wrapping.
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ServiceWithSetup is a Service which has a Setup step.
type ServiceWithSetup interface {
	Setup(ctx context.Context) error
	Teardown(ctx context.Context) error
}

// ServiceTask allows getting the source Service of the Task.
type ServiceTask interface {
	Task
	Service() Service
}

// ServiceAsTask wraps a Service into a Task.
func ServiceAsTask(service Service) ServiceTask {
	t := &serviceTask{
		service: service,
		steps:   []Step{StepStart, StepStop},
	}
	if _, ok := t.service.(ServiceWithSetup); ok {
		t.steps = append(t.steps, StepSetup, StepTeardown)
	}
	return t
}

type serviceTask struct {
	service Service
	steps   []Step
}

var _ Task = (*serviceTask)(nil)
var _ TaskName = (*serviceTask)(nil)
var _ TaskSteps = (*serviceTask)(nil)
var _ ServiceTask = (*serviceTask)(nil)

func (t *serviceTask) Run(ctx context.Context, step Step) error {
	switch step {
	case StepSetup:
		if tt, ok := t.service.(ServiceWithSetup); ok {
			return tt.Setup(ctx)
		}
	case StepStart:
		return t.service.Start(ctx)
	case StepStop:
		return t.service.Stop(ctx)
	case StepTeardown:
		if tt, ok := t.service.(ServiceWithSetup); ok {
			return tt.Teardown(ctx)
		}
	default:
	}
	return nil
}

func (t *serviceTask) TaskSteps() []Step {
	return t.steps
}

func (t *serviceTask) Service() Service {
	return t.service
}

func (t *serviceTask) TaskName() string {
	if ts, ok := t.service.(TaskName); ok {
		return ts.TaskName()
	}
	return ""
}

func (t *serviceTask) String() string {
	if tn := t.TaskName(); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t.service)
}
