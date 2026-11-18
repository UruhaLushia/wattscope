//go:build linux && !android

package main

import (
	"context"
	"fmt"

	"github.com/UruhaLushia/wattscope/internal/meteraccess"
)

func runCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != meteraccess.Command {
		return run(ctx, args)
	}
	if len(args) != 1 {
		return fmt.Errorf("%s takes no arguments", meteraccess.Command)
	}
	return meteraccess.Apply(ctx)
}
