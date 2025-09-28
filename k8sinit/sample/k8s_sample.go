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
		k8sinit.WithManagerOptions(
			svcinit.WithLogger(logger),
		),
	)
	if err != nil {
		return err
	}

	return sinit.Run(ctx)
}
