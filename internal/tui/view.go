package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/locales"
)

func compactScreen(width, height int) bool {
	return width < 40 || height < 12
}

// Render keeps acquisition state separate from a held/reviewed measurement.
func Render(s backend.Snapshot, width, height int, v View) string {
	v.Locale = locales.Resolve(v.Locale)
	v.RecordingActive = s.Recording.Active
	v.CanCheckPDHost = s.CanCheckPDHost
	if v.Page < 0 || v.Page >= pageCount {
		v.Page = PageOverview
	}
	width = max(1, width)
	height = max(1, height)
	v.Metric = (v.Metric%len(channels) + len(channels)) % len(channels)
	if v.Window <= 0 {
		v.Window = time.Minute
	}
	s.History = historyWindow(s.History, v.Window)
	v.Cursor = max(0, min(v.Cursor, len(s.History)-1))
	if v.Now.IsZero() {
		v.Now = time.Now()
	}
	var rows []string
	if compactScreen(width, height) {
		if v.Page == PagePD {
			rows = packetsPage(s, width, height, v)
			if v.Message != "" {
				rows[packetHeaderRows-1] = color(fit(safe(v.Message), width), accent)
			}
		} else {
			rows = compact(s, width, height, v)
		}
	} else {
		rows = workspace(s, width, height, v)
	}
	if v.Help {
		rows = overlay(rows, help(width, v.messages()), v.messages().KeyboardShortcuts(), width, height, v.Scroll, v.messages())
	}
	if v.Details {
		rows = overlay(rows, details(s, v, width), v.messages().DeviceStorage(), width, height, v.Scroll, v.messages())
	}
	return screen(rows, width, height)
}

func connectionStatus(s backend.Snapshot, msgs locales.Messages) (string, lipgloss.TerminalColor) {
	if s.ErrorKind == "permission" {
		return msgs.AccessRequired(), warning
	}
	if s.State == backend.Streaming {
		if s.Device.Simulated {
			return msgs.DemoStatus(), warning
		}
		return msgs.Live(), positive
	}
	switch s.State {
	case backend.Connecting:
		return msgs.Connecting(), accent
	case backend.Waiting:
		if s.Connections > 0 {
			return msgs.Reconnecting(), warning
		}
		return msgs.Waiting(), warning
	default:
		return msgs.Stopped(), muted
	}
}

const (
	overviewMetricCount = 4
	metricCardHeight    = 5
	dashboardHeaderRows = 3
	dashboardChartRow   = dashboardHeaderRows + metricCardHeight + 1
)

func metricCardWidths(width int) [overviewMetricCount]int {
	var widths [overviewMetricCount]int
	remaining := width - len(widths) + 1
	for index := range widths {
		widths[index] = remaining / (len(widths) - index)
		remaining -= widths[index]
	}
	return widths
}

func dashboardChartSize(width, height int) (chartWidth, chartHeight, sideWidth, recordHeight int) {
	recordHeight = 1
	if height >= 30 {
		recordHeight = 4
	}
	chartWidth = width
	chartHeight = height - dashboardChartRow - recordHeight - 4
	if width >= 108 && chartHeight >= 10 {
		sideWidth = min(34, width/4)
		chartWidth -= sideWidth + 1
	}
	return
}

func compactChartHeight(height int) int {
	return height - overviewMetricCount - 4
}

