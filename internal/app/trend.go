package app

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

type measurement struct {
	name     func(locales.Messages) string
	unit     string
	decimals int
	read     func(*device.Sample) (float64, bool)
	light    ui.Color
	dark     ui.Color
}

var (
	chartChannelOrder = [...]int{0, 1, 2, 3, 6, 4, 5}
	channels          = [...]measurement{
		{locales.Messages.Power, "W", 4, func(s *device.Sample) (float64, bool) { return s.Power, true }, ui.Hex("#3c718d"), ui.Hex("#77bbdd")},
		{locales.Messages.Voltage, "V", 4, func(s *device.Sample) (float64, bool) { return s.Voltage, true }, ui.Hex("#745b91"), ui.Hex("#baa3d6")},
		{locales.Messages.Current, "A", 4, func(s *device.Sample) (float64, bool) { return s.Current, true }, ui.Hex("#3a7946"), ui.Hex("#79c07d")},
		{locales.Messages.Temperature, "°C", 1, func(s *device.Sample) (float64, bool) { return optional(s.Temperature) }, ui.Hex("#96601e"), ui.Hex("#e4b665")},
		{locales.Messages.Dp, "V", 3, func(s *device.Sample) (float64, bool) { return optional(s.DP) }, ui.Hex("#2f6f9f"), ui.Hex("#6db3e8")},
		{locales.Messages.Dn, "V", 3, func(s *device.Sample) (float64, bool) { return optional(s.DN) }, ui.Hex("#745b91"), ui.Hex("#baa3d6")},
		{locales.Messages.ExternalTemperature, "°C", 1, func(s *device.Sample) (float64, bool) { return optional(s.ExternalTemperature) }, ui.Hex("#b85c38"), ui.Hex("#eb946e")},
	}
)

func optional(value *float64) (float64, bool) {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return 0, false
	}
	return *value, true
}

func (m measurement) color(p kit.Palette) ui.Color {
	if p.Dark {
		return m.dark
	}
	return m.light
}

func (m measurement) format(sample *device.Sample) string {
	if sample != nil {
		if value, ok := m.read(sample); ok {
			return fmt.Sprintf("%.*f", m.decimals, value)
		}
	}
	return "—"
}

const (
	chartDragNone uint8 = iota
	chartDragPending
	chartDragPan
)

func formatWindowLabel(seconds float64) string {
	if seconds >= 9.95 && seconds < 60 {
		return fmt.Sprintf("%ds", int(math.Round(seconds)))
	}
	return formatChartTime(seconds)
}

func formatRelTime(offset float64) string {
	if math.Abs(offset) < 0.05 {
		return "0s"
	}
	if offset < 0 {
		return "−" + formatChartTime(-offset)
	}
	return "+" + formatChartTime(offset)
}

func formatChartTime(seconds float64) string {
	if seconds >= 3600 {
		total := int(math.Round(seconds))
		hours, mins := total/3600, (total%3600)/60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%02dm", hours, mins)
	}
	if seconds >= 60 {
		total := int(math.Round(seconds))
		mins, rem := total/60, total%60
		if rem == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%02ds", mins, rem)
	}
	if math.Abs(seconds-math.Round(seconds)) < 0.05 {
		return fmt.Sprintf("%ds", int(math.Round(seconds)))
	}
	return fmt.Sprintf("%.1fs", seconds)
}

func (d *dashboard) effectiveSeriesMask() uint8 {
	mask := d.chartSeries
	if mask == 0 {
		mask = 1<<0 | 1<<1 | 1<<2
	}
	return mask | uint8(1<<d.channel)
}

func (d *dashboard) selectChartChannel(index int) {
	if index < 0 || index >= len(channels) {
		return
	}
	d.channel = index
	if d.chartSeries != 0 {
		d.chartSeries |= uint8(1 << index)
	}
}

func (d *dashboard) toggleChartSeries(index int) {
	if index < 0 || index >= len(channels) || index == d.channel {
		return
	}
	mask := d.effectiveSeriesMask()
	bit := uint8(1 << index)
	if mask&bit != 0 {
		mask &^= bit
	} else {
		mask |= bit
	}
	d.chartSeries = mask
}

func hasChannelData(samples []device.Sample, ch measurement) bool {
	n := len(samples)
	if n == 0 {
		return false
	}
	if _, ok := ch.read(&samples[n-1]); ok {
		return true
	}
	if _, ok := ch.read(&samples[0]); ok {
		return true
	}
	step := max(1, n/512)
	for i := n - 1 - step; i > 0; i -= step {
		if _, ok := ch.read(&samples[i]); ok {
			return true
		}
	}
	return false
}

type chartTraceKey int
type overviewPDKey uint64

type overviewFrame struct {
	snap          backend.Snapshot
	live          backend.Snapshot
	chartSample   *device.Sample
	readoutSample *device.Sample
	readoutStats  backend.Statistics
	metricStats   [3]channelStat
	cw            chartWindow
	width         float32
	height        float32
	notice        string
}

func (d *dashboard) clearSession() {
	d.resetChartView()
	if d.service != nil {
		d.service.ResetStatistics()
	}
}

type chartCardOptions struct {
	cardPad, cardBottomPad, cardGap float32
	touch, landscape, grow          bool
	basis, minW, plotHeight         float32
	leftAxis                        bool
}

func holdResumeButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, key string, held, touch bool, onClick func()) ui.Element {
	label, icon, variant := msgs.ActionHold(), kit.IconPause, kit.VariantSecondary
	if held {
		label, icon, variant = msgs.ActionResume(), kit.IconPlay, kit.VariantPrimary
	}
	return kit.ActionButton(ctx.Key(key), p, variant, icon, label, touch, onClick)
}

func (d *dashboard) cachedStaticCard(ctx *ui.Context, p kit.Palette) ui.Element {
	card := kit.StaticCard(ctx, p)
	if !d.snapshotOnly {
		card.Cache()
	}
	return card
}

func (d *dashboard) chartCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, frame overviewFrame, opts chartCardOptions) {
	channel := channels[d.channel]
	seriesMask := d.effectiveSeriesMask()
	fillHeight := opts.plotHeight <= 0
	var c ui.Element
	if opts.grow {
		// Skip FillWidth and slow size transitions on flex-grown chart cards so safe-area or orientation changes never lag past the viewport edge.
		col := ui.Column(ctx).MinWidth(0).Padding(opts.cardPad, opts.cardPad, opts.cardBottomPad, opts.cardPad).Gap(opts.cardGap)
		if fillHeight {
			col.FillHeight().MinHeight(0)
		}
		c = kit.GlassPanel(col, p, kit.RadiusL, p.Surface, p.LineSoft).Grow(1).Basis(opts.basis).MinWidth(opts.minW)
		if opts.landscape && d.mobile && !d.snapshotOnly {
			kit.ApplyRotateEnterPose(ctx, c.TransformOrigin(0.5, 0.5), "rot-ov-chart", ui.Pose{X: 12, Scale: 0.965, Fade: 0.55}, kit.MotionRotate)
		}
	} else {
		c = kit.Card(ctx, p).Padding(opts.cardPad, opts.cardPad, opts.cardBottomPad, opts.cardPad).Gap(opts.cardGap)
	}
	c.Children(func() {
		titleSize, hdrMinH, badgeH, btnH := float32(14), float32(0), float32(24), float32(28)
		if d.mobile {
			btnH = 32
		}
		if opts.landscape {
			titleSize, hdrMinH, badgeH, btnH = 13.5, 26, 22, 26
		}
		kit.ActionBar(ctx).MinHeight(hdrMinH).AlignItems(ui.Center).Gap(8).Shrink(0).Children(func() {
			ui.Row(ctx).Grow(1).MinWidth(100).AlignItems(ui.Center).Gap(8).Children(func() {
				kit.StatusDot(ctx, channel.color(p), 7)
				ui.Text(ctx, channel.name(msgs)).FontSize(titleSize).FontWeight(700).SingleLine()
				if d.held != nil {
					kit.Badge(ctx, p, msgs.Hold(), kit.ToneWarning)
				}
			})

			kit.ValueBadge(ctx, p, formatWindowLabel(frame.cw.windowSec), kit.TonePlain).Height(badgeH)

			detached := d.held != nil || d.hasViewEnd
			holdResumeButton(ctx, p, msgs, "hold", detached, opts.touch, func() {
				if detached {
					d.resumeLive()
				} else {
					d.hold(frame.snap)
					d.hasViewEnd = false
					d.hasCursor = false
				}
			}).Height(btnH).Disabled(!detached && len(frame.snap.History) == 0)
		})

		d.chartLegendRow(ctx, p, msgs, frame.snap.History, seriesMask, opts.landscape)

		empty := msgs.WaitingMeasurements()
		if len(frame.snap.History) != 0 {
			empty = msgs.NotReported()
		} else if frame.notice != "" {
			empty = frame.notice
		}

		pHeight := opts.plotHeight
		if len(frame.snap.History) == 0 && pHeight > 0 {
			pHeight = min(pHeight, 250)
		}
		d.trend(ctx, p, msgs, frame.snap, frame.live, channel, frame.chartSample, frame.cw, pHeight, empty, opts.touch, seriesMask, opts.leftAxis)
	})
}

