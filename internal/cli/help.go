package cli

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/UruhaLushia/wattscope/locales"
)

// Help runs before Cobra's PreRun hooks. Resolve the parsed --lang here too,
// so `read --lang zh-Hans --help` and `help read` use the same language.
func configureHelp(root *cobra.Command, source *sourceOptions, hasGUI bool) {
	usageTemplate := root.UsageTemplate()
	defaultHelp := root.HelpFunc()
	apply := func() {
		lang := locales.Resolve(source.language)
		msgs := locales.In(lang)
		localizeCommand(root, msgs, lang)
		if hasGUI {
			root.Long = msgs.CliLong()
		}
		root.SetUsageTemplate(strings.NewReplacer(
			"Usage:", msgs.UsageHeading(), "Aliases:", msgs.AliasesHeading(),
			"Examples:", msgs.ExamplesHeading(), "Available Commands:", msgs.CommandsHeading(),
			"Global Flags:", msgs.GlobalFlagsHeading(), "Flags:", msgs.FlagsHeading(),
			"Additional help topics:", msgs.AdditionalHelpHeading(),
			`Use "{{.CommandPath}} [command] --help" for more information about a command.`, msgs.HelpHint("{{.CommandPath}}"),
		).Replace(usageTemplate))
	}
	apply()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		apply()
		// Cobra/pflag supply these syntax labels outside the usage template.
		out := cmd.OutOrStdout()
		var buffer bytes.Buffer
		cmd.SetOut(&buffer)
		defer cmd.SetOut(out)
		defaultHelp(cmd, args)
		msgs := source.messages()
		text := strings.NewReplacer(" (default ", " ("+msgs.DefaultValue()+" ",
			"[flags]", msgs.FlagsToken(), "[command]", msgs.CommandToken()).Replace(buffer.String())
		fmt.Fprint(out, text)
	})
}

func localizeCommand(cmd *cobra.Command, m locales.Messages, lang string) {
	shorts := map[string]string{
		"wattscope": m.CliDescription(), "drivers": m.CliDrivers(), "devices": m.CliDevices(),
		"read": m.CliRead(), "record": m.CliRecord(), "gui": m.CliGui(), "tui": m.CliTui(),
		"help": m.HelpShort(), "completion": m.CompletionShort(),
		"pd": m.CliPd(), "capture": m.CliPdCapture(), "decode": m.CliPdDecode(), "view": m.CliPdView(),
		"analyze": m.CliPdAnalyze(),
	}
	if short, ok := shorts[cmd.Name()]; ok {
		cmd.Short = short
	}
	switch cmd.Name() {
	case "help":
		cmd.Long = m.HelpLong()
	case "completion":
		cmd.Long = m.CompletionLong()
	}
	if parent := cmd.Parent(); parent != nil && parent.Name() == "completion" {
		cmd.Short = m.ShellCompletion(cmd.Name())
		if lang != "en" {
			cmd.Long = m.ShellCompletionHelp(cmd.Root().Name(), cmd.Name())
		}
	}
	flags := map[string]string{
		"driver": m.FlagDriver(), "device": m.FlagDevice(), "demo": m.FlagDemo(),
		"recordings-dir": m.FlagRecordingsDir(), "lang": m.FlagLang(),
		"rate": m.FlagRate(), "history": m.FlagHistory(), "timeout": m.FlagTimeout(),
		"duration": m.FlagDuration(), "reconnect": m.FlagReconnect(), "window": m.FlagWindow(),
		"count": m.FlagCount(), "help": m.HelpFlag(), "version": m.VersionFlag(),
		"no-descriptions": m.CompletionNoDescriptions(),
		"pd-history":      m.FlagPdHistory(), "k2": m.FlagK2Envelope(), "sop": m.FlagSop(),
		"pd-output": m.FlagPdOutput(),
	}
	if cmd.Name() == "analyze" {
		flags["json"] = m.FlagAnalysisJson()
	} else if cmd.Name() == "capture" {
		flags["output"], flags["json"] = m.FlagCaptureOutput(), m.FlagCaptureJson()
	} else if cmd.Name() == "decode" {
		flags["json"] = m.FlagCaptureJson()
	} else if cmd.Name() == "record" {
		flags["output"], flags["json"] = m.FlagRecordOutput(), m.FlagRecordJson()
	} else {
		flags["output"] = m.FlagTuiOutput()
		if cmd.Name() == "read" {
			flags["json"] = m.FlagReadJson()
		} else {
			flags["json"] = m.FlagDevicesJson()
		}
	}
	localizeFlag := func(flag *pflag.Flag) {
		if usage, ok := flags[flag.Name]; ok {
			flag.Usage = usage
		}
	}
	cmd.Flags().VisitAll(localizeFlag)
	cmd.PersistentFlags().VisitAll(localizeFlag)
	for _, child := range cmd.Commands() {
		localizeCommand(child, m, lang)
	}
}
