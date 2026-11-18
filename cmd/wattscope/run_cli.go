//go:build !no_cli && no_gui

package main

import (
	"context"

	"github.com/UruhaLushia/wattscope/internal/cli"
)

func run(ctx context.Context, args []string) error {
	return cli.Run(ctx, args, nil)
}