func (d *dashboard) landscapeOverview(ctx *ui.Context, p kit.Palette, msgs locales.Messages, frame overviewFrame, pdTotal uint64, cardPad, cardBottomPad, cardGap float32, touch, hasBottom bool) {
	leftW := min(float32(268), max(float32(180), frame.width*0.33))
	chartW := frame.width - leftW - 10
	chartMinW := min(float32(260), max(float32(180), chartW))
	ui.Row(ctx).Fill().MinWidth(0).MinHeight(0).Gap(10).AlignItems(ui.Stretch).Children(func() {
		ovLeft := ui.Scroll(ctx.Key("ov-left")).Scrollbars(false).Width(leftW).Shrink(0).FillHeight().MinHeight(0).TrackScroll(&d.ovLeftScroll)
		if d.mobile && !d.snapshotOnly {
			kit.ApplyRotateEnterPose(ctx, ovLeft.TransformOrigin(0.5, 0.5), "rot-ov-left", ui.Pose{X: -12, Scale: 0.975, Fade: 0.55}, kit.MotionRotate)
		}
		ovLeft.Children(func() {
			ui.Column(ctx).FillWidth().Shrink(0).Padding(0, 0, 2, 0).Gap(8).Children(func() {
				d.landscapeMetricsCard(ctx, p, msgs, frame.readoutSample, frame.metricStats)
				if hasBottom {
					d.sessionSummaryCard(ctx, p, msgs, frame.live, frame.readoutStats, sessionCardOptions{
						compact: true, cardW: leftW, touch: touch, showCSV: true, showRec: true, clearKey: "clear-overview", chargePrec: 2,
					})
					if hasLandscapeSignals(frame.readoutSample, pdTotal) {
						d.overviewSignalsCard(ctx, p, msgs, frame.live, frame.readoutSample, pdTotal, true, true).Padding(10, 12).Gap(7)
					}
				}
			})
		})
		d.chartCard(ctx, p, msgs, frame, chartCardOptions{
			cardPad: cardPad, cardBottomPad: cardBottomPad, cardGap: cardGap,
			touch: touch, landscape: true, grow: true, minW: chartMinW, leftAxis: chartW >= 360,
		})
	})
}

func (d *dashboard) measurementPanel(ctx *ui.Context, p kit.Palette, msgs locales.Messages, frame overviewFrame) {
	touch := d.touchUI()
	landscape := d.isLandscape(ctx)
	width, height := frame.width, frame.height
	stacked, split := width < 280, width < 480
	compactStats := width < 680
	fontSize := float32(24)
	if d.mobile {
		fontSize = min(26, max(18, (width-140)/22))
		if split {
			fontSize = min(24, max(19, (width-80)/15))
		}
	}

	if !landscape {
		if d.mobile && split {
			d.mobileMetricsCard(ctx, p, msgs, frame.readoutSample, frame.metricStats, fontSize)
		} else {
			const gap = float32(12)
			readings := ui.Row(ctx).FillWidth().Gap(gap).Role(ui.RoleTabList)
			if split {
				readings.Column().Gap(gap)
			}
			readings.Children(func() {
				metric := func(index int, inline bool) {
					item := channels[index]
					d.metricTab(ctx, p, msgs, item, frame.readoutSample, frame.metricStats[index], item.name(msgs), index, fontSize, inline, compactStats)
				}
				metric(0, split)
				if split && !stacked {
					ui.Row(ctx).FillWidth().Gap(gap).Children(func() {
						metric(1, false)
						metric(2, false)
					})
				} else {
					metric(1, stacked)
					metric(2, stacked)
				}
			})
		}
	}

	cardPad := float32(16)
	cardBottomPad := cardPad
	cardGap := float32(10)
	if d.mobile && split {
		cardBottomPad = 13
		cardGap = 8
	} else if landscape {
		cardPad = 12
		cardBottomPad = 10
		cardGap = 6
	}
	wide2K := width >= 1280 && !d.mobile
	hasBottom := frame.live.State == backend.Streaming || frame.readoutStats.Samples > 0 || frame.live.Recording.Active
	var pdSnap backend.PacketSnapshot
	if hasBottom && d.service != nil {
		if wide2K {
			pdSnap = d.service.PacketSnapshot(0, 6)
		} else {
			pdSnap = d.service.PacketSnapshot(0, 0)
		}
	}
	pdTotal := pdSnap.Total

	if landscape {
		d.landscapeOverview(ctx, p, msgs, frame, pdTotal, cardPad, cardBottomPad, cardGap, touch, hasBottom)
		return
	}

	if wide2K && hasBottom {
		plotHeight := min(650, max(310, height-360))
		ui.Row(ctx).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
			d.chartCard(ctx, p, msgs, frame, chartCardOptions{
				cardPad: cardPad, cardBottomPad: cardBottomPad, cardGap: cardGap,
				touch: touch, grow: true, basis: 680, minW: 560, plotHeight: plotHeight, leftAxis: !split,
			})
			ui.Column(ctx).Width(384).Shrink(0).Gap(12).Children(func() {
				d.sessionSummaryCard(ctx, p, msgs, frame.live, frame.readoutStats, sessionCardOptions{
					touch: touch, showCSV: true, showRec: true, clearKey: "clear-overview", chargePrec: 2,
				})
				d.overviewSignalsCard(ctx, p, msgs, frame.live, frame.readoutSample, pdTotal, true, false)
				d.overviewPDCard(ctx, p, msgs, frame.live, pdSnap)
			})
		})
		return
	}

	plotHeight := min(230, max(168, height-510))
	if height >= 780 {
		plotHeight = min(420, max(220, height-540))
	}
	if d.mobile && split {
		plotHeight = max(160, min(198, height-500))
	}
	d.chartCard(ctx, p, msgs, frame, chartCardOptions{
		cardPad: cardPad, cardBottomPad: cardBottomPad, cardGap: cardGap,
		touch: touch, plotHeight: plotHeight, leftAxis: !split,
	})

	if hasBottom {
		compactSignals := d.mobile || (width >= 652 && width < 960)
		bottomGap := float32(12)
		if d.mobile && split {
			bottomGap = 10
		}
		ui.Row(ctx).FillWidth().Wrap().Gap(bottomGap).AlignItems(ui.Stretch).Children(func() {
			d.sessionSummaryCard(ctx, p, msgs, frame.live, frame.readoutStats, sessionCardOptions{
				touch: touch, showCSV: true, showRec: true, clearKey: "clear-overview", chargePrec: 2,
			}).Grow(1).Basis(300).MinWidth(260)
			d.overviewSignalsCard(ctx, p, msgs, frame.live, frame.readoutSample, pdTotal, compactSignals, false).Grow(1).Basis(300).MinWidth(260)
		})
	}
}

func hasLandscapeSignals(sample *device.Sample, pdTotal uint64) bool {
	if pdTotal > 0 {
		return true
	}
	if sample == nil {
		return false
	}
	if displayedCurrentDirection(sample) != "" {
		return true
	}
	for _, ptr := range [4]*float64{sample.DP, sample.DN, sample.CC1, sample.CC2} {
		if _, ok := optional(ptr); ok {
			return true
		}
	}
	return false
}

type sessionCardOptions struct {
	compact    bool
	cardW      float32
	touch      bool
	showCSV    bool
	showRec    bool
	clearKey   string
	chargePrec int
}

func (d *dashboard) sessionSummaryCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, live backend.Snapshot, stats backend.Statistics, opts sessionCardOptions) ui.Element {
	var card ui.Element
	hdrMinH, hdrGap := float32(28), float32(8)
	if opts.compact {
		card = d.cachedStaticCard(ctx, p).Radius(kit.RadiusL).Padding(8, 11).Gap(5)
		hdrMinH, hdrGap = 24, 5
	} else {
		card = kit.Card(ctx, p).Gap(10)
	}
	return card.Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(hdrMinH).AlignItems(ui.Center).Gap(hdrGap).Children(func() {
			if opts.compact {
				ui.Text(ctx, msgs.GuiSession()).FontSize(13).FontWeight(700).SingleLine().Shrink(0).Role(ui.RoleHeading).Level(2)
				if opts.showRec && live.Recording.Active {
					kit.Badge(ctx, p, msgs.RecBadge(), kit.ToneDanger)
				}
				ui.Spacer(ctx).MinWidth(0)
				if stats.Samples > 0 {
					if opts.cardW < 236 {
						kit.IconActionButton(ctx.Key(opts.clearKey), p, kit.IconTrash, msgs.ActionClear(), 24, false, d.clearSession)
					} else {
						kit.ActionButton(ctx.Key(opts.clearKey), p, kit.VariantGhost, kit.IconTrash, msgs.ActionClear(), opts.touch, d.clearSession).
							Height(24).Padding(0, 6)
					}
				}
				if opts.showCSV {
					d.csvButton(ctx, p, msgs, live.Recording, opts.touch).Height(24).Padding(0, 8)
				}
				return
			}
			kit.SectionTitle(ctx, msgs.GuiSession())
			ui.Spacer(ctx)
			if opts.showRec && live.Recording.Active {
				kit.Badge(ctx, p, msgs.RecBadge(), kit.ToneDanger)
			}
			if opts.showCSV {
				if stats.Samples > 0 {
					kit.ActionButton(ctx.Key(opts.clearKey), p, kit.VariantGhost, kit.IconTrash, msgs.ActionClear(), opts.touch, d.clearSession).
						Height(28).Padding(0, 8)
				}
				d.csvButton(ctx, p, msgs, live.Recording, opts.touch).Height(28)
			} else {
				kit.ActionButton(ctx.Key(opts.clearKey), p, kit.VariantGhost, kit.IconTrash, msgs.ActionClear(), opts.touch, d.clearSession).
					Height(28).Padding(0, 8).Disabled(d.service == nil)
			}
		})
		kit.Divider(ctx, p)
		durVal, durUnit := formatSessionDuration(stats.MeasuredSeconds)
		var items [3]kit.MetricSummaryItem
		if opts.compact {
			items = [3]kit.MetricSummaryItem{
				{Label: msgs.Energy(), Value: formatCompactMetric(stats.WattHours, 3), Unit: "Wh"},
				{Label: msgs.ChargeShort(), Value: formatCompactMetric(stats.AbsoluteAmpHours*1000, opts.chargePrec), Unit: "mAh"},
				{Label: msgs.GuiDuration(), Value: durVal, Unit: durUnit},
			}
		} else {
			items = [3]kit.MetricSummaryItem{
				{Label: msgs.Energy(), Value: fmt.Sprintf("%.4f", stats.WattHours), Unit: "Wh"},
				{Label: msgs.ChargeShort(), Value: fmt.Sprintf("%.*f", opts.chargePrec, stats.AbsoluteAmpHours*1000), Unit: "mAh"},
				{Label: msgs.GuiDuration(), Value: durVal, Unit: durUnit},
			}
		}
		kit.MetricSummaryRow(ctx, p, items[:], opts.compact, !opts.compact)
	})
}

