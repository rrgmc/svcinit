package svcinit

import "fmt"

type TaskWithID[T any] struct {
	id T
	*WrappedTask
}

func NewTaskWithID[T any](id T, task Task, options ...WrapTaskOption) *TaskWithID[T] {
	return &TaskWithID[T]{
		WrappedTask: NewWrappedTask(task, options...),
		id:          id,
	}
}

func (t *TaskWithID[T]) ID() T {
	return t.id
}

func (t *TaskWithID[T]) String() string {
	return fmt.Sprintf("TaskWithID(id=%v)", t.id)
}
