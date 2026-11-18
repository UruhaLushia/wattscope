package backend

import (
	"math"

	"github.com/UruhaLushia/wattscope/internal/device"
)

type Range struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
}

func (r *Range) add(v float64, n uint64) {
	if n == 1 {
		r.Min, r.Max, r.Mean = v, v, v
		return
	}
	r.Min = min(r.Min, v)
	r.Max = max(r.Max, v)
	r.Mean += (v - r.Mean) / float64(n)
}

// Statistics are host-session values, separate from on-device totals.
// Integrals use trapezoids and never span a disconnect, group change or reboot.
type Statistics struct {
	Samples          uint64  `json:"samples"`
	Voltage          Range   `json:"voltage_v"`
	Current          Range   `json:"current_a"`
	Power            Range   `json:"power_w"`
	MeasuredSeconds  float64 `json:"measured_s"`
	AmpHours         float64 `json:"net_ah"`
	AbsoluteAmpHours float64 `json:"absolute_ah"`
	WattHours        float64 `json:"energy_wh"`
	NetWattHours     float64 `json:"net_wh"`
}

func (s *Statistics) add(v device.Sample, prev *device.Sample, maxGap float64) {
	s.Samples++
	s.Voltage.add(v.Voltage, s.Samples)
	s.Current.add(math.Abs(v.Current), s.Samples)
	s.Power.add(v.Power, s.Samples)
	if prev == nil || prev.Segment != v.Segment {
		return
	}
	dt := v.Elapsed - prev.Elapsed
	if dt <= 0 || dt > maxGap {
		return
	}
	s.MeasuredSeconds += dt
	s.AmpHours += (math.Abs(prev.Current) + math.Abs(v.Current)) * .5 * dt / 3600
	s.AbsoluteAmpHours += (math.Abs(prev.Current) + math.Abs(v.Current)) * .5 * dt / 3600
	s.WattHours += (prev.Power + v.Power) * .5 * dt / 3600
	s.NetWattHours += (prev.SignedPower + v.SignedPower) * .5 * dt / 3600
}
