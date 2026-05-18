package instancetask

import (
	"context"

	"github.com/rrgmc/svcinit/v3"
)

func Provider(provider func(ctx context.Context) (svcinit.Task, error),
	options ...BuildOption[svcinit.Task]) svcinit.TaskWithData[svcinit.Task] {
	return Build[svcinit.Task](provider, append(options, WithParentFromSetup[svcinit.Task](true))...)
}
