package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/device"
)

const chartAxisWidth = 10

func chartPlotColumns(width int) int { return max(2, width-chartAxisWidth) }
func chartPlotHeight(height int) int { return max(2, height-5) }

// plot maps real elapsed time onto a fixed window. Braille gives two horizontal
// and four vertical pixels per terminal cell without altering measured values.
func plot(samples []device.Sample, width, height int, v View) []string {
	msgs := v.messages()
	height = max(2, height)
	columns := chartPlotColumns(width)
	end := 0.0
	if len(samples) > 0 {
		end = samples[len(samples)-1].Elapsed
	}
	start := end - v.Window.Seconds()
	r := visibleRange(samples, v.Metric, start)
	c := channels[v.Metric]
	if r.count == 0 {
		rows := make([]string, height)
		for i := range rows {
			rows[i] = strings.Repeat(" ", max(0, width))
		}
		message := msgs.WaitingMeasurements()
		if len(samples) > 0 {
			message = msgs.ChannelUnreported()
		}
		rows[height/2] = quietStyle.Render(fit("  "+message, width))
		return append(rows, timeAxis(columns, chartAxisWidth, v))
	}
	lo, hi := r.min, r.max
	margin := max((hi-lo)*.12, math.Max(math.Abs(lo)*.0002, .0001))
	lo -= margin
	hi += margin
	pw, ph := columns*2, height*4
	cells := make([][]rune, height)
	for y := range cells {
		cells[y] = make([]rune, columns)
	}
	put := func(x, y int) {
		if x < 0 || x >= pw || y < 0 || y >= ph {
			return
		}
		bits := [4][2]uint{{0, 3}, {1, 4}, {2, 5}, {6, 7}}
		cells[y/4][x/2] |= rune(1 << bits[y%4][x%2])
	}
	point := func(s device.Sample, n float64) (int, int) {
		x := int(math.Round((s.Elapsed - start) / v.Window.Seconds() * float64(pw-1)))
		y := int(math.Round((hi - n) / (hi - lo) * float64(ph-1)))
		return max(0, min(pw-1, x)), max(0, min(ph-1, y))
	}
	lastX, lastY := -1, 0
	var segment uint64
	for _, s := range samples {
		if s.Elapsed < start {
			continue
		}
		n, ok := metric(s, v.Metric)
		if !ok {
			lastX = -1
			continue
		}
		x, y := point(s, n)
		if lastX >= 0 && segment == s.Segment {
			steps := max(abs(x-lastX), abs(y-lastY))
			for step := 0; step < steps; step++ {
				put(lastX+(x-lastX)*step/steps, lastY+(y-lastY)*step/steps)
			}
		}
		put(x, y)
		lastX, lastY, segment = x, y, s.Segment
	}
	cursor := -1
	if v.Paused && len(samples) > 0 {
		selected := samples[max(0, len(samples)-1-v.Cursor)]
		if n, ok := metric(selected, v.Metric); ok {
			x, _ := point(selected, n)
			cursor = x / 2
		}
	}
	rows := make([]string, 0, height+1)
	for y, line := range cells {
		label := hi - (hi-lo)*float64(y)/float64(height-1)
		var row strings.Builder
		row.WriteString(quietStyle.Render(axisLabel(label, c.decimals) + " │"))
		for x, mask := range line {
			switch {
			case mask != 0:
				row.WriteString(color(string(rune(0x2800)+mask), c.tone))
			case x == cursor:
				row.WriteString(quietStyle.Render("┊"))
			case y == 0 || y == height-1 || y == height/2:
				row.WriteString(borderStyle.Render("┄"))
			case x%max(1, columns/4) == 0:
				row.WriteString(borderStyle.Render("┊"))
			default:
				row.WriteByte(' ')
			}
		}
		rows = append(rows, row.String())
	}
	return append(rows, timeAxis(columns, chartAxisWidth, v))
}

func timeAxis(columns, axisWidth int, v View) string {
	left := "−" + span(v.Window)
	center := "−" + span(v.Window/2)
	right := v.messages().AxisNow()
	if v.Paused {
		right = v.messages().AxisHold()
	}
	axis := fit(left, columns)
	centerAt := max(0, (columns-ansi.StringWidth(center))/2)
	axis = ansi.Cut(axis, 0, centerAt) + fit(center, columns-centerAt)
	rightAt := max(0, columns-ansi.StringWidth(right))
	axis = ansi.Cut(axis, 0, rightAt) + fit(right, columns-rightAt)
	return strings.Repeat(" ", axisWidth) + quietStyle.Render(axis)
}

func axisLabel(v float64, decimals int) string {
	for p := decimals; p >= 0; p-- {
		s := fmt.Sprintf("%.*f", p, v)
		if len(s) <= 8 {
			return fmt.Sprintf("%8s", s)
		}
	}
	return fmt.Sprintf("%8.2g", v)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
