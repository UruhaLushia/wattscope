// Package monitor is a native module: its interfaces are implemented in
// Swift, Kotlin, Rust or Zig, in the directory of the language (see AGENTS.md).
package monitor

//mygo:native
type Monitor interface {
	Update(power, voltage, current, energyWh, chargeMAh, measuredSec float64, deviceLabel, language, islandLeft string, streaming, recording, islandEnabled bool) error
	SupportsIsland() (bool, error)
	Ensure() error
	Stop() error
}