func dashboard(s backend.Snapshot, w, h int, v View) []string {
	msgs := v.messages()
	state, tone := connectionStatus(s, msgs)
	badge := strong("● "+state, tone)
	if v.Paused {
		badge += "  " + strong("Ⅱ "+msgs.Hold(), accent)
	}
	rows := []string{apart(strong("WattScope", accent)+quietStyle.Render("  /  "+msgs.Workspace()), badge, w)}
	name := safe(s.Device.Name)
	if s.Device.Simulated {
		name = msgs.SimulatedMeter()
	}
	if name == "" {
		name = msgs.AutoDetecting()
	}
	meta := name
	if s.Device.Transport != "" {
		meta += "  ·  " + strings.ToUpper(s.Device.Transport)
	}
	if s.Device.Serial != "" {
		meta += "  ·  " + safe(s.Device.Serial)
	}
	rows = append(rows, apart(quietStyle.Render(meta), quietStyle.Render(v.Now.Local().Format("15:04:05")), w), "")
	selected := selectedSample(s, v)
	var cards []string
	for i, width := range metricCardWidths(w) {
		card := metricCard(s, selected, i, width, v)
		if i == 0 {
			cards = card
		} else {
			cards = beside(cards, card, 1)
		}
	}
	rows = append(rows, cards...)
	rows = append(rows, "")
	chartWidth, chartHeight, sideWidth, recordHeight := dashboardChartSize(w, h)
	chart := chartPanel(s, chartWidth, chartHeight, v)
	if sideWidth > 0 {
		signalHeight := chartHeight / 2
		side := append(signalPanel(s, sideWidth, signalHeight, v), sessionPanel(s, sideWidth, chartHeight-signalHeight, msgs)...)
		rows = append(rows, beside(chart, side, 1)...)
	} else {
		rows = append(rows, chart...)
	}
	rows = append(rows, "")
	if recordHeight > 1 {
		rows = append(rows, recordPanel(s, w, recordHeight, v)...)
	} else {
		rows = append(rows, recordLine(s, w, v))
	}
	rows = append(rows, statusLine(s, w, v))
	rows = append(rows, quietStyle.Render(fit(msgs.DashboardKeys(), w)))
	stats := msgs.HostTotals(fmt.Sprintf("%.4f", s.Statistics.WattHours), fmt.Sprintf("%.3f", s.Statistics.AbsoluteAmpHours*1000))
	rows = append(rows, apart(quietStyle.Render(stats), quietStyle.Render(msgs.DetailsClearKeys()), w))
	return rows
}

func selectedSample(s backend.Snapshot, v View) *device.Sample {
	if v.Paused && len(s.History) > 0 {
		i := max(0, len(s.History)-1-v.Cursor)
		sample := s.History[i]
		return &sample
	}
	return s.Latest
}

func metricCard(s backend.Snapshot, sample *device.Sample, index, width int, v View) []string {
	msgs := v.messages()
	c := channels[index]
	numberText := "—"
	reported := false
	if sample != nil {
		if n, ok := metric(*sample, index); ok {
			numberText = number(n, c)
			reported = true
		}
	}
	tone := lipgloss.TerminalColor(border)
	title := fmt.Sprintf("%d %s", index+1, c.name(msgs))
	if index == v.Metric {
		tone = c.tone
		title = "▸ " + title
	}
	values := apart(strong(numberText, primary), color(c.unit, c.tone), width-4)
	trend := color(sparkline(s.History, index, width-4, msgs), c.tone)
	if !reported {
		if sample != nil {
			trend = quietStyle.Render(msgs.NotReported())
		} else {
			trend = quietStyle.Render(msgs.NotAvailable())
		}
	} else if s.State != backend.Streaming {
		trend = quietStyle.Render(msgs.LastMeasurement())
	}
	subtitle := ""
	if reported && s.State == backend.Streaming && len(s.History) > 1 {
		end := s.History[len(s.History)-1].Elapsed
		r := visibleRange(s.History, index, end-v.Window.Seconds())
		if r.count > 1 {
			spanText := fmt.Sprintf("%.*f–%.*f", min(2, c.decimals), r.min, min(2, c.decimals), r.max)
			if ansi.StringWidth(spanText) <= width-4 {
				subtitle = spanText
			}
		}
	}
	content := []string{values, trend, quietStyle.Render(subtitle)}
	return panel(strong(title, c.tone), "", width, metricCardHeight, content, tone)
}

const chartTabGap = "  "

func chartTabs(v View) []string {
	var tabs []string
	for i, ch := range channels {
		label := fmt.Sprintf("%d %s", i+1, ch.short)
		if i == v.Metric {
			tabs = append(tabs, activeStyle.Foreground(ch.tone).Render(label))
		} else {
			tabs = append(tabs, quietStyle.Render(label))
		}
	}
	return tabs
}