func (d *dashboard) overviewSignalsCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, live backend.Snapshot, sample *device.Sample, pdTotal uint64, compactGrid, staticCard bool) ui.Element {
	dirLabel := displayedCurrentDirectionLabel(msgs, sample)
	card := kit.Card(ctx, p)
	if staticCard {
		card = d.cachedStaticCard(ctx, p)
	}
	return card.Gap(10).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(28).AlignItems(ui.Center).Gap(8).Children(func() {
			kit.SectionTitle(ctx, msgs.GuiSignals())
			if dirLabel != "" {
				kit.Badge(ctx, p, dirLabel, kit.TonePlain)
			}
			ui.Spacer(ctx)
			if pdTotal > 0 {
				kit.ValueButton(ctx.Key("goto-pd"), p, kit.VariantGhost, kit.IconUSB, fmt.Sprintf("PD · %d", pdTotal), false, func() {
					d.goPage(pagePD)
				}).Height(28).Padding(0, 8)
			}
		})
		kit.Divider(ctx, p)

		if sample != nil {
			var items [4]kit.MetricSummaryItem
			n := 0
			for _, index := range [...]int{4, 5} {
				ch := channels[index]
				if _, ok := ch.read(sample); ok {
					items[n] = kit.MetricSummaryItem{Label: ch.name(msgs), Value: ch.format(sample), Unit: ch.unit}
					n++
				}
			}
			for _, line := range [...]struct {
				label string
				ptr   *float64
			}{{"CC1", sample.CC1}, {"CC2", sample.CC2}} {
				if val, ok := optional(line.ptr); ok {
					items[n] = kit.MetricSummaryItem{Label: line.label, Value: fmt.Sprintf("%.3f", val), Unit: "V"}
					n++
				}
			}
			if n == 0 {
				if live.Device.Driver != "" {
					items[n] = kit.MetricSummaryItem{Label: msgs.GuiInfo(), Value: live.Device.Driver}
					n++
				}
				if pdTotal > 0 {
					items[n] = kit.MetricSummaryItem{Label: msgs.PdHeading(), Value: strconv.FormatUint(pdTotal, 10)}
					n++
				}
			}
			switch {
			case compactGrid && n > 2:
				ui.Column(ctx).FillWidth().Gap(10).Children(func() {
					kit.MetricSummaryRow(ctx, p, items[:2], false, false)
					kit.MetricSummaryRow(ctx, p, items[2:n], false, false)
				})
			case n > 0:
				kit.MetricSummaryRow(ctx, p, items[:n], false, false)
			}
		} else if live.Device.Name != "" {
			ui.Text(ctx, live.Device.Name).FontSize(12).TextColor(p.Muted)
		}
	})
}

func (d *dashboard) overviewPDCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, live backend.Snapshot, pdSnap backend.PacketSnapshot) {
	if d.service == nil || (len(pdSnap.Packets) == 0 && !live.Device.Supports(device.CapPDCapture)) {
		return
	}
	kit.Card(ctx, p).Gap(8).Children(func() {
		ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			kit.SectionTitle(ctx, msgs.PagePd())
			ui.Spacer(ctx)
			if pdSnap.Total > 0 {
				kit.Badge(ctx, p, strconv.FormatUint(pdSnap.Total, 10), kit.ToneAccent)
			}
		})
		kit.Divider(ctx, p)
		if len(pdSnap.Packets) == 0 {
			ui.Text(ctx, msgs.PdWaiting()).FontSize(12).TextColor(p.Muted)
			return
		}
		for i := len(pdSnap.Packets) - 1; i >= max(0, len(pdSnap.Packets)-5); i-- {
			pkt := pdSnap.Packets[i]
			row := ui.ButtonBase(ctx.Key(overviewPDKey(pkt.Sequence))).
				FillWidth().Padding(5, 8).Radius(kit.RadiusS).AlignItems(ui.Center).Gap(8).
				Label(pkt.Type).OnClick(func() {
				d.packets.selected = pkt.Sequence
				d.goPage(pagePD)
			})
			bg := ui.Transparent
			if row.Hovered() || row.FocusVisible() {
				bg = p.Hover
			} else if row.Pressed() {
				bg = p.Press
			}
			row.Background(bg).Children(func() {
				ui.Text(ctx, fmt.Sprintf("#%d", pkt.Sequence)).FontFeatures("tnum").Font("monospace").FontSize(11).TextColor(p.Muted).Shrink(0)
				ui.Text(ctx, pkt.Type).FontSize(12).FontWeight(600).SingleLine().Grow(1).MinWidth(0)
				ui.Text(ctx, fmt.Sprintf("%.2fs", pkt.Elapsed)).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).Shrink(0)
			})
		}
	})
}

func (d *dashboard) hold(snapshot backend.Snapshot) {
	if d.held != nil {
		return
	}
	if d.service != nil {
		snapshot = d.service.SnapshotWindowSampled(0, 0)
	}
	d.held = &snapshot
	d.chartPaused.Store(true)
}

func (d *dashboard) historyBounds(cw chartWindow) (bufStart, bufEnd float64, ok bool) {
	if d.held != nil && len(d.held.History) > 0 {
		full := d.held.History
		return full[0].Elapsed, full[len(full)-1].Elapsed, true
	}
	if d.service != nil {
		meta := d.service.Snapshot(0)
		if meta.HistoryCount > 0 {
			return meta.HistoryStart, meta.HistoryEnd, true
		}
	}
	if cw.bufEnd > cw.bufStart {
		return cw.bufStart, cw.bufEnd, true
	}
	return 0, 0, false
}

func (d *dashboard) atLiveEdge(cw chartWindow, tol float64) bool {
	if !d.hasViewEnd {
		return true
	}
	_, bufEnd, ok := d.historyBounds(cw)
	if !ok {
		return true
	}
	return d.viewEnd >= bufEnd-tol
}

func trendAxis(low, high float64) chart.Linear {
	lo, hi := min(low, 0), max(high, 0)
	span := max(hi-lo, 0.5)
	pad := max(span*0.08, 0.05)
	if lo < 0 {
		lo -= pad
	}
	if hi > 0 || lo == 0 {
		hi += pad
	}
	axis := chart.NewLinear(lo, hi).Nice(3)
	if low >= 0 {
		axis.Lo = 0
	}
	if high <= 0 && low < 0 && axis.Hi > 0 {
		axis.Hi = 0
	}
	return axis
}

func chartAxis(bounds backend.ChartRange, stat channelStat) chart.Linear {
	if bounds.Valid {
		return trendAxis(bounds.Min, bounds.Max)
	}
	return trendAxis(stat.low, stat.high)
}

type chartSamplesKey struct {
	first        *device.Sample
	count        int
	firstElapsed float64
	lastElapsed  float64
}

func chartSamplesIdentity(history []device.Sample) chartSamplesKey {
	if len(history) == 0 {
		return chartSamplesKey{}
	}
	return chartSamplesKey{
		first:        &history[0],
		count:        len(history),
		firstElapsed: history[0].Elapsed,
		lastElapsed:  history[len(history)-1].Elapsed,
	}
}

type chartStatsCache struct {
	key            chartSamplesKey
	valid          uint8
	hasDataChecked uint8
	hasDataMask    uint8
	stats          [len(channels)]channelStat
}

func (d *dashboard) chartStatsFor(history []device.Sample) *chartStatsCache {
	key := chartSamplesIdentity(history)
	cache := &d.chartStats
	if cache.key != key {
		*cache = chartStatsCache{key: key}
	}
	return cache
}

func (d *dashboard) cachedHasChannelData(history []device.Sample, index int) bool {
	cache := d.chartStatsFor(history)
	bit := uint8(1 << index)
	if cache.hasDataChecked&bit == 0 {
		if hasChannelData(history, channels[index]) {
			cache.hasDataMask |= bit
		}
		cache.hasDataChecked |= bit
	}
	return cache.hasDataMask&bit != 0
}

func (d *dashboard) visibleChartStat(history []device.Sample, index int) channelStat {
	cache := d.chartStatsFor(history)
	mask := uint8(1 << index)
	if cache.valid&mask == 0 {
		stat := channelStat{low: math.Inf(1), high: math.Inf(-1)}
		read := channels[index].read
		for sampleIndex := range history {
			value, ok := read(&history[sampleIndex])
			if ok {
				stat.low, stat.high = min(stat.low, value), max(stat.high, value)
				stat.count++
				stat.mean += (value - stat.mean) / float64(stat.count)
			}
		}
		cache.stats[index] = stat
		cache.valid |= mask
	}
	return cache.stats[index]
}

