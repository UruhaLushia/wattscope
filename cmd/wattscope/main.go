package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/locales"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("WATTSCOPE_DEBUG") == "1" {
		ctx = diagnostics.WithLogger(ctx, slog.New(slog.NewTextHandler(os.Stderr, locales.DiagnosticTextOptions())))
	}
	if err := runCommand(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wattscope:", err)
		return 1
	}
	return 0
}
