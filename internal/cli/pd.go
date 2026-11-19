package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/pdview"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/internal/tui"
)

func newPDCommand(source *sourceOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "pd"}
	cmd.AddCommand(newPDCaptureCommand(source), newPDDecodeCommand(source), newPDViewCommand(source), newPDAnalyzeCommand(source))
	return cmd
}

func newPDCaptureCommand(source *sourceOptions) *cobra.Command {
	var opts acquisitionOptions
	var output string
	var jsonOutput bool
	cmd := &cobra.Command{Use: "capture", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := service(*source, opts)
		if err != nil {
			return err
		}
		path, err := source.recordingPath(output, recording.NextCapturePath)
		if err != nil {
			return err
		}
		if err := s.StartCapture(path); err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), source.messages().CaptureStarted(s.PacketSnapshot(0, 0).Recording.Path))
		ctx, cancel := acquisitionContext(cmd.Context(), opts.duration)
		defer cancel()
		updates, unsubscribe := s.Subscribe()
		defer unsubscribe()
		done := make(chan error, 1)
		go func() { done <- s.Run(ctx) }()
		var after uint64
		warned := false
		msgs := source.messages()
		encoder := json.NewEncoder(cmd.OutOrStdout())
		consume := func() error {
			for {
				v := s.PacketSnapshot(after, 256)
				if after+1 < v.Oldest && !warned {
					fmt.Fprintln(cmd.ErrOrStderr(), msgs.CaptureOutputGap())
					warned = true
				}
				if len(v.Packets) == 0 {
					return nil
				}
				for _, packet := range v.Packets {
					var err error
					if jsonOutput {
						err = encoder.Encode(packet)
					} else {
						_, err = fmt.Fprintln(cmd.OutOrStdout(), packetLine(packet, pdview.ExplainPacket(msgs, packet).Summary))
					}
					if err != nil {
						return err
					}
					after = packet.Sequence
				}
			}
		}
		finish := func(runErr error) error {
			if err := consume(); err != nil {
				return errors.Join(runErr, err)
			}
			v := s.PacketSnapshot(0, 0)
			fmt.Fprintln(cmd.ErrOrStderr(), msgs.CaptureSaved(fmt.Sprint(v.Recording.Packets), v.Recording.Path))
			if runErr == nil && v.Total == 0 && cmd.Context().Err() == nil {
				info := s.Snapshot(0).Device
				switch {
				case info.Supports(device.CapPDDirectUSB):
					return errors.New(msgs.PdDirectUsb())
				case info.Supports(device.CapPDHostMode):
					return errors.New(msgs.CaptureNoPackets() + " " + msgs.PdInstruction())
				default:
					return errors.New(msgs.CaptureNoPackets() + " " + msgs.PdRetrigger())
				}
			}
			return runErr
		}
		for {
			select {
			case <-updates:
				if err := consume(); err != nil {
					cancel()
					return errors.Join(err, <-done)
				}
			case err := <-done:
				return finish(err)
			}
		}
	}}
	opts.flags(cmd.Flags(), false)
	cmd.Flags().StringVarP(&output, "output", "o", "", "")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}

func newPDDecodeCommand(source *sourceOptions) *cobra.Command {
	var envelope, jsonOutput bool
	var sop string
	cmd := &cobra.Command{Use: "decode [hex]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		msgs := source.messages()
		input := ""
		if len(args) > 0 {
			input = args[0]
		} else {
			data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 4097))
			if err != nil {
				return err
			}
			input = string(data)
		}
		if len(input) > 4096 {
			return errors.New(msgs.InvalidHex())
		}
		input = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) || r == ':' {
				return -1
			}
			return r
		}, input)
		input = strings.TrimPrefix(strings.TrimPrefix(input, "0x"), "0X")
		raw, err := hex.DecodeString(input)
		if err != nil || len(raw) == 0 || len(raw) > 520 {
			return errors.New(msgs.InvalidHex())
		}
		var packet pd.Packet
		if envelope {
			packet, err = session.DecodePDReport("witrn-k2", raw)
			if err != nil {
				return err
			}
		} else {
			switch sop {
			case "SOP", "SOP'", "SOP''", "SOP'_DEBUG", "SOP''_DEBUG":
			default:
				return errors.New(msgs.InvalidSop())
			}
			packet = pd.Decode(sop, raw)
		}
		var analyzer pd.Analyzer
		analyzer.Observe(&packet)
		if jsonOutput {
			err = json.NewEncoder(cmd.OutOrStdout()).Encode(packet)
		} else {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), packetLine(packet, pdview.ExplainPacket(msgs, packet).Summary))
			if err == nil {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %04X · %s: %s\n%s: %s\n", msgs.Header(), packet.Header, msgs.Crc(), packet.CRC, msgs.RawBytes(), packet.MessageHex)
			}
		}
		if err != nil {
			return err
		}
		if packet.Error != "" {
			return fmt.Errorf("%s: %s", msgs.PacketError(), packet.Error)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&envelope, "k2", false, "")
	cmd.Flags().StringVar(&sop, "sop", "SOP", "")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}

func newPDAnalyzeCommand(source *sourceOptions) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{Use: "analyze <file>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		file, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer file.Close()
		m := source.messages()
		encoder := json.NewEncoder(cmd.OutOrStdout())
		count, err := recording.ReadCapture(file, func(packet pd.Packet) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			if jsonOutput {
				return encoder.Encode(packet)
			}
			explanation := pdview.ExplainPacket(m, packet)
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), packetLine(packet, explanation.Summary)); err != nil {
				return err
			}
			for _, field := range explanation.Fields {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "         %s · %s\n", field.Label, field.Value); err != nil {
					return err
				}
			}
			for _, line := range append(explanation.Details, explanation.Warnings...) {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "         "+line); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New(m.EmptyCapture())
		}
		fmt.Fprintln(cmd.ErrOrStderr(), m.AnalysisComplete(fmt.Sprint(count)))
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}

func newPDViewCommand(source *sourceOptions) *cobra.Command {
	var limit int
	cmd := &cobra.Command{Use: "view <file>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || limit > 100000 {
			return errors.New(source.messages().InvalidPdHistory())
		}
		file, err := os.Open(args[0])
		if err != nil {
			return err
		}
		packets, total, err := recording.LoadCapture(file, limit)
		err = errors.Join(err, file.Close())
		if err != nil {
			return err
		}
		if total == 0 {
			return errors.New(source.messages().EmptyCapture())
		}
		snapshot := backend.PacketSnapshot{Packets: packets, Total: total, Evicted: total - uint64(len(packets))}
		if len(packets) > 0 {
			snapshot.Oldest = packets[0].Sequence
		}
		for _, p := range packets {
			if p.Error != "" {
				snapshot.Invalid++
			}
		}
		dir, err := recording.Directory(source.recordingsDir)
		if err != nil {
			return err
		}
		return tui.Run(cmd.Context(), nil, tui.Options{Locale: source.language, RecordingDir: dir, Capture: &snapshot, SourcePath: args[0]})
	}}
	cmd.Flags().IntVar(&limit, "pd-history", 2000, "")
	return cmd
}

func packetLine(p pd.Packet, summary string) string {
	line := fmt.Sprintf("%6d  %10.3f  %-10s %-6s %-30s", p.Sequence, p.Elapsed, p.SOP, p.PowerRole, p.Type)
	line += "  " + summary
	for _, pdo := range p.PDOs {
		line += "  " + pdo.Summary()
	}
	if p.Error != "" {
		line += "  ! " + p.Error
	}
	return line
}
