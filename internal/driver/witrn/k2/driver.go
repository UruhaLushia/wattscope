package k2

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/transport/hid"
)

type Driver struct{}

func (Driver) ID() string   { return "witrn-k2" }
func (Driver) Name() string { return "WITRN K2" }

func (d Driver) Discover(ctx context.Context) ([]device.Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	interfaces, err := enumerateHID(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]device.Info, 0, len(interfaces))
	for _, h := range interfaces {
		devices = append(devices, d.info(h))
	}
	return devices, nil
}

func (d Driver) info(h hid.Device) device.Info {
	location := h.Location
	if location == "" {
		location = h.Path
	}
	return device.Info{
		ID: d.ID() + ":" + location + ":" + h.Serial, Driver: d.ID(), Path: h.Path,
		Name: d.Name(), Serial: h.Serial, Transport: "hid", Capabilities: []string{device.CapPDCapture, device.CapPDHostMode},
		PathDependentID: runtime.GOOS == "android" && h.Location == "",
	}
}

func (d Driver) Open(ctx context.Context, selected device.Info) (device.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if selected.Driver != d.ID() {
		return nil, errors.New("incorrect driver selection")
	}
	interfaces, err := enumerateHID(ctx)
	if err != nil {
		return nil, err
	}
	for _, h := range interfaces {
		if d.info(h).ID != selected.ID || h.Path != selected.Path {
			continue
		}
		r, err := openHID(ctx, h)
		if err != nil {
			return nil, err
		}
		return &connection{raw: r, device: h}, nil
	}
	return nil, errors.New("selected K2 disconnected")
}

type connection struct {
	raw    hid.Reader
	device hid.Device
}

func (c *connection) Read(ctx context.Context) (device.Sample, error) {
	event, err := c.ReadEvent(ctx)
	if err != nil {
		return device.Sample{}, err
	}
	if event.Sample == nil {
		return device.Sample{}, device.ErrIgnoredReport
	}
	return *event.Sample, nil
}

func (c *connection) ReadEvent(ctx context.Context) (device.Event, error) {
	raw, err := c.raw.Read(ctx)
	if err != nil {
		return device.Event{}, err
	}
	packetRaw := raw
	if len(raw) == ReportSize+1 && raw[0] == 0 {
		packetRaw = raw[1:]
	}
	if len(packetRaw) > 0 && packetRaw[0] == 0xfe {
		packet := DecodePD(raw)
		return device.Event{PD: &packet}, nil
	}
	sample, err := Decode(raw)
	if err != nil && !errors.Is(err, device.ErrIgnoredReport) {
		return device.Event{}, fmt.Errorf("%w: %v", device.ErrInvalidReport, err)
	}
	if err != nil {
		return device.Event{}, err
	}
	return device.Event{Sample: &sample}, nil
}

func (c *connection) Close() error { return c.raw.Close() }