func (d *dashboard) chartLegendRow(ctx *ui.Context, p kit.Palette, msgs locales.Messages, history []device.Sample, seriesMask uint8, compact bool) {
	var visBuf [len(chartChannelOrder)]int
	visible := visBuf[:0]
	for _, idx := range chartChannelOrder {
		if idx < 3 || d.cachedHasChannelData(history, idx) {
			visible = append(visible, idx)
		}
	}
	if len(visible) == 0 {
		return
	}
	rowGap, chipH, chipPadX := float32(6), float32(24), float32(9)
	if compact {
		rowGap, chipH, chipPadX = 5, 22, 8
	}
	ui.Row(ctx).FillWidth().Wrap().Gap(rowGap).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, idx := range visible {
			ch := channels[idx]
			tone := ch.color(p)
			primary := idx == d.channel
			enabled := seriesMask&uint8(1<<idx) != 0
			label := ch.name(msgs)

			chip := ui.ButtonBase(ctx.Key(chartTraceKey(idx))).
				Height(chipH).Shrink(0).Padding(0, chipPadX).Gap(5).Radius(kit.RadiusFull).
				AlignItems(ui.Center).Justify(ui.Center).
				Label(label + " (" + ch.unit + ")").
				OnClick(func() {
					if primary {
						return
					}
					d.toggleChartSeries(idx)
				})

			bg, bd, fg, dot, weight := ui.Transparent, p.LineSoft, p.Muted, p.Faint, 500
			switch {
			case primary:
				bg, bd, fg, dot, weight = tone.Alpha(0.14), tone.Alpha(0.45), tone, tone, 650
			case enabled:
				bg, bd, fg, dot, weight = p.Inset, tone.Alpha(0.32), p.Text, tone, 550
			case chip.Hovered() || chip.FocusVisible():
				bg, bd, fg = p.Hover, p.Line, p.Text
			}
			if chip.Pressed() && !primary {
				bg = p.Press
			}
			var chipTint ui.Color
			switch {
			case primary:
				chipTint = tone.Alpha(0.24)
			case enabled:
				chipTint = tone.Alpha(0.13)
			}
			kit.ControlMotion(chip).Children(func() {
				kit.GlassPillControl(ctx.Key("bg"), chip, p, kit.RadiusFull, chipTint, primary || enabled, bg, 1, bd, 0.96)
				kit.StatusDot(ctx, dot, 6)
				ui.Text(ctx, label).FontSize(11).FontWeight(weight).TextColor(fg).SingleLine()
			})
		}
	})
}

func formatCompactMetric(value float64, decimals int) string {
	switch {
	case math.Abs(value) >= 100:
		decimals = min(decimals, 1)
	case math.Abs(value) >= 10:
		decimals = min(decimals, 2)
	default:
		decimals = min(decimals, 3)
	}
	return fmt.Sprintf("%.*f", decimals, value)
}

type seriesSample struct {
	elapsed float64
	value   float64
}

func chartSmoothingKernel(span float64, dpWidth float32) (sigma, radius float64) {
	const baseSigma = 0.08
	sPerDp := span / max(float64(dpWidth), 1)
	sigma = max(baseSigma, sPerDp*0.85)
	return sigma, sigma * 3.0
}

// A sampled history can have wider spacing than the meter's native cadence.
func seriesGapThreshold(history []device.Sample) float64 {
	const baseGap = 0.5
	n := len(history)
	if n < 4 {
		return baseGap
	}
	const maxSamples = 8192
	// An odd stride samples both spacings when a bucket contributes two extrema.
	stride := 1 + (n-2)/maxSamples
	if stride%2 == 0 {
		stride++
	}
	deltas := make([]float64, 0, 1+(n-2)/stride)
	for i := 1; i < n; i += stride {
		if history[i].Segment != history[i-1].Segment {
			continue
		}
		if dt := history[i].Elapsed - history[i-1].Elapsed; dt > 0 {
			deltas = append(deltas, dt)
		}
	}
	if len(deltas) < 3 {
		return baseGap
	}
	slices.Sort(deltas)
	cadence := deltas[(len(deltas)-1)*3/4]
	if len(deltas) < 8 {
		cadence = deltas[(len(deltas)-1)/2]
	}
	return max(baseGap, cadence*4)
}

func historyWindowBounds(n int, at func(int) float64, start, end float64) (lo, hi int) {
	lo = sort.Search(n, func(i int) bool { return at(i) >= start })
	hi = sort.Search(n, func(i int) bool { return at(i) > end })
	if lo > 0 {
		lo--
	}
	if hi < n {
		hi++
	}
	return lo, hi
}

func visibleHistory(history []device.Sample, start, end float64) []device.Sample {
	lo, hi := historyWindowBounds(len(history), func(i int) float64 { return history[i].Elapsed }, start, end)
	return history[lo:hi]
}

func downsampleSeriesRun(run []seriesSample, target int) []seriesSample {
	if len(run) <= target {
		return run
	}
	first, last := run[0], run[len(run)-1]
	span := last.elapsed - first.elapsed
	if span <= 0 {
		return run[len(run)-1:]
	}
	buckets := target - 2
	pointsPerSecond := float64(buckets) / span
	out := make([]seriesSample, 1, target)
	out[0] = first
	var elapsedSum, valueSum float64
	bucket, count := 0, 0
	flush := func() {
		if count > 0 {
			out = append(out, seriesSample{
				elapsed: first.elapsed + elapsedSum/float64(count),
				value:   valueSum / float64(count),
			})
			elapsedSum, valueSum, count = 0, 0, 0
		}
	}
	for _, sample := range run[1 : len(run)-1] {
		offset := sample.elapsed - first.elapsed
		next := min(int(offset*pointsPerSecond), buckets-1)
		if next != bucket {
			flush()
			bucket = next
		}
		elapsedSum += offset
		valueSum += sample.value
		count++
	}
	flush()
	return append(out, last)
}

func smoothSeriesRun(run []seriesSample, sigma, radius float64) []seriesSample {
	if len(run) < 3 {
		return run
	}
	out := make([]seriesSample, len(run))
	leftIdx, rightIdx := 0, 0
	for i := range run {
		t0 := run[i].elapsed
		for leftIdx < i && t0-run[leftIdx].elapsed > radius {
			leftIdx++
		}
		if rightIdx < i {
			rightIdx = i
		}
		for rightIdx+1 < len(run) && run[rightIdx+1].elapsed-t0 <= radius {
			rightIdx++
		}
		var wSum, vSum float64
		for j := leftIdx; j <= rightIdx; j++ {
			dt := (run[j].elapsed - t0) / sigma
			w := math.Exp(-0.5 * dt * dt)
			wSum += w
			vSum += w * run[j].value
		}
		out[i] = seriesSample{elapsed: t0, value: vSum / wSum}
	}
	return out
}

func trimRunToWindow(run []seriesSample, winStart, winEnd float64) []seriesSample {
	n := len(run)
	if n == 0 || run[n-1].elapsed < winStart || run[0].elapsed > winEnd {
		return nil
	}
	lo, hi := historyWindowBounds(n, func(i int) float64 { return run[i].elapsed }, winStart, winEnd)
	return run[lo:hi]
}

func sampleNormalizedSeries(view []device.Sample, ch measurement, axis chart.Linear, winStart, winEnd, maxGap, sigma, radius float64, pixelWidth int) (xs, ysUp []float32, lastValue float64) {
	span := max(winEnd-winStart, 1e-6)
	n := len(view)
	samples := make([]seriesSample, n)
	for i := range view {
		value, ok := ch.read(&view[i])
		if !ok || math.IsInf(value, 0) {
			value = math.NaN()
		}
		samples[i] = seriesSample{elapsed: view[i].Elapsed, value: value}
	}

	xs = make([]float32, 0, min(n, max(256, pixelWidth*2)))
	ysUp = make([]float32, 0, cap(xs))
	gap := float32(math.NaN())
	maxSmoothPts := max(4000, pixelWidth*2)
	lastValue = math.NaN()

	for runStart := 0; runStart < n; {
		if math.IsNaN(samples[runStart].value) {
			runStart++
			continue
		}
		seg := view[runStart].Segment
		runEnd := runStart + 1
		for runEnd < n && !math.IsNaN(samples[runEnd].value) && view[runEnd].Segment == seg &&
			samples[runEnd].elapsed-samples[runEnd-1].elapsed < maxGap {
			runEnd++
		}

		run := samples[runStart:runEnd]
		if sigma > 0 {
			run = smoothSeriesRun(downsampleSeriesRun(run, maxSmoothPts), sigma, radius)
		}
		visible := trimRunToWindow(run, winStart, winEnd)
		if len(visible) > 0 {
			if len(xs) > 0 {
				xs, ysUp = append(xs, gap), append(ysUp, gap)
			}
			for _, pt := range visible {
				xf := float32((pt.elapsed - winStart) / span)
				yf := clampOrder(axis.At(pt.value), 0, 1)
				xs = append(xs, xf)
				ysUp = append(ysUp, yf)
				if pt.elapsed >= winStart && pt.elapsed <= winEnd+1e-9 {
					lastValue = pt.value
				}
			}
		}
		runStart = runEnd
	}
	if pixelWidth > 8 && len(xs) > pixelWidth*2 {
		xs, ysUp = chart.MinMax(nil, nil, xs, ysUp, pixelWidth)
	}
	return xs, ysUp, lastValue
}

