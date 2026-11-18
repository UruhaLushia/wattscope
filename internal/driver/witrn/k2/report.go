package k2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

const (
	VendorID         = 0x0716
	ProductID        = 0x5060
	ReportSize       = 64
	TelemetryCommand = 0x1a
)

func reportChecksums(frame []byte) (inner, outer byte) {
	for _, value := range frame[:8] {
		outer += value
	}
	for _, value := range frame[8:62] {
		inner += value
	}
	return inner, outer + inner
}

// Decode accepts a 64-byte HID report or a zero Report ID followed by 64 bytes.
func Decode(raw []byte) (device.Sample, error) {
	var s device.Sample
	if len(raw) == ReportSize+1 && raw[0] == 0 {
		raw = raw[1:]
	}
	if len(raw) != ReportSize {
		return s, fmt.Errorf("HID report length %d, want 64 or 00+64", len(raw))
	}
	if raw[0] != 0xff || raw[1] != 0x55 {
		return s, errors.New("invalid K2 report header")
	}
	payloadLength := int(raw[9])
	if payloadLength > 52 {
		return s, errors.New("K2 payload exceeds 52 bytes")
	}
	inner, outer := reportChecksums(raw)
	if inner != raw[62] || outer != raw[63] {
		return s, errors.New("K2 report checksum mismatch")
	}
	if raw[8] != TelemetryCommand {
		return s, device.ErrIgnoredReport
	}
	if payloadLength < 45 {
		return s, errors.New("K2 telemetry payload shorter than 45 bytes")
	}
	f := func(offset int) float64 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset:])))
	}
	ah, wh, dp, dn := f(14), f(18), f(30), f(34)
	rawCurrent := f(50)
	recordSeconds, uptime := binary.LittleEndian.Uint32(raw[22:]), binary.LittleEndian.Uint32(raw[26:])
	group := uint16(raw[54]) + 1
	recordMinCurrent := float64(binary.LittleEndian.Uint16(raw[12:14])) / 1000
	s = device.Sample{
		Timestamp: time.Now().UTC(), AmpHours: &ah, WattHours: &wh,
		RecordSeconds: &recordSeconds, Uptime: &uptime, RecordMinCurrent: &recordMinCurrent,
		DP: &dp, DN: &dn, Voltage: f(46), Current: math.Abs(rawCurrent), Group: &group,
	}
	payloadEnd := 10 + payloadLength
	if payloadEnd > 55 {
		cc1 := float64(raw[55]) / 10
		s.CC1 = &cc1
	}
	if payloadEnd > 56 {
		cc2 := float64(raw[56]) / 10
		s.CC2 = &cc2
	}
	for _, v := range []float64{s.Voltage, rawCurrent, ah, wh, dp, dn} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return device.Sample{}, errors.New("non-finite K2 measurement")
		}
	}
	if s.Voltage < 0 || s.Voltage > 100 || s.Current > 100 {
		return device.Sample{}, errors.New("K2 voltage or current out of range")
	}
	switch {
	case rawCurrent > 0:
		s.CurrentDirection = device.CurrentPlugToReceptacle
	case rawCurrent < 0:
		s.CurrentDirection = device.CurrentReceptacleToPlug
	}
	s.Power = s.Voltage * s.Current
	s.SignedPower = s.Voltage * rawCurrent
	temperature := func(v float64, external bool) *float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -100 || v > 200 || (external && v <= 0) {
			return nil
		}
		return &v
	}
	s.Temperature = temperature(f(38), false)
	s.ExternalTemperature = temperature(f(42), true)
	return s, nil
}
