package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newDevicesCommand(source *sourceOptions) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{Use: "devices", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := source.source().Registry()
		if err != nil {
			return err
		}
		devices, err := r.Discover(cmd.Context(), source.driver)
		if err != nil {
			return err
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(devices)
		}
		if len(devices) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), source.messages().NoDeviceFound())
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, source.messages().DeviceColumns())
		for _, d := range devices {
			name := d.Name
			if d.Simulated {
				name = source.messages().SimulatedMeter()
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", d.Driver, d.Path, name, d.Serial)
		}
		return w.Flush()
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "")
	return cmd
}