func seriesPointsFromNormalized(plotRect ui.Rect, xs, ysUp []float32) [][2]float32 {
	points := make([][2]float32, 0, len(xs))
	gap := float32(math.NaN())
	breakPt := [2]float32{gap, gap}
	connected := false
	for i, x := range xs {
		if math.IsNaN(float64(x)) || math.IsNaN(float64(ysUp[i])) {
			if connected {
				points = append(points, breakPt)
			}
			connected = false
			continue
		}
		px := chart.X(plotRect, x)
		py := max(plotRect.Y, min(plotRect.Y+plotRect.H, chart.Y(plotRect, ysUp[i])))
		points = append(points, [2]float32{px, py})
		connected = true
	}
	return points
}

func buildSeriesArea(rect ui.Rect, axis chart.Linear, points [][2]float32) *ui.Path {
	baseline := rect.Y + rect.H
	if axis.Lo < 0 {
		baseline = chart.Y(rect, clampOrder(axis.At(0), 0, 1))
	}
	var path *ui.Path
	start := 0
	for end := 0; end <= len(points); end++ {
		if end < len(points) && !math.IsNaN(float64(points[end][0])) {
			continue
		}
		if end-start >= 2 {
			if path == nil {
				path = &ui.Path{}
			}
			path.MoveTo(points[start][0], points[start][1])
			for _, point := range points[start+1 : end] {
				path.LineTo(point[0], point[1])
			}
			path.LineTo(points[end-1][0], baseline)
			path.LineTo(points[start][0], baseline)
			path.Close()
		}
		start = end + 1
	}
	return path
}

type chartGeometryKey struct {
	samples    chartSamplesKey
	rect       ui.Rect
	scale      float32
	start, end float64
	primary    int
	series     uint8
	smooth     bool
	axes       [len(channels)][2]float64
}

type chartGeometry struct {
	key       chartGeometryKey
	lines     [len(channels)][][2]float32
	area      *ui.Path
	lastValue float64
}

func (d *dashboard) prepareChartGeometry(history []device.Sample, ranges [7]backend.ChartRange, rect ui.Rect, scale float32, start, end float64, primary int, series uint8) *chartGeometry {
	if scale <= 0 {
		scale = 1
	}
	source := history
	if d.held != nil {
		// The paused readout uses visible samples, but the curve needs neighbors past the edges.
		source = d.held.History
	}
	key := chartGeometryKey{samples: chartSamplesIdentity(source), rect: rect, scale: scale, start: start, end: end, primary: primary, series: series, smooth: d.settings.SmoothChart}
	for index := range channels {
		if index == primary || series&uint8(1<<index) != 0 {
			axis := chartAxis(ranges[index], d.visibleChartStat(history, index))
			key.axes[index] = [2]float64{axis.Lo, axis.Hi}
		}
	}
	cache := &d.chartGeometry
	// Scroll only moves the plot. Reuse its paths and their masks at the new origin.
	if key.rect.W == cache.key.rect.W && key.rect.H == cache.key.rect.H {
		key.rect.X, key.rect.Y = cache.key.rect.X, cache.key.rect.Y
	}
	if cache.key == key {
		return cache
	}
	key.rect = rect
	*cache = chartGeometry{key: key, lastValue: math.NaN()}
	var sigma, radius float64
	if key.smooth {
		sigma, radius = chartSmoothingKernel(end-start, rect.W)
	}
	view := visibleHistory(source, start-radius, end+radius)
	maxGap := seriesGapThreshold(view)
	pixelWidth := max(1, int(math.Ceil(float64(rect.W*scale))))
	for index, channel := range channels {
		if index != primary && series&uint8(1<<index) == 0 {
			continue
		}
		stat := d.visibleChartStat(history, index)
		if stat.count == 0 {
			continue
		}
		axis := chart.NewLinear(key.axes[index][0], key.axes[index][1])
		xs, ys, lastValue := sampleNormalizedSeries(view, channel, axis, start, end, maxGap, sigma, radius, pixelWidth)
		cache.lines[index] = seriesPointsFromNormalized(rect, xs, ys)
		if index == primary {
			cache.lastValue = lastValue
			cache.area = buildSeriesArea(rect, axis, cache.lines[index])
		}
	}
	return cache
}

func (d *dashboard) secondaryAxisChannel(history []device.Sample, seriesMask uint8, wideLeftAxis bool) (int, bool) {
	if !wideLeftAxis {
		return -1, false
	}
	for _, idx := range [...]int{3, 6, 4, 5, 1, 2, 0} {
		if idx == d.channel || seriesMask&uint8(1<<idx) == 0 {
			continue
		}
		if d.cachedHasChannelData(history, idx) {
			return idx, true
		}
	}
	return -1, false
}

type chartTickMetricsKey struct {
	channel       int
	axisLo        float64
	axisHi        float64
	unit          string
	scale         float32
	compactMobile bool
	hasSample     bool
}

type chartTickMetricsCache struct {
	key         chartTickMetricsKey
	ticks       []chart.Tick
	labels      []string
	widths      []float32
	rightMargin float32
}

func (d *dashboard) resolveChartTicks(ctx *ui.Context, slot, channel int, axis chart.Linear, unit string, compactMobile, hasSample bool) chartTickMetricsCache {
	scale := ctx.Zoom()
	if scale <= 0 {
		scale = 1
	}
	key := chartTickMetricsKey{
		channel:       channel,
		axisLo:        axis.Lo,
		axisHi:        axis.Hi,
		unit:          unit,
		scale:         scale,
		compactMobile: compactMobile,
		hasSample:     hasSample,
	}
	if cached := d.chartTickMetrics[slot]; cached.key == key && len(cached.ticks) > 0 {
		return cached
	}
	ticks := axis.Ticks(4)
	const tickLabelGap = float32(6)
	minRight, maxRight := float32(34), float32(64)
	if compactMobile {
		minRight, maxRight = 28, 54
	}
	if hasSample {
		if compactMobile {
			minRight = max(minRight, 40)
		} else {
			minRight = max(minRight, 46)
		}
	}
	rightMargin := minRight
	labels := make([]string, len(ticks))
	widths := make([]float32, len(ticks))
	for idx, tick := range ticks {
		lbl := tick.Label
		if idx == len(ticks)-1 {
			lbl = tick.Label + " " + unit
		}
		tw, _ := ctx.MeasureText(1<<20, ui.Span{Text: lbl, Size: 10})
		labels[idx] = lbl
		widths[idx] = tw
		rightMargin = max(rightMargin, float32(math.Ceil(float64(tw)))+tickLabelGap)
	}
	rightMargin = min(rightMargin, maxRight)
	resolved := chartTickMetricsCache{
		key:         key,
		ticks:       ticks,
		labels:      labels,
		widths:      widths,
		rightMargin: rightMargin,
	}
	if !d.snapshotOnly {
		d.chartTickMetrics[slot] = resolved
	}
	return resolved
}

type chartPlotCanvasKey struct {
	geom        chartGeometryKey
	wPx         int
	hPx         int
	scale       float32
	dark        bool
	accent      ui.Color
	surface     ui.Color
	inset       ui.Color
	line        ui.Color
	lineSoft    ui.Color
	xDivs       int
	tickCount   int
	firstTickAt float32
	lastTickAt  float32
	firstVal    float64
	lastVal     float64
	primary     int
	seriesMask  uint8
	hasLeftAxis bool
}

func makeChartPlotCanvasKey(geomKey chartGeometryKey, wPx, hPx int, scale float32, p kit.Palette, xDivs int, ticks []chart.Tick, primary int, seriesMask uint8, hasLeftAxis bool) chartPlotCanvasKey {
	geomKey.rect.X = 0
	geomKey.rect.Y = 0
	var firstAt, lastAt float32
	var firstVal, lastVal float64
	if n := len(ticks); n > 0 {
		firstAt, lastAt = ticks[0].At, ticks[n-1].At
		firstVal, lastVal = ticks[0].Value, ticks[n-1].Value
	}
	return chartPlotCanvasKey{
		geom:        geomKey,
		wPx:         wPx,
		hPx:         hPx,
		scale:       scale,
		dark:        p.Dark,
		accent:      p.Accent,
		surface:     p.Surface,
		inset:       p.Inset,
		line:        p.Line,
		lineSoft:    p.LineSoft,
		xDivs:       xDivs,
		tickCount:   len(ticks),
		firstTickAt: firstAt,
		lastTickAt:  lastAt,
		firstVal:    firstVal,
		lastVal:     lastVal,
		primary:     primary,
		seriesMask:  seriesMask,
		hasLeftAxis: hasLeftAxis,
	}
}

func (d *dashboard) pixelPoints(points [][2]float32, origin ui.Rect, scale float32) [][2]float32 {
	n := len(points)
	if cap(d.chartPixelPts) < n {
		d.chartPixelPts = make([][2]float32, n)
	}
	out := d.chartPixelPts[:n]
	for i, pt := range points {
		if math.IsNaN(float64(pt[0])) || math.IsNaN(float64(pt[1])) {
			out[i] = pt
			continue
		}
		out[i] = [2]float32{(pt[0] - origin.X) * scale, (pt[1] - origin.Y) * scale}
	}
	d.chartPixelPts = out
	return out
}

func secondaryCurveColor(ch measurement, p kit.Palette, wellAlpha float32) ui.Color {
	wellBg := p.Inset.Alpha(wellAlpha).Over(p.Surface)
	c := ch.color(p).Mix(wellBg, 0.14)
	c.A = 255
	return c
}

func (d *dashboard) closestHistorySample(history []device.Sample, target float64) *device.Sample {
	switch {
	case d.held != nil && len(d.held.History) > 0:
		return closestSample(d.held.History, target)
	case d.service != nil:
		return d.service.ClosestSample(target)
	default:
		return closestSample(history, target)
	}
}

