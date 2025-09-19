package svcinit

import (
	"context"
)

// ExecuteTask executes the passed task which don't need a stop task.
// The context passed to the task will be canceled on stop.
// It is equivalent to "StartTask().AutoStop()".
// The task is only executed at the Run call.
func (s *Manager) ExecuteTask(task Task, options ...TaskOption) {
	s.addTask(s.unorderedCancelCtx, task, options...)
}

// StartTask executes a task and allows the shutdown method to be customized.
// At least one method of StartTaskCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *Manager) StartTask(task Task, options ...TaskOption) StartTaskCmd {
	cmd := StartTaskCmd{
		s:        s,
		start:    task,
		options:  options,
		preStop:  newValuePtr[Task](),
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// StartService executes a service task and allows the shutdown method to be customized.
// A service is a task with StartTask and StopTask methods.
// At least one method of StartServiceCmd must be called, or Run will fail.
// The task is only executed at the Run call.
func (s *Manager) StartService(svc Service, options ...TaskOption) StartServiceCmd {
	cmd := StartServiceCmd{
		s:        s,
		svc:      svc,
		options:  options,
		resolved: newResolved(),
	}
	s.addPendingStart(cmd)
	return cmd
}

// PreStopTask adds a pre-stop task. They will be called in parallel, before stop tasks starts.
func (s *Manager) PreStopTask(task Task, options ...TaskOption) {
	s.preStopTasks = append(s.preStopTasks, s.newStopTaskWrapper(task, options...))
}

// StopTask adds a shutdown task. The shutdown will be done in the order they are added.
func (s *Manager) StopTask(task Task, options ...TaskOption) {
	s.stopTasksOrdered = append(s.stopTasksOrdered, s.newStopTaskWrapper(task, options...))
}

// StopMultipleTasks adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *Manager) StopMultipleTasks(f func(MultipleTaskBuilder)) {
	var multiTasks []taskWrapper
	mtb := &multipleTaskBuilder{
		stopFuture: func(task StopFuture) {
			multiTasks = append(multiTasks, s.taskFromStopFuture(task))
		},
		stop: func(task Task) {
			multiTasks = append(multiTasks, s.newStopTaskWrapper(task))
		},
	}
	f(mtb)
	if len(multiTasks) > 0 {
		s.StopTask(newMultipleTask(multiTasks...))
	}
}

// StopFuture adds a shutdown task. The shutdown will be done in the order they are added.
func (s *Manager) StopFuture(task StopFuture) {
	s.stopTasksOrdered = append(s.stopTasksOrdered, s.taskFromStopFuture(task))
}

// StopFutureMultiple adds a shutdown task. The shutdown will be done in the order they are added.
// This method groups a list of stop tasks into a single one and run all of them in parallel.
// In this case, order between these tasks are undefined.
func (s *Manager) StopFutureMultiple(tasks ...StopFuture) {
	s.StopMultipleTasks(func(builder MultipleTaskBuilder) {
		for _, task := range tasks {
			builder.StopFuture(task)
		}
	})
}

// AutoStopTask adds a shutdown task, when the shutdown order DOES NOT matter.
func (s *Manager) AutoStopTask(task Task, options ...TaskOption) {
	s.stopTasks = append(s.stopTasks, s.newStopTaskWrapper(task, options...))
}

// addTask adds a task to be started.
func (s *Manager) addTask(ctx context.Context, task Task, options ...TaskOption) {
	s.tasks = append(s.tasks, s.newTaskWrapper(ctx, task, options...))
}
