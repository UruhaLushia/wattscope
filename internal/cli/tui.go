package cli

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/internal/tui"
)

func newTUICommand(source *sourceOptions) *cobra.Command {
	var opts acquisitionOptions
	var window time.Duration
	var output, pdOutput string
	cmd := &cobra.Command{Use: "tui", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if window <= 0 || window > 24*time.Hour {
			return errors.New(source.messages().InvalidWindow())
		}
		s, err := service(*source, opts)
		if err != nil {
			return err
		}
		directory, err := recording.Directory(source.recordingsDir)
		if err != nil {
			return err
		}
		if output != "" {
			if err := s.StartRecording(output); err != nil {
				return err
			}
			if source.recordingsDir == "" && os.Getenv("WATTSCOPE_RECORDINGS_DIR") == "" {
				directory = filepath.Dir(s.Snapshot(0).Recording.Path)
			}
		}
		if pdOutput != "" {
			if err := s.StartCapture(pdOutput); err != nil {
				return errors.Join(err, s.StopRecording())
			}
			if output == "" && source.recordingsDir == "" && os.Getenv("WATTSCOPE_RECORDINGS_DIR") == "" {
				directory = filepath.Dir(s.PacketSnapshot(0, 0).Recording.Path)
			}
		}
		ctx, cancel := acquisitionContext(cmd.Context(), opts.duration)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- s.Run(ctx); cancel() }()
		err = tui.Run(ctx, s, tui.Options{Window: window, RecordingDir: directory, Locale: source.language})
		cancel()
		return errors.Join(err, <-done)
	}}
	opts.flags(cmd.Flags(), true)
	cmd.Flags().DurationVar(&window, "window", time.Minute, "")
	cmd.Flags().StringVarP(&output, "output", "o", "", "")
	cmd.Flags().StringVar(&pdOutput, "pd-output", "", "")
	return cmd
}
