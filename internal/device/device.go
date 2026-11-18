// Package device defines the common contract for power meters. Protocols and
// transports belong to drivers; consumers do not depend on a particular meter.
package device

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

var (
	ErrIgnoredReport   = errors.New("non-measurement report")
	ErrInvalidReport   = errors.New("invalid measurement report")
	ErrNoDevice        = errors.New("no matching meter")
	ErrMultipleDevices = errors.New("multiple matching meters")
	ErrAccessDeclined  = fmt.Errorf("device access not granted: %w", os.ErrPermission)
)

const (
	CapPDCapture   = "pd_capture"
	CapPDHostMode  = "pd_host_mode"
	CapPDDirectUSB = "pd_direct_usb"
)

type Info struct {
	ID              string   `json:"id"`
	Driver          string   `json:"driver"`
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	Serial          string   `json:"serial,omitempty"`
	Transport       string   `json:"transport"`
	Simulated       bool     `json:"simulated"`
	Capabilities    []string `json:"capabilities,omitempty"`
	PathDependentID bool     `json:"-"`
}

func (d Info) Supports(capability string) bool {
	return slices.Contains(d.Capabilities, capability)
}

type CurrentDirection string

const (
	CurrentPlugToReceptacle CurrentDirection = "plug_to_receptacle"
	CurrentReceptacleToPlug CurrentDirection = "receptacle_to_plug"
)

// Sample uses V, A, W, Celsius, Ah, Wh and seconds. Optional fields are nil when
// unsupported or unavailable, never fabricated zero values. Current and Power
// are non-negative magnitudes; CurrentDirection and SignedPower retain direction. Segment identifies breaks
// in acquisition continuity, independently of a device's recording group.
type Sample struct {
	Timestamp           time.Time        `json:"timestamp"`
	Elapsed             float64          `json:"elapsed_s"`
	Segment             uint64           `json:"segment"`
	Simulated           bool             `json:"simulated"`
	Voltage             float64          `json:"voltage_v"`
	Current             float64          `json:"current_a"`
	CurrentDirection    CurrentDirection `json:"current_direction,omitempty"`
	Power               float64          `json:"power_w"`
	SignedPower         float64          `json:"signed_power_w"`
	Temperature         *float64         `json:"temperature_c"`
	ExternalTemperature *float64         `json:"external_temperature_c"`
	DP                  *float64         `json:"dp_v"`
	DN                  *float64         `json:"dn_v"`
	CC1                 *float64         `json:"cc1_v,omitempty"`
	CC2                 *float64         `json:"cc2_v,omitempty"`
	AmpHours            *float64         `json:"device_ah"`
	WattHours           *float64         `json:"device_wh"`
	RecordMinCurrent    *float64         `json:"device_record_min_current_a,omitempty"`
	RecordSeconds       *uint32          `json:"device_record_s"`
	Uptime              *uint32          `json:"device_uptime_s"`
	Group               *uint16          `json:"group"`

	CurrentDirectionStable bool `json:"-"`
}

// Connection is single-consumer and must honor context cancellation. Drivers
// normalize reports into samples. Cancel Read before calling Close.
type Connection interface {
	Read(context.Context) (Sample, error)
	Close() error
}

// EventConnection is an optional capability for meters that also emit protocol
// captures. A single reader demultiplexes both streams; never read it in parallel
// with Connection.Read. Measurement-only drivers need not implement this.
type EventConnection interface {
	Connection
	ReadEvent(context.Context) (Event, error)
}

type PDHostChecker interface {
	CheckSavedPDHost(context.Context) (bool, error)
}

type PDReportDecoder interface {
	DecodePDReport([]byte) pd.Packet
}

type Event struct {
	Sample      *Sample
	PD          *pd.Packet
	PDLinkReset bool
}

type Driver interface {
	ID() string
	Name() string
	Discover(context.Context) ([]Info, error)
	Open(context.Context, Info) (Connection, error)
}

type LinuxUSBMatch struct {
	VendorID, ProductID uint16
	HIDRaw              bool
}

type OpenFunc func(context.Context) (Info, Connection, error)

// Clone prevents frontends mutating optional measurements owned by the backend.
func Clone(s Sample) Sample {
	s.Temperature = copyValue(s.Temperature)
	s.ExternalTemperature = copyValue(s.ExternalTemperature)
	s.DP = copyValue(s.DP)
	s.DN = copyValue(s.DN)
	s.CC1 = copyValue(s.CC1)
	s.CC2 = copyValue(s.CC2)
	s.AmpHours = copyValue(s.AmpHours)
	s.WattHours = copyValue(s.WattHours)
	s.RecordMinCurrent = copyValue(s.RecordMinCurrent)
	s.RecordSeconds = copyValue(s.RecordSeconds)
	s.Uptime = copyValue(s.Uptime)
	s.Group = copyValue(s.Group)
	return s
}

func copyValue[T any](v *T) *T {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}
