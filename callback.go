package svcinit

import "context"

// WaitStartTaskCallback is a TaskCallback that makes the stop task wait for the stop task to finished.
func WaitStartTaskCallback() TaskCallback {
	ret := &waitStartTaskCallback{}
	ret.waitCtx, ret.waitCancel = context.WithCancel(context.Background())
	return ret
}

type waitStartTaskCallback struct {
	waitCtx    context.Context
	waitCancel context.CancelFunc
}

func (t *waitStartTaskCallback) Callback(ctx context.Context, task Task, stage Stage, step Step, err error) {
	if stage == StageStart && step == StepAfter {
		t.waitCancel()
	} else if stage == StageStop && step == StepAfter {
		select {
		case <-ctx.Done():
		case <-t.waitCtx.Done():
		}
	}
}
