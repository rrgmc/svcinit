package svcinit

type BaseWrappedTask struct {
	Task Task
}

var _ TaskSteps = (*BaseWrappedTask)(nil)
var _ TaskWithOptions = (*BaseWrappedTask)(nil)

func (t *BaseWrappedTask) TaskOptions() []TaskInstanceOption {
	if tt, ok := t.Task.(TaskWithOptions); ok {
		return tt.TaskOptions()
	}
	return nil
}

func (t *BaseWrappedTask) TaskSteps() []Step {
	if tt, ok := t.Task.(TaskSteps); ok {
		return tt.TaskSteps()
	}
	return DefaultTaskSteps()
}

type baseWrappedTaskPrivate = BaseWrappedTask
