package svcinit

import (
	"context"
)

type TaskBuildDataFunc[T any] func(ctx context.Context, data T) error

type TaskBuildDataSetupFunc[T any] func(ctx context.Context) (T, error)

func BuildDataTask[T any](setupFunc TaskBuildDataSetupFunc[T], options ...TaskBuildDataOption[T]) Task {
	return newTaskBuildData[T](setupFunc, options...)
}

type TaskBuildDataOption[T any] func(*taskBuildData[T])

func WithDataDescription[T any](description string) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.description = description
	}
}

func WithDataStart[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStart, f)
}

func WithDataPreStop[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepPreStop, f)
}

func WithDataStop[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepStop, f)
}

func WithDataTeardown[T any](f TaskBuildDataFunc[T]) TaskBuildDataOption[T] {
	return withDataStep(StepTeardown, f)
}

func WithDataParent[T any](parent Task) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.parent = parent
	}
}

func WithDataParentFromSetup[T any](parentFromSetup bool) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.parentFromSetup = parentFromSetup
	}
}

func WithDataTaskOptions[T any](options ...TaskInstanceOption) TaskBuildDataOption[T] {
	return func(build *taskBuildData[T]) {
		build.options = append(build.options, options...)
	}
}
