package tui

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/locales"
)

type channel struct {
	name        func(locales.Messages) string
	short, unit string
	decimals    int
	tone        lipgloss.TerminalColor
	read        func(device.Sample) (float64, bool)
}

const (
	powerMetric   = 0
	currentMetric = 1
)

var channels = []channel{
	powerMetric:   {locales.Messages.Power, "W", "W", 4, warning, func(s device.Sample) (float64, bool) { return s.Power, true }},
	currentMetric: {locales.Messages.Current, "A", "A", 4, positive, func(s device.Sample) (float64, bool) { return math.Abs(s.Current), true }},
	{locales.Messages.Voltage, "V", "V", 4, accent, func(s device.Sample) (float64, bool) { return s.Voltage, true }},
	{locales.Messages.Temperature, "°C", "°C", 1, purple, func(s device.Sample) (float64, bool) { return value(s.Temperature) }},
	{locales.Messages.Dp, "D+", "V", 3, accent, func(s device.Sample) (float64, bool) { return value(s.DP) }},
	{locales.Messages.Dn, "D−", "V", 3, accent, func(s device.Sample) (float64, bool) { return value(s.DN) }},
	{locales.Messages.ExternalTemperature, "NTC", "°C", 1, purple, func(s device.Sample) (float64, bool) { return value(s.ExternalTemperature) }},
}

func value(v *float64) (float64, bool) {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return 0, false
	}
	return *v, true
}

func metric(s device.Sample, i int) (float64, bool) { return channels[i].read(s) }

func historyWindow(samples []device.Sample, window time.Duration) []device.Sample {
	if len(samples) == 0 {
		return samples
	}
	start := samples[len(samples)-1].Elapsed - window.Seconds()
	i := sort.Search(len(samples), func(i int) bool { return samples[i].Elapsed >= start })
	return samples[i:]
}
func number(v float64, c channel) string {
	return fmt.Sprintf("%.*f", c.decimals, v)
}

type rangeValues struct {
	min, max, mean float64
	count          int
}

func visibleRange(samples []device.Sample, index int, start float64) rangeValues {
	var r rangeValues
	for _, s := range samples {
		if s.Elapsed < start {
			continue
		}
		v, ok := metric(s, index)
		if !ok {
			continue
		}
		r.count++
		if r.count == 1 {
			r.min, r.max, r.mean = v, v, v
		} else {
			r.min = min(r.min, v)
			r.max = max(r.max, v)
			r.mean += (v - r.mean) / float64(r.count)
		}
	}
	return r
}

func sparkline(samples []device.Sample, index, width int, msgs locales.Messages) string {
	if len(samples) == 0 || width < 1 {
		return ""
	}
	samples = samples[max(0, len(samples)-width*4):]
	r := visibleRange(samples, index, -1)
	if r.count == 0 {
		return msgs.NotReported()
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, width)
	for x := range out {
		lo, hi := x*len(samples)/width, (x+1)*len(samples)/width
		if hi <= lo {
			out[x] = ' '
			continue
		}
		var v float64
		n := 0
		for _, s := range samples[lo:hi] {
			if f, ok := metric(s, index); ok {
				v += f
				n++
			}
		}
		if n == 0 {
			out[x] = ' '
			continue
		}
		v /= float64(n)
		level := 3
		if r.max > r.min {
			level = int(math.Round((v - r.min) / (r.max - r.min) * 7))
		}
		out[x] = bars[max(0, min(7, level))]
	}
	return string(out)
}
