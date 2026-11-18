package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newReadCommand(source *sourceOptions) *cobra.Command {
	var opts acquisitionOptions
	var count uint64
	var jsonOutput bool
	cmd := &cobra.Command{Use: "read", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := service(*source, opts)
		if err != nil {
			return err
		}
		ctx, cancel := acquisitionContext(cmd.Context(), opts.duration)
		defer cancel()
		updates, unsubscribe := s.Subscribe()
		defer unsubscribe()
		done := make(chan error, 1)
		go func() { done <- s.Run(ctx) }()
		encoder := json.NewEncoder(cmd.OutOrStdout())
		msgs := source.messages()
		var seen, printed uint64
		consume := func() error {
			snapshot := s.Snapshot(0)
			if snapshot.Latest == nil || snapshot.Statistics.Samples == seen {
				return nil
			}
			seen = snapshot.Statistics.Samples
			v := snapshot.Latest
			var err error
			if jsonOutput {
				err = encoder.Encode(v)
			} else {
				label := ""
				if v.Simulated {
					label = " [" + msgs.Simulated() + "]"
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s  %.4f V  %.4f A  %.4f W%s\n", v.Timestamp.Format("15:04:05.000"), v.Voltage, v.Current, v.Power, label)
			}
			printed++
			return err
		}
		for {
			select {
			case <-updates:
				if err := consume(); err != nil {
					cancel()
					return errors.Join(err, <-done)
				}
				if count > 0 && printed >= count {
					cancel()
					return <-done
				}
			case runErr := <-done:
				if err := consume(); err != nil {
					return errors.Join(runErr, err)
				}
				if runErr == nil && printed == 0 && cmd.Context().Err() == nil {
					return errors.New(msgs.NoMeasurementRead())
				}
				return runErr
			}
		}
	}}
	opts.flags(cmd.Flags(), false)
	cmd.Flags().Uint64VarP(&count, "count", "n", 1, "")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}
