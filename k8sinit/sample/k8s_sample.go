package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rrgmc/svcinit/v3"
	"github.com/rrgmc/svcinit/v3/k8sinit"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		fmt.Println(err)
	}
}

func run(ctx context.Context) error {
	logger := defaultLogger(os.Stdout)

	sinit, err := k8sinit.New(
		k8sinit.WithHealthHandler(k8sinit.NewHealthHTTPServer()),
		k8sinit.WithManagerOptions(
			svcinit.WithLogger(logger),
		),
	)
	if err != nil {
		return err
	}

	sinit.AddTask(k8sinit.StageService, svcinit.BuildTask(
		svcinit.WithStart(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
			}
			return nil
		}),
	), svcinit.WithCancelContext(true))

	return sinit.Run(ctx)
}