func (d *dashboard) drawChartPlotLayer(painter *ui.Painter, rect ui.Rect, scale float32, p kit.Palette, wellAlpha float32, xDivs int, ticks []chart.Tick, geometry *chartGeometry, axis chart.Linear, primary int, hasLeftAxis, pixelSpace bool) {
	strokeScale, lineW, edge := float32(1), float32(1), float32(0)
	if pixelSpace {
		strokeScale, lineW = scale, max(1, scale)
		edge = lineW * 0.5
	}
	left, top, right, bottom := rect.X, rect.Y, rect.X+rect.W, rect.Y+rect.H
	painter.Fill(rect, p.Inset.Alpha(wellAlpha), 4*strokeScale)

	gridV := p.LineSoft.Alpha(0.65)
	for i := 1; i < xDivs; i++ {
		gx := left + rect.W*float32(i)/float32(xDivs)
		painter.Line(gx, top, gx, bottom, lineW, gridV)
	}
	for _, tick := range ticks {
		y := bottom - rect.H*tick.At
		gridColor := p.LineSoft
		if math.Abs(tick.Value) < 1e-9 {
			gridColor = p.Line
		}
		painter.Line(left, y, right, y, lineW, gridColor)
	}

	origin := geometry.key.rect
	tone := channels[primary].color(p)
	drawCurves := func() {
		for index, points := range geometry.lines {
			if index != primary && len(points) >= 2 {
				pts := points
				if pixelSpace {
					pts = d.pixelPoints(points, origin, scale)
				}
				painter.StrokePolyline(pts, 1.5*strokeScale, secondaryCurveColor(channels[index], p, wellAlpha))
			}
		}
		primaryPts := geometry.lines[primary]
		area := geometry.area
		if pixelSpace {
			area = nil
			if len(primaryPts) >= 2 {
				primaryPts = d.pixelPoints(primaryPts, origin, scale)
				area = buildSeriesArea(rect, axis, primaryPts)
			}
		}
		if area != nil {
			painter.FillPathGradient(area, ui.LinearGradient{Angle: 180, From: tone.Alpha(0.24), To: tone.Alpha(0.01)})
		}
		if len(primaryPts) >= 2 {
			painter.StrokePolyline(primaryPts, 2.2*strokeScale, tone)
		}
	}
	painter.Clip(rect, 0, func() {
		if pixelSpace {
			drawCurves()
			return
		}
		painter.Transform(ui.Translation(rect.X-origin.X, rect.Y-origin.Y), 0, 0, drawCurves)
	})

	painter.Line(left, bottom-edge, right, bottom-edge, lineW, p.Line)
	painter.Line(right-edge, top, right-edge, bottom, lineW, p.LineSoft)
	if hasLeftAxis {
		painter.Line(left+edge, top, left+edge, bottom, lineW, p.LineSoft)
	}
}

func (d *dashboard) resetChartDrag() {
	d.dragMode = chartDragNone
	d.dragMoved = false
	d.dragClicks = 0
}

func (d *dashboard) trendEmptyState(ctx *ui.Context, p kit.Palette, msgs locales.Messages, live backend.Snapshot, height float32, empty string, touch bool) {
	emptyBox := ui.Box(ctx).FillWidth().Center()
	if height <= 0 {
		emptyBox.Grow(1).MinHeight(120)
	} else {
		emptyBox.MinHeight(height)
	}
	emptyBox.Children(func() {
		kit.EmptyState(ctx, p, kit.IconActivity, empty, "", func() {
			switch {
			case live.ErrorKind == "selection":
				kit.ActionButton(ctx.Key("find-empty"), p, kit.VariantPrimary, kit.IconSearch, msgs.GuiFindMeters(), touch, func() {
					d.goPage(pageDevice)
					d.scanMeters(msgs, false)
				}).Disabled(d.busy)
			case live.State != backend.Streaming && live.State != backend.Connecting:
				kit.ActionButton(ctx.Key("connect-empty"), p, kit.VariantPrimary, kit.IconPlay, msgs.GuiConnect(), touch, func() {
					d.connect()
				}).Disabled(d.busy)
			}
		})
	})
}

func (d *dashboard) handleChartInput(ctx *ui.Context, history []device.Sample, cw chartWindow, leftMargin, rightMargin float32) func(ui.InputEvent) bool {
	winStart, winEnd := cw.winStart, cw.winEnd
	pointerFractionW := func(localX, boxW float32) float64 {
		plotW := max(1, boxW-leftMargin-rightMargin)
		return clampOrder(float64(localX-leftMargin)/float64(plotW), 0, 1)
	}
	seekAtX := func(localX, boxW float32) {
		target := winStart + pointerFractionW(localX, boxW)*(winEnd-winStart)
		s := d.closestHistorySample(history, target)
		if s == nil {
			return
		}
		if !d.hasViewEnd {
			if bufStart, bufEnd, ok := d.historyBounds(cw); ok {
				minEnd := min(bufEnd, bufStart+cw.windowSec)
				d.hasViewEnd = true
				d.viewEnd = clampOrder(winEnd, minEnd, bufEnd)
			}
		}
		d.cursorAt = s.Elapsed
		d.hasCursor = true
		d.displayAt = time.Time{}
	}
	nearCursor := func(localX, boxW float32) bool {
		if !d.hasCursor || winEnd <= winStart || !cw.contains(d.cursorAt) {
			return false
		}
		plotW := max(1, boxW-leftMargin-rightMargin)
		t1X := leftMargin + float32((d.cursorAt-winStart)/(winEnd-winStart))*plotW
		return math.Abs(float64(localX-t1X)) <= 12
	}
	return func(event ui.InputEvent) bool {
		rect := d.chartHandle.Bounds(ctx)
		plotW := max(1, rect.W-leftMargin-rightMargin)
		switch event.Kind {
		case ui.InputZoom:
			if event.Scale > 0 && event.Scale != 1 {
				d.zoomByFactor(cw, pointerFractionW(event.X, rect.W), 1/float64(event.Scale))
				return true
			}
		case ui.InputScroll:
			if math.Abs(float64(event.DX)) > math.Abs(float64(event.DY)) || event.Mods&ui.Shift != 0 {
				delta := event.DX
				if delta == 0 {
					delta = event.DY
				}
				if d.held == nil && !d.hasViewEnd && (delta > 0 || event.Momentum) {
					return true
				}
				atEdge := d.panChart(delta, plotW, cw)
				if d.held == nil && d.hasViewEnd && delta > 0 && atEdge {
					d.resumeLive()
				}
				return true
			}
			if (d.mobile && event.Precise) || event.DY == 0 {
				return false
			}
			if event.Precise && event.Momentum {
				return true
			}
			dy := clampOrder(float64(event.DY), -120, 120)
			d.zoomByFactor(cw, pointerFractionW(event.X, rect.W), math.Exp(dy*0.0045))
			return true
		case ui.InputPointerDown:
			if event.Button == 0 {
				d.dragStartX, d.dragLastX = event.X, event.X
				d.dragMoved = false
				d.dragClicks = event.Clicks
				if d.lastPressDragged {
					d.dragClicks = 1
				}
				d.dragMode = chartDragPending
				return true
			}
			if event.Button == 2 {
				d.dragMode = chartDragPan
				d.dragMoved = true
				d.dragLastX = event.X
				return true
			}
		case ui.InputPointerMove:
			switch d.dragMode {
			case chartDragPending:
				moved := event.X - d.dragStartX
				if math.Abs(float64(moved)) < 4 {
					return true
				}
				d.dragMoved = true
				d.dragMode = chartDragPan
				d.panChart(-moved, plotW, cw)
				d.dragLastX = event.X
				return true
			case chartDragPan:
				dx := event.X - d.dragLastX
				d.dragLastX = event.X
				d.panChart(-dx, plotW, cw)
				return true
			}
		case ui.InputPointerUp:
			switch d.dragMode {
			case chartDragPending:
				wasMoved, clicks := d.dragMoved, d.dragClicks
				d.lastPressDragged = wasMoved
				d.resetChartDrag()
				if !event.Canceled && event.Button == 0 && !wasMoved {
					switch {
					case clicks >= 2:
						if d.held != nil || d.hasViewEnd || d.hasCursor {
							d.resumeLive()
						} else {
							sessionSpan := max(0, cw.bufEnd-cw.bufStart)
							if d.fullSession || math.Abs(cw.windowSec-defaultWindowSec) > 0.5 {
								d.fullSession, d.windowSec, d.displayAt = false, defaultWindowSec, time.Time{}
							} else if sessionSpan > defaultWindowSec+0.5 {
								d.fullSession, d.windowSec, d.displayAt = true, sessionSpan, time.Time{}
							}
						}
					case nearCursor(event.X, rect.W):
						d.hasCursor = false
						d.displayAt = time.Time{}
						if d.held == nil && d.atLiveEdge(cw, 1.5) {
							d.resumeLive()
						}
					default:
						seekAtX(event.X, rect.W)
					}
				}
				return true
			case chartDragPan:
				if event.Button == 0 || event.Button == 2 || event.Canceled {
					d.lastPressDragged = true
					d.resetChartDrag()
					if d.held == nil && d.hasViewEnd && d.atLiveEdge(cw, 0.15) {
						d.resumeLive()
					}
					return true
				}
			}
		}
		return false
	}
}

