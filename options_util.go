package svcinit

// type managerCallbackFunc struct {
// 	beforeRun func(ctx context.Context, stage TaskStage, cause error) error
// 	afterRun  func(ctx context.Context, stage TaskStage, cause error) error
// }
//
// var _ ManagerCallback = managerCallbackFunc{}
//
// func (t managerCallbackFunc) BeforeRun(ctx context.Context, stage TaskStage, cause error) error {
// 	if t.beforeRun != nil {
// 		return t.beforeRun(ctx, stage, cause)
// 	}
// 	return nil
// }
//
// func (t managerCallbackFunc) AfterRun(ctx context.Context, stage TaskStage, cause error) error {
// 	if t.afterRun != nil {
// 		return t.afterRun(ctx, stage, cause)
// 	}
// 	return nil
// }
