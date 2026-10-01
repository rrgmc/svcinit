package svcinit

import (
	"context"
	"slices"
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
}

// ServiceWithTeardown is a Service which has a Teardown step.
type ServiceWithTeardown interface {
	Teardown(ctx context.Context) error
}

// ServiceTask allows getting the source Service of the Task.
type ServiceTask interface {
	Task
	Service() Service
}

// ServiceAsTask wraps a Service into a Task.
// The Service may implement [TaskWithInfo]; its info is used for the task, except for [TaskInfo.Steps], which is
// computed from the Service interfaces it implements.
func ServiceAsTask(service Service) ServiceTask {
	t := &serviceTask{
		service: service,
		steps:   []Step{StepStart, StepStop},
	}
	if _, ok := t.service.(ServiceWithSetup); ok {
		t.steps = append(t.steps, StepSetup)
	}
	if _, ok := t.service.(ServiceWithTeardown); ok {
		t.steps = append(t.steps, StepTeardown)
	}
	return t
}

type serviceTask struct {
	service Service
	steps   []Step
}

var _ ServiceTask = (*serviceTask)(nil)
var _ TaskWithInfo = (*serviceTask)(nil)

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
		if tt, ok := t.service.(ServiceWithTeardown); ok {
			return tt.Teardown(ctx)
		}
	default:
	}
	return nil
}

func (t *serviceTask) TaskInfo() TaskInfo {
	var info TaskInfo
	if ti, ok := t.service.(TaskWithInfo); ok {
		info = ti.TaskInfo()
	}
	info.Steps = slices.Clone(t.steps)
	return info
}

func (t *serviceTask) Service() Service {
	return t.service
}

func (t *serviceTask) String() string {
	if tn := GetTaskName(t); tn != "" {
		return tn
	}
	return getDefaultTaskDescription(t.service)
}
