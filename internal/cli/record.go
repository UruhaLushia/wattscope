package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/wattscope/internal/recording"
)

func newRecordCommand(source *sourceOptions) *cobra.Command {
	var opts acquisitionOptions
	var output string
	var jsonOutput bool
	cmd := &cobra.Command{Use: "record", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := service(*source, opts)
		if err != nil {
			return err
		}
		path, err := source.recordingPath(output, recording.NextPath)
		if err != nil {
			return err
		}
		if err := s.StartRecording(path); err != nil {
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), source.messages().CliRecording(s.Snapshot(0).Recording.Path))
		ctx, cancel := acquisitionContext(cmd.Context(), opts.duration)
		defer cancel()
		runErr := s.Run(ctx)
		v := s.Snapshot(0)
		if jsonOutput {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(v); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), source.messages().CliSaved(int(v.Recording.Samples), v.Recording.Path))
		}
		if runErr == nil && v.Recording.Samples == 0 && cmd.Context().Err() == nil {
			return errors.New(source.messages().NoMeasurementRecorded())
		}
		return runErr
	}}
	opts.flags(cmd.Flags(), false)
	cmd.Flags().StringVarP(&output, "output", "o", "", "")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}
