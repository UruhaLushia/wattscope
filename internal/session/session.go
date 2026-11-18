// Package session assembles meter drivers and the shared acquisition service.
// It is independent of Cobra, Bubble Tea and MyGo.
package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/driver/demo"
	"github.com/UruhaLushia/wattscope/internal/driver/powerz/km003c"
	"github.com/UruhaLushia/wattscope/internal/driver/witrn/k2"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

var (
	ErrDemoConflict  = errors.New("demo source conflicts with selected driver")
	ErrUnknownDriver = errors.New("unknown driver")
)

type Source struct {
	Driver string
	Device string
	Demo   bool
}

type Options struct {
	Source            Source
	Rate              int
	HistorySize       int
	PacketHistorySize int
	StaleAfter        time.Duration
	Reconnect         bool
}

// Drivers lists the available drivers, including the explicit demo source.
// New real meter drivers are registered here, once for all frontends.
func Drivers() []device.Driver {
	return []device.Driver{k2.Driver{}, km003c.Driver{}, demo.Driver{}}
}

func DecodePDReport(driverID string, raw []byte) (pd.Packet, error) {
	for _, driver := range Drivers() {
		if driver.ID() == driverID {
			decoder, ok := driver.(device.PDReportDecoder)
			if !ok {
				return pd.Packet{}, fmt.Errorf("driver %q does not decode PD reports", driverID)
			}
			return decoder.DecodePDReport(raw), nil
		}
	}
	return pd.Packet{}, fmt.Errorf("%w: %q", ErrUnknownDriver, driverID)
}

func (s Source) Registry() (*device.Registry, error) {
	if s.Demo {
		if s.Driver != "" && s.Driver != "demo" {
			return nil, fmt.Errorf("%w: %q", ErrDemoConflict, s.Driver)
		}
		return device.NewRegistry(demo.Driver{})
	}
	var drivers []device.Driver
	for _, driver := range Drivers() {
		if driver.ID() != "demo" {
			drivers = append(drivers, driver)
		}
	}
	r, err := device.NewRegistry(drivers...)
	if err != nil {
		return nil, err
	}
	if s.Driver != "" {
		for _, driver := range r.Drivers() {
			if driver.ID() == s.Driver {
				return r, nil
			}
		}
		return nil, fmt.Errorf("%w: %q", ErrUnknownDriver, s.Driver)
	}
	return r, nil
}

// New creates a session; the caller owns Run's context and must wait for it on
// shutdown, so pending recording data is flushed before closing the frontend.
func New(opts Options) (*backend.Service, error) {
	r, err := opts.Source.Registry()
	if err != nil {
		return nil, err
	}
	return backend.New(backend.Options{
		Open: r.Opener(opts.Source.Driver, opts.Source.Device), Rate: opts.Rate,
		HistorySize: opts.HistorySize, PacketHistorySize: opts.PacketHistorySize, Reconnect: opts.Reconnect, StaleAfter: opts.StaleAfter,
	})
}
