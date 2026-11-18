//go:build !android || no_gui

package k2

import (
	"context"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/transport/hid"
)

func (Driver) LinuxUSBMatches() []device.LinuxUSBMatch {
	return []device.LinuxUSBMatch{{VendorID: VendorID, ProductID: ProductID, HIDRaw: true}}
}

func enumerateHID(context.Context) ([]hid.Device, error) {
	return hid.Enumerate(VendorID, ProductID)
}

func openHID(_ context.Context, info hid.Device) (hid.Reader, error) {
	return hid.Open(info, ReportSize+1)
}
