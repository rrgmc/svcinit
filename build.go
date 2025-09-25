package svcinit

import (
	"context"
)

type TaskBuildFunc func(ctx context.Context) error

func BuildTask(options ...TaskBuildOption) Task {
	return newTaskBuild(options...)
}

type TaskBuildOption func(*taskBuild)

func WithParent(parent Task) TaskBuildOption {
	return func(build *taskBuild) {
		build.parent.Store(&parent)
	}
}

func WithDescription(description string) TaskBuildOption {
	return func(build *taskBuild) {
		build.description = description
	}
}

func WithStep(step Step, f TaskBuildFunc) TaskBuildOption {
	return func(build *taskBuild) {
		build.stepFunc[step] = f
	}
}

func WithSetup(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepSetup, f)
}

func WithStart(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStart, f)
}

func WithStop(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepStop, f)
}

func WithTeardown(f TaskBuildFunc) TaskBuildOption {
	return WithStep(StepTeardown, f)
}

func WithTaskOptions(options ...TaskInstanceOption) TaskBuildOption {
	return func(build *taskBuild) {
		build.options = append(build.options, options...)
	}
}