func chartPanel(s backend.Snapshot, w, h int, v View) []string {
	msgs := v.messages()
	c := channels[v.Metric]
	content := []string{strings.Join(chartTabs(v), chartTabGap)}
	content = append(content, plot(s.History, w-4, chartPlotHeight(h), v)...)
	readout := quietStyle.Render(msgs.NoSamples())
	if selected := selectedSample(s, v); selected != nil {
		var val string
		if n, ok := metric(*selected, v.Metric); ok {
			val = strong(number(n, c)+" "+c.unit, c.tone)
		} else {
			val = quietStyle.Render(msgs.NotReported())
		}
		mode := msgs.Latest()
		if v.Paused {
			mode = msgs.Inspect()
		}
		readout = quietStyle.Render(selected.Timestamp.Local().Format("15:04:05.000")+"  ·  ") + val + quietStyle.Render("  ·  "+mode)
	}
	content = append(content, readout)
	return panel(strong(msgs.HistoryTitle(c.name(msgs)), c.tone), span(v.Window), w, h, content, border)
}

func signalPanel(s backend.Snapshot, w, h int, v View) []string {
	msgs := v.messages()
	c := channels[v.Metric]
	end := 0.0
	if len(s.History) > 0 {
		end = s.History[len(s.History)-1].Elapsed
	}
	r := visibleRange(s.History, v.Metric, end-v.Window.Seconds())
	minText, meanText, maxText := "—", "—", "—"
	if r.count > 0 {
		minText = number(r.min, c)
		meanText = number(r.mean, c)
		maxText = number(r.max, c)
	}
	return panel(strong(msgs.VisibleRange(), primary), c.unit, w, h, []string{
		apart(quietStyle.Render(msgs.Minimum()), minText, w-4), apart(quietStyle.Render(msgs.Average()), meanText, w-4), apart(quietStyle.Render(msgs.Maximum()), maxText, w-4),
		apart(quietStyle.Render(msgs.Samples()), fmt.Sprint(r.count), w-4),
	}, border)
}

func sessionPanel(s backend.Snapshot, w, h int, msgs locales.Messages) []string {
	stats := s.Statistics
	return panel(strong(msgs.HostSession(), primary), "", w, h, []string{
		apart(quietStyle.Render(msgs.Measured()), duration(stats.MeasuredSeconds), w-4),
		apart(quietStyle.Render(msgs.Energy()), fmt.Sprintf("%.4f Wh", stats.WattHours), w-4),
		apart(quietStyle.Render(msgs.ChargeThroughput()), fmt.Sprintf("%.3f mAh", stats.AbsoluteAmpHours*1000), w-4),
		apart(quietStyle.Render(msgs.Samples()), fmt.Sprint(stats.Samples), w-4),
		apart(quietStyle.Render(msgs.Connections()), fmt.Sprint(s.Connections), w-4),
		apart(quietStyle.Render(msgs.BadReports()), fmt.Sprint(s.InvalidReports), w-4),
	}, border)
}

func recordPanel(s backend.Snapshot, w, h int, v View) []string {
	msgs := v.messages()
	title, badge := msgs.Recording(), msgs.StartRecordingKey()
	tone := lipgloss.TerminalColor(border)
	path := v.RecordingDir
	if s.Recording.Path != "" {
		path = s.Recording.Path
	}
	if s.Recording.Active {
		title = strong("● "+msgs.Recording(), danger)
		badge = msgs.RecordingRowsStop(int(s.Recording.Samples))
		tone = danger
	} else if s.Recording.Path != "" {
		title = strong("✓ "+msgs.RecordingSaved(), positive)
		badge = msgs.Rows(int(s.Recording.Samples))
	}
	label := msgs.Destination()
	if s.Recording.Path != "" {
		label = msgs.File()
	}
	content := []string{quietStyle.Render(label+"  ") + middle(safe(path), w-4-ansi.StringWidth(label)-2)}
	content = append(content, quietStyle.Render(msgs.RecordingNote(fmt.Sprint(s.Statistics.Samples))))
	return panel(title, badge, w, h, content, tone)
}

func recordLine(s backend.Snapshot, w int, v View) string {
	msgs := v.messages()
	label := quietStyle.Render(msgs.CsvOff())
	path := v.RecordingDir
	if s.Recording.Path != "" {
		path = s.Recording.Path
	}
	if s.Recording.Active {
		label = strong(msgs.RecordingRows(int(s.Recording.Samples)), danger)
	} else if s.Recording.Path != "" {
		label = color(msgs.SavedRows(int(s.Recording.Samples)), positive)
	}
	return fit(label+"  "+quietStyle.Render(middle(safe(path), max(0, w-ansi.StringWidth(label)-2))), w)
}