func (d *dashboard) drawChartOverlay(painter *ui.Painter, p kit.Palette, msgs locales.Messages, rect, plotRect ui.Rect, history []device.Sample, sample *device.Sample, channel measurement, geometry *chartGeometry, axis chart.Linear, tickMetrics, secTickMetrics chartTickMetricsCache, secIdx int, rightMargin, xLabelOffset float32, xDivs int, cw chartWindow, seriesMask uint8) {
	winStart, winEnd := cw.winStart, cw.winEnd
	left, top, right, bottom := plotRect.X, plotRect.Y, plotRect.X+plotRect.W, plotRect.Y+plotRect.H
	tone := channel.color(p)

	for i := 0; i <= xDivs; i++ {
		frac := float32(i) / float32(xDivs)
		gx := left + plotRect.W*frac
		label := formatChartTime(winStart + float64(frac)*(winEnd-winStart))
		labelWidth, _ := painter.MeasureText(1<<20, ui.Span{Text: label, Size: 10})
		lx := clampOrder(gx-labelWidth/2, left, right-labelWidth)
		painter.Text(lx, bottom+xLabelOffset, label, 10, p.Muted)
	}

	toPoint := func(elapsed, value float64, ax chart.Linear) ui.Point {
		xf := float32((elapsed - winStart) / max(winEnd-winStart, 1e-6))
		yf := clampOrder(ax.At(value), 0, 1)
		return ui.Point{
			X: clampOrder(chart.X(plotRect, xf), left, right),
			Y: clampOrder(chart.Y(plotRect, yf), top, bottom),
		}
	}

	// Primary Right Y-axis + live pill band.
	hasLivePill := false
	var livePos ui.Point
	var liveStr string
	var livePillY float32
	if sample != nil {
		if value, ok := channel.read(sample); ok {
			hasLivePill = true
			markerX, markerValue := sample.Elapsed, value
			if geometry.key.smooth && !d.hasCursor && !math.IsNaN(geometry.lastValue) && len(history) > 0 {
				last := &history[len(history)-1]
				if sample.Segment == last.Segment && last.Elapsed <= winEnd+1e-9 && math.Abs(sample.Elapsed-last.Elapsed) < 0.25 {
					if _, valid := channel.read(last); valid {
						markerX, markerValue = last.Elapsed, geometry.lastValue
					}
				}
			}
			livePos = toPoint(markerX, markerValue, axis)
			if !d.hasCursor {
				liveStr = formatCompactMetric(value, channel.decimals)
				livePillY = clampOrder(livePos.Y-9, top, bottom-18)
			}
		}
	}

	topTickSkipped := false
	for idx, tick := range tickMetrics.ticks {
		y := bottom - plotRect.H*tick.At
		labelY := clampOrder(y-6, top-2, bottom-10)
		if liveStr != "" && labelY+12 > livePillY-2 && labelY < livePillY+20 {
			if idx == len(tickMetrics.ticks)-1 {
				topTickSkipped = true
			}
			continue
		}
		painter.Text(rect.X+rect.W-tickMetrics.widths[idx], labelY, tickMetrics.labels[idx], 10, p.Muted)
	}

	if len(secTickMetrics.ticks) > 0 {
		secTone := channels[secIdx].color(p)
		for idx, tick := range secTickMetrics.ticks {
			y := bottom - plotRect.H*tick.At
			painter.Text(left-6-secTickMetrics.widths[idx], clampOrder(y-6, top-2, bottom-10), secTickMetrics.labels[idx], 10, secTone.Alpha(0.85))
		}
	}

	if hasLivePill {
		painter.Fill(ui.Rect{X: livePos.X - 5.5, Y: livePos.Y - 5.5, W: 11, H: 11}, tone.Alpha(0.25), 5.5)
		painter.Fill(ui.Rect{X: livePos.X - 3, Y: livePos.Y - 3, W: 6, H: 6}, tone, 3)

		if liveStr != "" {
			pillStr := liveStr
			if topTickSkipped {
				withUnit := liveStr + " " + channel.unit
				if uw, _ := painter.MeasureText(1<<20, ui.Span{Text: withUnit, Size: 10}); uw+8 <= rightMargin-3 {
					pillStr = withUnit
				}
			}
			lw, _ := painter.MeasureText(1<<20, ui.Span{Text: pillStr, Size: 10})
			pillW := min(rightMargin-3, lw+8)
			pillX := rect.X + rect.W - pillW
			painter.Fill(ui.Rect{X: pillX, Y: livePillY, W: pillW, H: 18}, tone, 4)
			painter.Text(pillX+(pillW-lw)/2, livePillY+3, pillStr, 10, kit.AccentInk(tone))
		}
	}

	if (d.held != nil || d.hasViewEnd) && cw.bufEnd-cw.bufStart > cw.windowSec+0.5 {
		bufSpan := cw.bufEnd - cw.bufStart
		startFrac := clampOrder(float32((winStart-cw.bufStart)/bufSpan), 0, 1)
		endFrac := clampOrder(float32((winEnd-cw.bufStart)/bufSpan), startFrac, 1)
		thumbX := left + plotRect.W*startFrac
		thumbW := max(14, plotRect.W*(endFrac-startFrac))
		if thumbX+thumbW > right {
			thumbX = max(left, right-thumbW)
		}
		painter.Fill(ui.Rect{X: thumbX, Y: bottom + 1, W: thumbW, H: 1.5}, p.Muted.Alpha(0.45), 0.75)
	}

	var t1, t2 *device.Sample
	t1Visible := false
	if d.hasCursor {
		t1 = d.closestHistorySample(history, d.cursorAt)
		if t1 != nil && cw.contains(t1.Elapsed) {
			t1Visible = true
			if v1, ok := channel.read(t1); ok {
				p1 := toPoint(t1.Elapsed, v1, axis)
				painter.Line(p1.X, top, p1.X, bottom, 1, tone.Alpha(0.75))
				painter.Line(left, p1.Y, right, p1.Y, 1, tone.Alpha(0.22))
				painter.Fill(ui.Rect{X: p1.X - 3, Y: top, W: 6, H: 3}, tone, 1.5)
				painter.Fill(ui.Rect{X: p1.X - 5, Y: p1.Y - 5, W: 10, H: 10}, tone.Alpha(0.25), 5)
				painter.Fill(ui.Rect{X: p1.X - 3, Y: p1.Y - 3, W: 6, H: 6}, tone, 3)
			}
		}
	}
	if d.hasHover {
		t2 = closestSample(history, d.hoverAt)
		if t2 != nil {
			if v2, ok := channel.read(t2); ok {
				p2 := toPoint(t2.Elapsed, v2, axis)
				painter.Line(p2.X, top, p2.X, bottom, 1, p.Muted.Alpha(0.60))
				painter.Line(left, p2.Y, right, p2.Y, 1, p.Muted.Alpha(0.22))
				painter.Fill(ui.Rect{X: p2.X - 3.5, Y: p2.Y - 3.5, W: 7, H: 7}, p.Surface, 3.5)
				painter.Stroke(ui.Rect{X: p2.X - 3.5, Y: p2.Y - 3.5, W: 7, H: 7}, tone, 3.5, 1.5)
			}
		}
	}

	hudSample := t2
	if hudSample == nil && t1Visible {
		hudSample = t1
	}
	if hudSample != nil {
		d.drawCrosshairHUD(painter, p, msgs, plotRect, seriesMask, winStart, winEnd, hudSample, t1, t2)
	}
}

