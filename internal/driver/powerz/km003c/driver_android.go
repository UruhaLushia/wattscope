//go:build !no_gui

package km003c

import (
	"context"
	"fmt"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/transport/usbhost"
)

func (driver Driver) Discover(ctx context.Context) ([]device.Info, error) {
	paths, err := usbhost.Enumerate(ctx, vendorID, productID)
	if err != nil {
		return nil, err
	}
	devices := make([]device.Info, 0, len(paths))
	for _, path := range paths {
		devices = append(devices, device.Info{
			ID: driver.ID() + ":" + path, Driver: driver.ID(), Path: path, Name: driver.Name(),
			Transport: "usb", Capabilities: []string{device.CapPDCapture}, PathDependentID: true,
		})
	}
	return devices, nil
}

func (driver Driver) Open(ctx context.Context, selected device.Info) (device.Connection, error) {
	devices, err := driver.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, info := range devices {
		if info.ID == selected.ID && info.Path == selected.Path {
			conn, err := usbhost.Connect(ctx, info.Path, 0, endpointIn, endpointOut)
			if err != nil {
				return nil, err
			}
			return &usbConnection{transport: androidTransport{conn}}, nil
		}
	}
	return nil, device.ErrNoDevice
}

type androidTransport struct{ *usbhost.Connection }

func (conn androidTransport) bulk(ctx context.Context, endpoint byte, data []byte) (int, error) {
	switch endpoint {
	case endpointIn:
		raw, err := conn.Read(ctx, len(data))
		return copy(data, raw), err
	case endpointOut:
		return conn.Write(ctx, data)
	default:
		return 0, fmt.Errorf("unknown KM003C endpoint 0x%02x", endpoint)
	}
}
