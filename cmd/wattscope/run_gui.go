//go:build (android || no_cli) && !no_gui

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/UruhaLushia/wattscope/internal/app"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
)

func run(ctx context.Context, args []string) error {
	var source session.Source
	var language, directory string
	if len(args) != 0 && args[0] == "gui" {
		args = args[1:]
	}
	msgs := locales.In(locales.Resolve("auto"))
	flags := flag.NewFlagSet("wattscope", flag.ContinueOnError)
	flags.StringVar(&source.Driver, "driver", "", msgs.FlagDriver())
	flags.StringVar(&source.Device, "device", "", msgs.FlagDevice())
	flags.StringVar(&source.Device, "d", "", msgs.FlagDevice())
	flags.BoolVar(&source.Demo, "demo", false, msgs.FlagDemo())
	flags.StringVar(&language, "lang", "auto", msgs.FlagLang())
	flags.StringVar(&directory, "recordings-dir", "", msgs.FlagRecordingsDir())
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	return app.Run(ctx, source, language, directory)
}