func (d *dashboard) trend(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap, live backend.Snapshot, channel measurement, sample *device.Sample, cw chartWindow, height float32, empty string, touch bool, seriesMask uint8, wideLeftAxis bool) {
	history := snap.History
	primary := d.channel
	stat := d.visibleChartStat(history, primary)
	if stat.count == 0 {
		d.trendEmptyState(ctx, p, msgs, live, height, empty, touch)
		return
	}

	compactMobile := d.mobile && !wideLeftAxis
	secIdx, hasLeftAxis := d.secondaryAxisChannel(history, seriesMask, wideLeftAxis)
	leftMargin := float32(0)
	if hasLeftAxis {
		leftMargin = 46
	}
	axis := chartAxis(snap.ChartRanges[primary], stat)
	tickMetrics := d.resolveChartTicks(ctx, 0, primary, axis, channel.unit, compactMobile, sample != nil)
	ticks := tickMetrics.ticks
	rightMargin := tickMetrics.rightMargin

	var secTickMetrics chartTickMetricsCache
	if hasLeftAxis {
		if secStat := d.visibleChartStat(history, secIdx); secStat.count > 0 {
			secCh := channels[secIdx]
			secAxis := chartAxis(snap.ChartRanges[secIdx], secStat)
			secTickMetrics = d.resolveChartTicks(ctx, 1, secIdx, secAxis, secCh.unit, compactMobile, false)
		}
	}

	if cw.winEnd <= cw.winStart {
		bufStart := history[0].Elapsed
		if d.fullSession {
			bufStart = 0
		}
		bufEnd := history[len(history)-1].Elapsed
		cw = chartWindowAt(bufStart, bufEnd, cw.windowSec, bufEnd)
	}
	winStart, winEnd := cw.winStart, cw.winEnd
	sampleAtX := func(localX, boxW float32, samples []device.Sample) *device.Sample {
		if len(samples) == 0 {
			return nil
		}
		plotW := max(1, boxW-leftMargin-rightMargin)
		frac := clampOrder(float64(localX-leftMargin)/float64(plotW), 0, 1)
		return closestSample(samples, winStart+frac*(winEnd-winStart))
	}

	canPan := cw.bufEnd-cw.bufStart > cw.windowSec+1e-3 || (d.hasViewEnd && d.viewEnd < cw.bufEnd-1e-3)
	plot := ui.Box(ctx.Key("chart")).Bind(&d.chartHandle).FillWidth()
	if height <= 0 {
		plot.Grow(1).MinHeight(120)
	} else {
		plot.Height(height)
	}
	plot.HandleInput(d.handleChartInput(ctx, history, cw, leftMargin, rightMargin))
	if plot.RightClicked() {
		d.resumeLive()
	}

	if !d.mobile && d.dragMode != chartDragPan {
		hx, hy, over := plot.PointerPosition()
		if over {
			bounds := plot.Bounds()
			if bounds.W > leftMargin+rightMargin && hx >= leftMargin && hx <= bounds.W-rightMargin && hy >= 4 && hy <= bounds.H-14 {
				if hs := sampleAtX(hx, bounds.W, history); hs != nil {
					d.hoverAt = hs.Elapsed
					d.hasHover = true
				} else {
					d.hasHover = false
				}
			} else {
				d.hasHover = false
			}
		} else {
			d.hasHover = false
		}
	} else if d.mobile || d.dragMode == chartDragPan {
		d.hasHover = false
	}

	switch {
	case d.dragMode == chartDragPan:
		plot.Cursor(ui.CursorGrabbing)
	case canPan:
		plot.Cursor(ui.CursorGrab)
	default:
		plot.Cursor(ui.CursorCrosshair)
	}

	plot.Draw(func(painter *ui.Painter, rect ui.Rect) {
		plotTop, plotBottom, xLabelOffset := float32(8), float32(20), float32(5)
		if compactMobile {
			plotTop, plotBottom, xLabelOffset = 4, 18, 4
		}
		left, right := rect.X+leftMargin, rect.X+rect.W-rightMargin
		top, bottom := rect.Y+plotTop, rect.Y+rect.H-plotBottom
		if right <= left+4 || bottom <= top+4 {
			return
		}
		plotRect := ui.Rect{X: left, Y: top, W: right - left, H: bottom - top}

		wellAlpha := float32(0.32)
		if p.Dark {
			wellAlpha = 0.45
		}
		xDivs := 4
		if plotRect.W < 420 {
			xDivs = 2
		}
		scale := painter.Scale()
		if scale <= 0 {
			scale = 1
		}
		geometry := d.prepareChartGeometry(history, snap.ChartRanges, plotRect, scale, winStart, winEnd, primary, seriesMask)
		wPx := max(1, int(math.Round(float64(plotRect.W*scale))))
		hPx := max(1, int(math.Round(float64(plotRect.H*scale))))

		if !d.snapshotOnly && wPx <= 4096 && hPx <= 4096 {
			plotKey := makeChartPlotCanvasKey(geometry.key, wPx, hPx, scale, p, xDivs, ticks, primary, seriesMask, hasLeftAxis)
			if d.chartCanvas == nil {
				d.chartCanvas = ui.NewCanvas(wPx, hPx)
			} else if cwPx, chPx := d.chartCanvas.Size(); cwPx != wPx || chPx != hPx {
				d.chartCanvas = ui.NewCanvas(wPx, hPx)
				d.chartCanvasKey = chartPlotCanvasKey{}
			}
			if d.chartCanvasKey != plotKey {
				d.chartCanvas.Draw(painter, func(cp *ui.Painter) {
					d.drawChartPlotLayer(cp, ui.Rect{W: float32(wPx), H: float32(hPx)}, scale, p, wellAlpha, xDivs, ticks, geometry, axis, primary, hasLeftAxis, true)
				})
				d.chartCanvasKey = plotKey
			}
			painter.Image(d.chartCanvas.Bitmap(), plotRect, ui.FillBox)
		} else {
			d.drawChartPlotLayer(painter, plotRect, scale, p, wellAlpha, xDivs, ticks, geometry, axis, primary, hasLeftAxis, false)
		}

		d.drawChartOverlay(painter, p, msgs, rect, plotRect, history, sample, channel, geometry, axis, tickMetrics, secTickMetrics, secIdx, rightMargin, xLabelOffset, xDivs, cw, seriesMask)
	})
}

func (d *dashboard) drawCrosshairHUD(painter *ui.Painter, p kit.Palette, msgs locales.Messages, plotRect ui.Rect, seriesMask uint8, winStart, winEnd float64, target, t1, t2 *device.Sample) {
	ch := channels[d.channel]
	val, ok := ch.read(target)
	if !ok {
		return
	}
	xf := float32((target.Elapsed - winStart) / max(winEnd-winStart, 1e-6))
	cx := clampOrder(chart.X(plotRect, xf), plotRect.X, plotRect.X+plotRect.W)

	header := formatChartTime(target.Elapsed)
	if t1 != nil && t2 != nil && math.Abs(t2.Elapsed-t1.Elapsed) >= 0.05 {
		dt := t2.Elapsed - t1.Elapsed
		if v1, ok1 := ch.read(t1); ok1 {
			header = fmt.Sprintf("Δt %s · %+.2f %s", formatRelTime(dt), val-v1, ch.unit)
		}
	}

	type row struct {
		color ui.Color
		text  string
	}
	var rowBuf [len(chartChannelOrder)]row
	rows := rowBuf[:0]
	maxW, _ := painter.MeasureText(1<<20, ui.Span{Text: header, Size: 10.5, Weight: 600})
	addRow := func(idx int) {
		c := channels[idx]
		if v, ok := c.read(target); ok {
			txt := fmt.Sprintf("%s: %s %s", c.name(msgs), formatCompactMetric(v, c.decimals), c.unit)
			tw, _ := painter.MeasureText(1<<20, ui.Span{Text: txt, Size: 10.5})
			maxW = max(maxW, tw+12)
			rows = append(rows, row{c.color(p), txt})
		}
	}
	addRow(d.channel)
	for _, idx := range chartChannelOrder {
		if idx != d.channel && seriesMask&uint8(1<<idx) != 0 {
			addRow(idx)
		}
	}

	hudW := min(plotRect.W-12, maxW+18)
	hudH := float32(22 + len(rows)*16 + 6)
	hudX := cx + 10
	if hudX+hudW > plotRect.X+plotRect.W-4 {
		hudX = cx - hudW - 10
	}
	hudX = clampOrder(hudX, plotRect.X+4, plotRect.X+plotRect.W-hudW-4)
	hudY := clampOrder(plotRect.Y+6, plotRect.Y+4, plotRect.Y+plotRect.H-hudH-4)

	cardRect := ui.Rect{X: hudX, Y: hudY, W: hudW, H: hudH}
	painter.Shadow(cardRect, kit.RadiusS, 0, 4, 12, 0, p.Shadow)
	painter.Fill(cardRect, p.Surface.Alpha(0.95), kit.RadiusS)
	painter.Stroke(cardRect, ch.color(p).Alpha(0.45), kit.RadiusS, 1)
	painter.Text(hudX+8, hudY+6, header, 10.5, p.Muted)
	for i, r := range rows {
		ry := hudY + 23 + float32(i)*16
		painter.Fill(ui.Rect{X: hudX + 8, Y: ry + 4, W: 6, H: 6}, r.color, 3)
		painter.Text(hudX+18, ry, r.text, 10.5, p.Text)
	}
}

func (d *dashboard) zoomByFactor(cw chartWindow, fraction, factor float64) {
	if !(factor > 0) || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return
	}
	bufStart, bufEnd, hasBounds := d.historyBounds(cw)
	sessionSpan := 0.0
	if hasBounds {
		sessionSpan = max(0, bufEnd-bufStart)
	}
	maxWin := d.maxWindowFor(sessionSpan)
	oldWin := cw.windowSec
	newWin := clampOrder(oldWin*factor, minWindowSec, maxWin)
	hitMax := factor > 1 && sessionSpan > 0 && newWin >= sessionSpan-1e-3
	if hitMax {
		newWin = max(minWindowSec, sessionSpan)
	}
	if math.Abs(newWin-oldWin) < 1e-6 && d.fullSession == hitMax {
		return
	}
	d.windowSec = newWin
	d.fullSession = hitMax
	d.displayAt = time.Time{}

	if d.held == nil && !d.hasViewEnd {
		return
	}
	if !hasBounds {
		return
	}
	tMouse := cw.winStart + fraction*oldWin
	rawEnd := tMouse + (1-fraction)*newWin
	minEnd := min(bufEnd, bufStart+newWin)
	newEnd := clampOrder(rawEnd, minEnd, bufEnd)
	if d.held == nil && factor > 1 && newEnd >= bufEnd-1e-3 && newEnd-newWin <= bufStart+1e-3 {
		d.hasViewEnd = false
		d.hasCursor = false
		return
	}
	d.hasViewEnd = true
	d.viewEnd = newEnd
}

func (d *dashboard) panChart(dx, plotW float32, cw chartWindow) bool {
	if dx == 0 {
		return false
	}
	bufStart, bufEnd, ok := d.historyBounds(cw)
	if !ok {
		return false
	}
	minEnd := min(bufEnd, bufStart+cw.windowSec)
	if bufEnd-minEnd < 1e-6 {
		return true
	}
	curEnd := bufEnd
	if d.hasViewEnd {
		curEnd = d.viewEnd
	} else if d.held != nil && cw.winEnd > cw.winStart {
		curEnd = cw.winEnd
	}
	rawEnd := curEnd + float64(dx)/float64(max(1, plotW))*cw.windowSec
	if d.held == nil && !d.hasViewEnd && rawEnd >= bufEnd-1e-3 {
		return true
	}
	clamped := clampOrder(rawEnd, minEnd, bufEnd)
	d.hasViewEnd = true
	d.viewEnd = clamped
	d.displayAt = time.Time{}
	return rawEnd >= bufEnd-1e-3
}
