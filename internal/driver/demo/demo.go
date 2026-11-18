// Package demo is a synthetic meter driver. It is registered only when the
// caller explicitly requests --demo; hardware failures never fall back to it.
package demo

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"math"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

type Driver struct{}

func (Driver) ID() string   { return "demo" }
func (Driver) Name() string { return "Simulated power meter" }
func (d Driver) Discover(ctx context.Context) ([]device.Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []device.Info{{ID: "demo", Driver: d.ID(), Path: "demo", Name: d.Name(), Transport: "simulated", Simulated: true, Capabilities: []string{device.CapPDCapture}}}, nil
}
func (Driver) Open(ctx context.Context, _ device.Info) (device.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &connection{started: time.Now(), tick: time.NewTicker(10 * time.Millisecond)}, nil
}

type connection struct {
	started time.Time
	tick    *time.Ticker
	events  uint64
	packets uint64
}

func (d *connection) ReadEvent(ctx context.Context) (device.Event, error) {
	sample, err := d.Read(ctx)
	if err != nil {
		return device.Event{}, err
	}
	d.events++
	if d.events%20 != 0 {
		return device.Event{Sample: &sample}, nil
	}
	index := d.packets % 8
	d.packets++
	header := uint16(0x1a1) | uint16(d.packets%8)<<9
	var objects []uint32
	switch index {
	case 0:
		header |= 3 << 12
		objects = []uint32{100<<10 | 300, 180<<10 | 300, 3<<30 | 110<<17 | 33<<8 | 60}
	case 1:
		header = 0x82 | uint16(d.packets%8)<<9 | 1<<12
		objects = []uint32{2<<28 | 200<<10 | 200}
	case 2:
		header = header&^31 | 3
	case 3:
		header = header&^31 | 6
	case 4: // Structured Discover Identity VDM.
		header = header&^31 | 15 | 1<<12
		objects = []uint32{0xff008001}
	case 5: // Extended_Control, EPR KeepAlive (type 3).
		header = header&^31 | 16 | 0x8000 | 1<<12
		objects = []uint32{0x00030002}
	case 6:
		header = header&^31 | 13 // Soft_Reset.
	}
	raw := make([]byte, 2+4*len(objects)+4)
	binary.LittleEndian.PutUint16(raw, header)
	for i, object := range objects {
		binary.LittleEndian.PutUint32(raw[2+i*4:], object)
	}
	binary.LittleEndian.PutUint32(raw[len(raw)-4:], crc32.ChecksumIEEE(raw[:len(raw)-4]))
	packet := pd.Decode("SOP", raw)
	return device.Event{Sample: &sample, PD: &packet}, nil
}

func (d *connection) Read(ctx context.Context) (device.Sample, error) {
	select {
	case <-ctx.Done():
		return device.Sample{}, ctx.Err()
	case at := <-d.tick.C:
		t := at.Sub(d.started).Seconds()
		v := 9 + .025*math.Sin(t*1.7)
		a := 1.8 + .45*math.Sin(t*.35) + .035*math.Sin(t*3)
		temp := 31 + 3*math.Sin(t*.05)
		return device.Sample{Timestamp: at.UTC(), Voltage: v, Current: a, Power: v * math.Abs(a), SignedPower: v * a, Temperature: &temp, Simulated: true}, nil
	}
}
func (d *connection) Close() error { d.tick.Stop(); return nil }
