// Package hid provides read-only OS HID transports. Device report interpretation
// belongs to drivers, so this layer can be shared by unrelated meters.
package hid

import "context"

const maxReportSize = 64 * 1024

type Device struct {
	Path, Name, Serial, Location string
	VendorID, ProductID          uint16
}

type Reader interface {
	Read(context.Context) ([]byte, error)
	Close() error
}
