//go:build !linux || android

package main

import "context"

func runCommand(ctx context.Context, args []string) error {
	return run(ctx, args)
}
