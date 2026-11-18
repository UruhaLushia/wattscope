// Package cli wires device drivers and frontends. The backend itself has no
// dependency on command flags, terminal UI, MyGo or concrete meter protocols.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
)

type sourceOptions struct {
	driver, selector string
	recordingsDir    string
	demo             bool
	language         string
}

func (o sourceOptions) source() session.Source {
	return session.Source{Driver: o.driver, Device: o.selector, Demo: o.demo}
}

func (o sourceOptions) messages() locales.Messages {
	return locales.In(locales.Resolve(o.language))
}

func Run(ctx context.Context, args []string, runGUI func(context.Context, session.Source, string, string) error) error {
	if len(args) == 0 && runGUI != nil {
		args = []string{"gui"}
	}
	command := newCommand(runGUI)
	command.SetArgs(args)
	return command.ExecuteContext(ctx)
}

func newCommand(runGUI func(context.Context, session.Source, string, string) error) *cobra.Command {
	cobra.MousetrapHelpText = ""
	source := &sourceOptions{}
	root := &cobra.Command{
		Use: "wattscope", Version: "0.1.0-dev",
		SilenceUsage: true, SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&source.driver, "driver", "", "")
	root.PersistentFlags().StringVarP(&source.selector, "device", "d", "", "")
	root.PersistentFlags().BoolVar(&source.demo, "demo", false, "")
	root.PersistentFlags().StringVar(&source.recordingsDir, "recordings-dir", "", "")
	root.PersistentFlags().StringVar(&source.language, "lang", "auto", "")
	if runGUI != nil {
		root.AddCommand(&cobra.Command{Use: "gui", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return runGUI(cmd.Context(), source.source(), source.language, source.recordingsDir)
		}})
	}
	tuiCmd := newTUICommand(source)
	// Copy the TUI's local flags to support `wattscope --demo` without a subcommand.
	root.Flags().AddFlagSet(tuiCmd.Flags())
	root.RunE = tuiCmd.RunE
	root.Args = cobra.NoArgs
	root.AddCommand(tuiCmd, newDevicesCommand(source), newDriversCommand(source), newReadCommand(source), newRecordCommand(source), newPDCommand(source))
	configureHelp(root, source, runGUI != nil)
	localizeErrors(root, source)
	return root
}

func localizeErrors(cmd *cobra.Command, source *sourceOptions) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			err := run(cmd, args)
			msgs := source.messages()
			switch {
			case errors.Is(err, session.ErrDemoConflict):
				return errors.New(msgs.DemoConflict(source.driver))
			case errors.Is(err, session.ErrUnknownDriver):
				return errors.New(msgs.UnknownDriver(source.driver))
			case errors.Is(err, device.ErrNoDevice):
				return errors.New(msgs.MissingMessage())
			case errors.Is(err, device.ErrMultipleDevices):
				return errors.New(msgs.SelectionMessage())
			case errors.Is(err, os.ErrPermission):
				return fmt.Errorf("%s: %w", msgs.PermissionMessage(), err)
			default:
				return err
			}
		}
	}
	for _, child := range cmd.Commands() {
		localizeErrors(child, source)
	}
}

func newDriversCommand(source *sourceOptions) *cobra.Command {
	return &cobra.Command{Use: "drivers", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		for _, driver := range session.Drivers() {
			name := driver.Name()
			if driver.ID() == "demo" {
				name = source.messages().SimulatedMeter()
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %s\n", driver.ID(), name)
		}
		return nil
	}}
}