func statusLine(s backend.Snapshot, w int, v View) string {
	msgs := v.messages()
	message := v.Message
	if s.Error != "" {
		switch s.ErrorKind {
		case "permission":
			message = msgs.PermissionMessage()
		case "missing":
			message = msgs.MissingMessage()
		case "selection":
			message = msgs.SelectionMessage()
		case "timeout":
			message = msgs.TimeoutMessage()
		case "recording":
			message = msgs.RecordingMessage()
		case "report":
			message = msgs.ReportMessage()
		default:
			message = msgs.ConnectionMessage()
		}
		return color(fit(safe(message), w), warning)
	}
	if message != "" {
		return color(fit(safe(message), w), accent)
	}
	fresh := msgs.WaitingFirst()
	lastReceived := v.LastReceived
	if lastReceived.IsZero() && s.Latest != nil {
		lastReceived = s.Latest.Timestamp
	}
	if !lastReceived.IsZero() {
		age := v.Now.Sub(lastReceived)
		fresh = msgs.LastReceived(duration(max(0, age.Seconds())))
		if age < time.Second {
			fresh = msgs.Receiving()
		}
	}
	quality := msgs.Quality(fmt.Sprint(s.Statistics.Samples), fmt.Sprint(s.InvalidReports))
	return apart(quietStyle.Render(fresh), quietStyle.Render(quality), w)
}

func compact(s backend.Snapshot, w, h int, v View) []string {
	msgs := v.messages()
	state, tone := connectionStatus(s, msgs)
	rows := []string{apart(strong("WattScope", accent), color(state, tone), w)}
	sample := selectedSample(s, v)
	for i := range overviewMetricCount {
		c := channels[i]
		label := fmt.Sprintf("%d %s", i+1, c.name(msgs))
		value := "—"
		if sample != nil {
			if n, ok := metric(*sample, i); ok {
				value = number(n, c)
			}
		}
		styledLabel := color(label, c.tone)
		if i == v.Metric {
			styledLabel = strong("▸ "+label, c.tone)
		}
		rows = append(rows, apart(styledLabel, strong(value+" "+c.unit, primary), w))
	}
	available := compactChartHeight(h)
	if available >= 5 {
		rows = append(rows, chartPanel(s, w, available, v)...)
	}
	rows = append(rows, recordLine(s, w, v), statusLine(s, w, v), quietStyle.Render(fit(msgs.CompactKeys(), w)))
	return rows
}

func overlay(base, content []string, title string, w, h, scroll int, msgs locales.Messages) []string {
	width := min(76, max(4, w-4))
	height := min(h, max(4, len(content)+2))
	scroll = max(0, min(scroll, len(content)-max(0, height-2)))
	box := panel(strong(title, accent), msgs.OverlayKeys(), width, height, content[scroll:], accent)
	left, top := max(0, (w-width)/2), max(0, (h-height)/2)
	for len(base) < h {
		base = append(base, strings.Repeat(" ", w))
	}
	for i, line := range box {
		if top+i >= h {
			break
		}
		base[top+i] = ansi.Cut(fit(base[top+i], w), 0, left) + line + ansi.Cut(fit(base[top+i], w), left+width, w)
	}
	return base
}

func help(width int, msgs locales.Messages) []string {
	inner := max(1, min(76, max(4, width-4))-4)
	keyWidth := min(18, max(8, inner/3))
	var rows []string
	section := func(heading string) {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, strong(heading, accent))
	}
	item := func(keys, desc string) {
		if inner < 30 {
			rows = append(rows, color(keys, primary))
			rows = append(rows, strings.Split(ansi.Wrap(quietStyle.Render(desc), inner, ""), "\n")...)
			return
		}
		descWidth := max(1, inner-keyWidth-2)
		wrapped := strings.Split(ansi.Wrap(desc, descWidth, ""), "\n")
		for i, line := range wrapped {
			prefix := strings.Repeat(" ", keyWidth+2)
			if i == 0 {
				prefix = color(fit(keys, keyWidth), primary) + "  "
			}
			rows = append(rows, prefix+quietStyle.Render(line))
		}
	}
	section(msgs.NavigationHeading())
	item("F1–F4", msgs.HelpPagesDesc())
	item("Tab / Shift+Tab", msgs.HelpFocusDesc())

	section(msgs.MeasurementsHeading())
	item("1–7 / [ ] / g", msgs.HelpChannelsDesc())
	item("← / →", msgs.HelpInspectDesc())
	item("Shift+← / →", msgs.HelpStepDesc())
	item("Home / End", msgs.HelpLiveDesc())
	item(msgs.HelpZoomKey(), msgs.HelpZoomDesc())
	item("Space", msgs.HelpHoldDesc())

	section(msgs.PdHeading())
	item("↑↓ / PgUp / PgDn", msgs.HelpPdSelectDesc())
	item("Enter", msgs.HelpPdExpandDesc())
	item("/ · a · k", msgs.HelpPdFilterDesc())
	item("r · p · e", msgs.HelpPdCaptureDesc())
	item("Space · End", msgs.HelpPdPauseDesc())

	section(msgs.SessionHeading())
	item("r / p", msgs.HelpRecordDesc())
	item("c", msgs.HelpClearDesc())
	item("d", msgs.HelpDetailsDesc())
	item("? / Esc", msgs.HelpHelpDesc())
	item("q / Ctrl+C", msgs.HelpQuitDesc())

	return rows
}

