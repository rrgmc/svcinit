package instancetask

import (
	"context"

	"github.com/rrgmc/svcinit/v3"
)

// Provider builds a task from the task returned by a callback.
// Any step not set in the built task will be forwarded to it.
func Provider(provider func(ctx context.Context) (svcinit.Task, error),
	options ...BuildOption[svcinit.Task]) svcinit.TaskWithData[svcinit.Task] {
	return Build[svcinit.Task](provider, append(options, WithParentFromSetup[svcinit.Task](true))...)
}
