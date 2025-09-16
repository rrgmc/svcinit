package svcinit

import "context"

type managerCallbackFunc struct {
	beforeRun func(ctx context.Context, isStart bool, cause error) error
	afterRun  func(ctx context.Context, isStart bool, cause error) error
}

var _ ManagerCallback = managerCallbackFunc{}

func (t managerCallbackFunc) BeforeRun(ctx context.Context, isStart bool, cause error) error {
	if t.beforeRun != nil {
		return t.beforeRun(ctx, isStart, cause)
	}
	return nil
}

func (t managerCallbackFunc) AfterRun(ctx context.Context, isStart bool, cause error) error {
	if t.afterRun != nil {
		return t.afterRun(ctx, isStart, cause)
	}
	return nil
}