func details(s backend.Snapshot, v View, w int) []string {
	msgs := v.messages()
	width := max(1, min(76, max(4, w-4))-4)
	var rows []string
	add := func(label, text string) {
		if text == "" {
			text = "—"
		}
		rows = append(rows, quietStyle.Render(label))
		rows = append(rows, strings.Split(ansi.Hardwrap(safe(text), width, true), "\n")...)
	}
	name := s.Device.Name
	if s.Device.Simulated {
		name = msgs.SimulatedMeter()
	}
	add(msgs.DeviceHeading(), name)
	add(msgs.DriverTransport(), strings.Trim(s.Device.Driver+" / "+s.Device.Transport, " /"))
	add(msgs.Identity(), s.Device.ID)
	if s.Device.Serial != "" {
		add(msgs.SerialHeading(), s.Device.Serial)
	}
	add(msgs.Interface(), s.Device.Path)
	pdSupport := msgs.PdNotAdvertised()
	if s.Device.Supports(device.CapPDCapture) {
		pdSupport = msgs.PdSupported()
	}
	add(msgs.PdCaptureLabel(), pdSupport)

	rows = append(rows, "", strong(msgs.StorageHeading(), accent))
	add(msgs.RecordingsDirectory(), v.RecordingDir)
	if v.SourcePath != "" {
		add(msgs.SourceFile(), v.SourcePath)
	}

	if s.Latest != nil {
		var telemetry []string
		var ccReadings []string
		if s.Latest.CC1 != nil {
			ccReadings = append(ccReadings, fmt.Sprintf("CC1 %.1f V", *s.Latest.CC1))
		}
		if s.Latest.CC2 != nil {
			ccReadings = append(ccReadings, fmt.Sprintf("CC2 %.1f V", *s.Latest.CC2))
		}
		if len(ccReadings) > 0 {
			telemetry = append(telemetry, strings.Join(ccReadings, " · "))
		}
		if s.Latest.RecordMinCurrent != nil {
			telemetry = append(telemetry, msgs.DeviceRecordMinimumCurrent(fmt.Sprintf("%.0f", *s.Latest.RecordMinCurrent*1000)))
		}
		if s.Latest.AmpHours != nil {
			telemetry = append(telemetry, msgs.DeviceCharge(fmt.Sprintf("%.6f", *s.Latest.AmpHours)))
		}
		if s.Latest.WattHours != nil {
			telemetry = append(telemetry, msgs.DeviceEnergy(fmt.Sprintf("%.6f", *s.Latest.WattHours)))
		}
		if s.Latest.Group != nil {
			telemetry = append(telemetry, msgs.DeviceGroup(fmt.Sprint(*s.Latest.Group)))
		}
		if s.Latest.Uptime != nil {
			telemetry = append(telemetry, msgs.DeviceUptime(duration(float64(*s.Latest.Uptime))))
		}
		if s.Latest.RecordSeconds != nil {
			telemetry = append(telemetry, msgs.DeviceTimer(duration(float64(*s.Latest.RecordSeconds))))
		}
		if len(telemetry) > 0 {
			rows = append(rows, "", strong(msgs.MeterTelemetryHeading(), accent))
			rows = append(rows, telemetry...)
		}
	}
	if s.Error != "" {
		rows = append(rows, "")
		add(msgs.Detail(), s.Error)
	}
	return rows
}
