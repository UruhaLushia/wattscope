//go:build !no_gui

package k2

import (
	"context"
	"errors"
	"time"

	"github.com/UruhaLushia/wattscope/internal/transport/hid"
	"github.com/UruhaLushia/wattscope/internal/transport/usbhost"
)

func enumerateHID(ctx context.Context) ([]hid.Device, error) {
	paths, err := usbhost.Enumerate(ctx, VendorID, ProductID)
	if err != nil {
		return nil, err
	}
	devices := make([]hid.Device, 0, len(paths))
	for _, path := range paths {
		devices = append(devices, hid.Device{Path: path, VendorID: VendorID, ProductID: ProductID})
	}
	return devices, nil
}

func openHID(ctx context.Context, info hid.Device) (hid.Reader, error) {
	conn, err := usbhost.Connect(ctx, info.Path, 0, 0x81, -1)
	if err != nil {
		return nil, err
	}
	return androidHID{conn}, nil
}

type androidHID struct{ *usbhost.Connection }

func (conn androidHID) Read(ctx context.Context) ([]byte, error) {
	for ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		raw, err := conn.Connection.Read(readCtx, ReportSize)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			return raw, err
		}
	}
	return nil, ctx.Err()
}
