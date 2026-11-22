package app

import (
	"fmt"
	"sort"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

const (
	readoutInterval  = 250 * time.Millisecond
	minWindowSec     = 1.0
	defaultWindowSec = 60.0
	chartMaxLivePts  = 4000
)

type chartWindow struct {
	windowSec float64
	winStart  float64
	winEnd    float64
	bufStart  float64
	bufEnd    float64
}

func chartWindowAt(bufStart, bufEnd, windowSec, end float64) chartWindow {
	return chartWindow{
		windowSec: windowSec,
		winStart:  max(bufStart, end-windowSec),
		winEnd:    max(end, bufStart+windowSec),
		bufStart:  bufStart,
		bufEnd:    bufEnd,
	}
}

type chartHistoryQuery struct {
	duration   time.Duration
	start, end float64
	ranged     bool
}

type chartHistoryCache struct {
	service *backend.Service
	query   chartHistoryQuery
	version uint64
	samples []device.Sample
}

func (d *dashboard) sampledChartHistory(live backend.Snapshot, query chartHistoryQuery) backend.Snapshot {
	if d.service == nil {
		return live
	}
	cache := &d.chartHistory
	if cache.service == d.service && cache.version == live.HistoryVersion && cache.query == query {
		live.History = cache.samples
		return live
	}
	if query.ranged {
		live = d.service.SnapshotRangeSampled(query.start, query.end, chartMaxLivePts)
	} else {
		live = d.service.SnapshotWindowSampled(query.duration, chartMaxLivePts)
	}
	*cache = chartHistoryCache{service: d.service, query: query, version: live.HistoryVersion, samples: live.History}
	return live
}

func clampFloat(v, lo, hi float64) float64 {
	return max(lo, min(hi, v))
}

func (d *dashboard) maxWindowFor(sessionSpan float64) float64 {
	return max(defaultWindowSec, sessionSpan)
}

func (d *dashboard) effectiveWindow() float64 {
	if d.windowSec > 0 {
		return max(minWindowSec, d.windowSec)
	}
	return defaultWindowSec
}

func (d *dashboard) effectiveWindowFor(sessionSpan float64) float64 {
	if d.fullSession {
		return max(minWindowSec, sessionSpan)
	}
	if d.windowSec > 0 {
		return clampFloat(d.windowSec, minWindowSec, d.maxWindowFor(sessionSpan))
	}
	return defaultWindowSec
}

func (d *dashboard) resumeLive() {
	d.held = nil
	d.chartPaused.Store(false)
	d.hasViewEnd = false
	d.hasCursor = false
	d.hasHover = false
	d.dragMode = chartDragNone
	d.dragMoved = false
	d.dragClicks = 0
	d.displayAt = time.Time{}
}

func (d *dashboard) resetChartView() {
	d.resumeLive()
	d.chartHistory = chartHistoryCache{}
	d.chartStats = chartStatsCache{}
	d.chartGeometry = chartGeometry{}
	d.fullSession = true
	d.windowSec = defaultWindowSec
}

func closestSample(samples []device.Sample, at float64) *device.Sample {
	if len(samples) == 0 {
		return nil
	}
	idx := sort.Search(len(samples), func(i int) bool { return samples[i].Elapsed >= at })
	if idx == len(samples) {
		idx = len(samples) - 1
	} else if idx > 0 && at-samples[idx-1].Elapsed < samples[idx].Elapsed-at {
		idx--
	}
	return &samples[idx]
}

func (d *dashboard) locale() string {
	language := d.language
	if d.mobile && language == settingsAuto {
		language = i18n.Locale()
	}
	return locales.Resolve(language)
}

func (d *dashboard) themeIsDark(ctx *ui.Context) bool {
	switch d.settings.Theme {
	case mygo.ThemeDark:
		return true
	case mygo.ThemeLight:
		return false
	default:
		if ctx != nil {
			return ctx.Theme().Dark
		}
		return d.dark
	}
}

func (d *dashboard) frameTheme(ctx *ui.Context) (kit.Palette, locales.Messages) {
	locale := d.locale()
	if d.mobile && d.notifyLanguage.Load() != locale {
		d.notifyLanguage.Store(locale)
	}
	dark := d.themeIsDark(ctx)
	d.dark = dark
	if d.mobile && d.window != nil {
		style := mygo.BarStyleDark
		if dark {
			style = mygo.BarStyleLight
		}
		if d.lastBarStyle != style {
			d.lastBarStyle = style
			mygo.SystemBars.SetStyle(style)
		}
	}
	theme, p := kit.ResolveTheme(dark, d.settings.Accent)
	p.LiquidGlass = d.settings.LiquidGlass
	if ctx != nil {
		p.LiquidGlass = p.LiquidGlass && !ctx.Preferences().ReduceTransparency
		ctx.SetTheme(theme)
	}
	return p, locales.In(locale)
}

func (d *dashboard) view(ctx *ui.Context) {
	defer kit.BeginFrame(ctx, d.mobile)()

	d.drainUIQueue()
	d.applyThemeChanges(ctx)
	p, msgs := d.frameTheme(ctx)
	alternate, reveal := d.themeReveal.Sync(ctx, p.Dark)

	for _, t := range d.toasts {
		ctx.AddToast(t)
	}
	clear(d.toasts)
	d.toasts = d.toasts[:0]

	var snapshot backend.Snapshot
	if d.service != nil {
		snapshot = d.service.Snapshot(0)
		if snapshot.Device.PathDependentID && d.source.Device != "" {
			d.source.Device = snapshot.Device.ID
		}
	}

	// Anchor the theme-stage to the full window padding box so theme transitions cover safe-area insets too.
	stage := ui.Box(ctx.Key("theme-stage")).Absolute().Top(0).Right(0).Bottom(0).Left(0)
	stage.Children(func() {
		if reveal != nil {
			oldTheme, oldP := kit.ResolveTheme(!p.Dark, d.settings.Accent)
			oldP.LiquidGlass = p.LiquidGlass
			oldSnap := d.snapshotForReveal(!p.Dark)
			ui.Box(ctx.Key("theme-reveal-alternate")).Absolute().Fill().ZIndex(0).
				PassThrough().IgnoreInput(true).AccessHidden(true).Cache().Effect(alternate).Children(func() {
				defer kit.BeginStaticSnapshot(ctx)()
				ui.Themed(ctx, oldTheme).Fill().Children(func() {
					oldSnap.draw(ctx, oldP, msgs, snapshot)
				})
			})
		}
		host := ui.Box(ctx.Key("workspace-host")).Fill().ZIndex(1)
		if reveal != nil {
			if d.mobile {
				host.Background(p.Canvas)
			}
			host.Cache().Effect(reveal)
		}
		host.Children(func() {
			d.draw(ctx, p, msgs, snapshot)
		})
	})
}

func (d *dashboard) snapshotForReveal(fromDark bool) *dashboard {
	snap := &dashboard{
		meterAccessState: d.meterAccessState,
		ctx:              d.ctx,
		source:           d.source,
		language:         d.language,
		mobile:           d.mobile,
		settings:         d.settings,
		dark:             fromDark,
		service:          d.service,
		err:              d.err,
		logs:             d.logs,
		logLevel:         d.logLevel,
		logView:          d.logView,
		page:             d.page,
		pageDirection:    d.pageDirection,
		scroll:           ui.ScrollState{X: d.scroll.X, Y: d.scroll.Y},
		channel:          d.channel,
		chartSeries:      d.chartSeries,
		chartHistory:     d.chartHistory,
		chartStats:       d.chartStats,
		chartGeometry:    d.chartGeometry,
		windowSec:        d.windowSec,
		fullSession:      d.fullSession,
		held:             d.held,
		hasViewEnd:       d.hasViewEnd,
		viewEnd:          d.viewEnd,
		hasCursor:        d.hasCursor,
		cursorAt:         d.cursorAt,
		hasHover:         d.hasHover,
		hoverAt:          d.hoverAt,
		displayAt:        d.displayAt,
		displaySample:    d.displaySample,
		displayStats:     d.displayStats,
		displayBounds:    d.displayBounds,
		packets:          d.packets,
		meters:           d.meters,
		menus:            nil,
		dir:              d.dir,
		busy:             d.busy,
		snapshotOnly:     true,
	}
	snap.packets.expanded = false
	snap.packets.list.Handle = ui.Handle{}
	snap.logView.list.Handle = ui.Handle{}
	return snap
}

func (d *dashboard) draw(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snapshot backend.Snapshot) {
	width, height := ctx.Size()
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	landscape := d.isLandscape(ctx)
	position := navigationPlacement(d.mobile, landscape, d.settings.Navigation)
	if landscape {
		width -= leftInset + rightInset
		if position == navigationSide {
			width -= d.sidebarWidthFor(ctx)
		}
		width -= 24
		height -= topInset + bottomInset + mobileHeaderHeight + 16
	} else {
		if position == navigationSide {
			width -= d.sidebarWidthFor(ctx)
		}
		width -= 4 * workspacePadding
		if position == navigationBottom {
			height -= topInset + kit.BottomNavReserveFor(bottomInset, true, d.bottomBarStyle()) + mobileHeaderHeight + 16
		} else {
			height -= headerHeight
		}
	}

	status, notice, tone := dashboardStatus(p, msgs, snapshot, d.err)
	name := snapshot.Device.Name
	switch {
	case snapshot.Device.Simulated:
		name = msgs.SimulatedMeter()
	case name == "":
		name = msgs.GuiNoMeter()
	}

	overviewPage := workspacePage{
		label: msgs.GuiOverview(),
		icon:  kit.IconActivity,
	}
	if landscape {
		overviewPage.fixed = func() { d.overview(ctx, p, msgs, snapshot, width, height, notice) }
	} else {
		overviewPage.content = func() { d.overview(ctx, p, msgs, snapshot, width, height, notice) }
	}

	pages := [...]workspacePage{
		pageOverview: overviewPage,
		pagePD: {
			label:   msgs.PagePd(),
			icon:    kit.IconUSB,
			list:    func(bottomPadding float32) { d.pdPage(ctx, p, msgs, snapshot, bottomPadding) },
			overlay: func() { d.packetDialog(ctx, p, msgs) },
		},
		pageRecords: {
			label:   msgs.GuiRecords(),
			icon:    kit.IconFileText,
			content: func() { d.recordsPage(ctx, p, msgs, snapshot) },
		},
		pageDevice: {
			label:   msgs.GuiDevice(),
			icon:    kit.IconCpu,
			content: func() { d.devicePage(ctx, p, msgs, snapshot) },
		},
		pageSettings: {
			label:   msgs.GuiSettings(),
			icon:    kit.IconSliders,
			content: func() { d.settingsPage(ctx, p, msgs) },
		},
		pageMore: {
			label:   msgs.GuiMore(),
			icon:    kit.IconMore,
			content: func() { d.morePage(ctx, p, msgs) },
		},
		pageConnectionLog: {
			label: msgs.GuiConnectionLog(),
			icon:  kit.IconFileText,
			list:  func(bottomPadding float32) { d.connectionLogPage(ctx, p, msgs, bottomPadding) },
		},
	}
	d.workspace(ctx, p, msgs, pages[:], position, name, status, tone)
}

type channelStat struct {
	low, high, mean float64
	count           int
}

func (d *dashboard) throttledReadout(ctx *ui.Context, live, windowSnap backend.Snapshot) (*device.Sample, backend.Statistics, [3]channelStat) {
	if live.State != backend.Streaming || live.Latest == nil {
		d.displaySample = nil
		d.displayAt = time.Time{}
		d.displayBounds = [3]channelStat{}
		return nil, live.Statistics, d.displayBounds
	}
	now := ctx.Now()
	elapsed := now.Sub(d.displayAt)
	if d.displaySample == nil || elapsed >= readoutInterval {
		sampleCopy := device.Clone(*live.Latest)
		d.displaySample = &sampleCopy
		d.displayStats = live.Statistics
		d.displayAt = now
		for i := range d.displayBounds {
			d.displayBounds[i] = d.visibleChartStat(windowSnap.History, i)
		}
	} else if live.Latest.Elapsed != d.displaySample.Elapsed {
		ctx.After(readoutInterval - elapsed)
	}
	return d.displaySample, d.displayStats, d.displayBounds
}

func (d *dashboard) overview(ctx *ui.Context, p kit.Palette, msgs locales.Messages, live backend.Snapshot, width, height float32, notice string) {
	snapshot := live
	cw := chartWindow{windowSec: d.effectiveWindow()}
	var chartSample *device.Sample
	var readoutSample *device.Sample
	var readoutStats backend.Statistics
	var metricStats [3]channelStat

	if d.held != nil {
		snapshot = *d.held
		full := d.held.History
		if len(full) != 0 {
			bufStart, bufEnd := full[0].Elapsed, full[len(full)-1].Elapsed
			if d.fullSession {
				bufStart = 0
			}
			sessionSpan := max(0, bufEnd-bufStart)
			window := d.effectiveWindowFor(sessionSpan)
			winEnd := bufEnd
			if d.hasViewEnd {
				minEnd := min(bufEnd, bufStart+window)
				winEnd = clampFloat(d.viewEnd, minEnd, bufEnd)
				d.viewEnd = winEnd
			}
			cw = chartWindowAt(bufStart, bufEnd, window, winEnd)
			startIdx := sort.Search(len(full), func(i int) bool { return full[i].Elapsed >= cw.winStart })
			endIdx := sort.Search(len(full), func(i int) bool { return full[i].Elapsed > cw.winEnd })
			if startIdx <= endIdx {
				snapshot.History = full[startIdx:endIdx]
			} else {
				snapshot.History = nil
			}
		}
		readoutStats = snapshot.Statistics
		if d.hasCursor && len(full) > 0 {
			readoutSample = closestSample(full, d.cursorAt)
		} else {
			readoutSample = snapshot.Latest
			if readoutSample == nil && len(full) > 0 {
				readoutSample = &full[len(full)-1]
			}
		}
		if len(snapshot.History) > 0 {
			if d.hasCursor && d.cursorAt >= cw.winStart-1e-6 && d.cursorAt <= cw.winEnd+1e-6 {
				chartSample = closestSample(snapshot.History, d.cursorAt)
			} else {
				chartSample = &snapshot.History[len(snapshot.History)-1]
			}
		}
		for i := range metricStats {
			metricStats[i] = d.visibleChartStat(snapshot.History, i)
		}
	} else {
		sessionSpan := 0.0
		if live.HistoryCount > 0 {
			sessionSpan = max(0, live.HistoryEnd-live.HistoryStart)
			if d.fullSession {
				sessionSpan = max(0, live.HistoryEnd)
			}
		} else {
			d.hasViewEnd = false
			d.hasCursor = false
		}
		window := d.effectiveWindowFor(sessionSpan)
		if d.hasViewEnd && d.service != nil && live.HistoryCount > 0 {
			bufStart, bufEnd := live.HistoryStart, live.HistoryEnd
			minEnd := min(bufEnd, bufStart+window)
			winEnd := clampFloat(d.viewEnd, minEnd, bufEnd)
			d.viewEnd = winEnd
			cw = chartWindowAt(bufStart, bufEnd, window, winEnd)
			snapshot = d.sampledChartHistory(live, chartHistoryQuery{start: cw.winStart, end: winEnd, ranged: true})
			if snapshot.HistoryCount > 0 {
				cw.bufStart = snapshot.HistoryStart
				cw.bufEnd = snapshot.HistoryEnd
			}
			readoutSample, readoutStats, metricStats = d.throttledReadout(ctx, live, snapshot)
			if d.hasCursor {
				if t1 := d.service.ClosestSample(d.cursorAt); t1 != nil {
					readoutSample = t1
					if t1.Elapsed >= cw.winStart-1e-6 && t1.Elapsed <= cw.winEnd+1e-6 {
						chartSample = t1
					}
				}
			}
			if chartSample == nil && len(snapshot.History) > 0 {
				chartSample = &snapshot.History[len(snapshot.History)-1]
			}
			if snapshot.State != backend.Streaming && snapshot.HistoryCount == 0 {
				chartSample = nil
				readoutSample = nil
			}
		} else {
			query := chartHistoryQuery{}
			if !d.fullSession {
				query.duration = time.Duration(window * float64(time.Second))
			}
			snapshot = d.sampledChartHistory(live, query)
			if snapshot.HistoryCount > 0 {
				cw.bufStart = snapshot.HistoryStart
				cw.bufEnd = snapshot.HistoryEnd
			} else if len(snapshot.History) > 0 {
				cw.bufStart = snapshot.History[0].Elapsed
				cw.bufEnd = snapshot.History[len(snapshot.History)-1].Elapsed
			}
			if d.fullSession {
				cw.bufStart = 0
			}
			window = d.effectiveWindowFor(max(0, cw.bufEnd-cw.bufStart))
			cw.windowSec = window
			if snapshot.HistoryCount > 0 || len(snapshot.History) > 0 {
				cw = chartWindowAt(cw.bufStart, cw.bufEnd, window, cw.bufEnd)
			}
			chartSample = snapshot.Latest
			readoutSample, readoutStats, metricStats = d.throttledReadout(ctx, live, snapshot)
			if snapshot.State != backend.Streaming {
				chartSample = nil
				readoutSample = nil
			}
		}
	}
	d.measurementPanel(ctx, p, msgs, overviewFrame{
		snap:          snapshot,
		live:          live,
		chartSample:   chartSample,
		readoutSample: readoutSample,
		readoutStats:  readoutStats,
		metricStats:   metricStats,
		cw:            cw,
		width:         width,
		height:        height,
		notice:        notice,
	})
}

func dashboardStatus(p kit.Palette, msgs locales.Messages, snap backend.Snapshot, err error) (string, string, ui.Color) {
	switch {
	case err != nil:
		return msgs.GuiOffline(), err.Error(), p.Red
	case snap.ErrorKind == "permission":
		return msgs.AccessRequired(), "", p.Warning
	case snap.ErrorKind == "missing":
		return msgs.GuiOffline(), "", p.Muted
	case snap.ErrorKind == "selection":
		return msgs.GuiOffline(), "", p.Warning
	case snap.ErrorKind == "timeout":
		return msgs.GuiOffline(), msgs.LogErrorTimeout(), p.Warning
	case snap.ErrorKind != "" && snap.ErrorKind != "report":
		return msgs.GuiOffline(), snap.Error, p.Warning
	case snap.State == backend.Streaming:
		return "", "", p.Green
	case snap.State == backend.Connecting:
		return msgs.Connecting(), "", p.Warning
	case snap.State == backend.Waiting:
		return msgs.Waiting(), "", p.Warning
	default:
		return msgs.GuiOffline(), "", p.Muted
	}
}

func metricSummaryText(msgs locales.Messages, channel measurement, stat channelStat, compactStats bool) string {
	if stat.count <= 0 {
		return ""
	}
	if compactStats {
		decimals := min(channel.decimals, 2)
		return fmt.Sprintf("%s %.*f · %s %.*f", msgs.Average(), decimals, stat.mean, msgs.GuiMax(), decimals, stat.high)
	}
	return fmt.Sprintf("%s %.*f · %s %.*f · %s %.*f",
		msgs.Minimum(), channel.decimals, stat.low,
		msgs.Average(), channel.decimals, stat.mean,
		msgs.GuiMax(), channel.decimals, stat.high)
}

func metricCellColors(p kit.Palette, tone ui.Color, active, pressed, hovered, focused bool) (bg, border, labelColor, tint ui.Color, borderW float32) {
	activeMix, pressMix, activeBorderAlpha := float32(.09), float32(.13), float32(.50)
	if p.Dark {
		activeMix, pressMix, activeBorderAlpha = .15, .20, .46
	}
	bg, border, borderW, labelColor = p.Inset, p.LineSoft.Alpha(.65), 1, p.Muted
	switch {
	case active:
		bg = p.Surface.Mix(tone, activeMix)
		if pressed {
			bg = p.Surface.Mix(tone, pressMix)
		}
		border = tone.Alpha(activeBorderAlpha)
		borderW = 1.25
		labelColor = tone
		tint = tone.Alpha(0.14)
	case pressed:
		bg = p.SurfaceHi.Mix(tone, .09)
		border = tone.Alpha(.38)
		labelColor = p.Text
	case hovered || focused:
		bg = p.SurfaceHi
		border = p.Line
		labelColor = p.Text
	}
	return bg, border, labelColor, tint, borderW
}

type metricCellStyle struct {
	wide          bool
	inlineSummary bool
	padY, padX    float32
	gap           float32
	dotSize       float32
	dotGap        float32
	labelSize     float32
	summarySize   float32
	valueSize     float32
	unitSize      float32
	readingGap    float32
}

func metricLabelRow(ctx *ui.Context, dotColor, labelColor ui.Color, dotSize, gap, fontSize float32, label string) ui.Element {
	return ui.Row(ctx).AlignItems(ui.Center).Gap(gap).Children(func() {
		kit.StatusDot(ctx, dotColor, dotSize)
		ui.Text(ctx, label).FontSize(fontSize).FontWeight(650).TextColor(labelColor).SingleLine()
	})
}

func (d *dashboard) metricCell(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample, metricStats [3]channelStat, index int, style metricCellStyle) {
	channel := channels[index]
	label := channel.name(msgs)
	tone := channel.color(p)
	value := channel.format(sample)
	active := d.channel == index
	summary := metricSummaryText(msgs, channel, metricStats[index], true)

	btn := ui.ButtonBase(ctx.Key(index)).Label(label+" "+value+" "+channel.unit).
		Role(ui.RoleTab).AccessSelected(active).Background(ui.Transparent).Radius(kit.RadiusM).MinWidth(0).
		Column().AlignItems(ui.Start).Padding(style.padY, style.padX).Gap(style.gap).
		OnClick(func() { d.selectChartChannel(index) })
	if style.wide {
		btn.FillWidth()
	} else {
		btn.Grow(1).Basis(0)
	}

	bg, border, labelColor, cellTint, borderW := metricCellColors(p, tone, active, btn.Pressed(), btn.Hovered(), btn.FocusVisible())
	dotColor := tone.Alpha(.60)
	if active {
		dotColor = tone
	}

	btn.Children(func() {
		kit.GlassPillControl(ctx.Key("bg"), btn, p, kit.RadiusM, cellTint, true, bg, borderW, border, 0.985)
		if style.inlineSummary {
			ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(style.dotGap).Justify(ui.SpaceBetween).Children(func() {
				metricLabelRow(ctx, dotColor, labelColor, style.dotSize, style.dotGap, style.labelSize, label).Shrink(0)
				if summary != "" {
					ui.Text(ctx, summary).FontFeatures("tnum").FontSize(style.summarySize).TextColor(p.Muted).MinWidth(0).SingleLine()
				}
			})
		} else {
			metricLabelRow(ctx, dotColor, labelColor, style.dotSize, style.dotGap, style.labelSize, label)
		}
		readingWithUnit(ctx, p, value, channel.unit, style.valueSize, style.unitSize, style.readingGap, tone)
		if !style.inlineSummary && summary != "" {
			ui.Text(ctx, summary).FontFeatures("tnum").FontSize(style.summarySize).TextColor(p.Muted).SingleLine()
		}
	})
}

func (d *dashboard) mobileMetricsCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample, metricStats [3]channelStat, fontSize float32) {
	wideStyle := metricCellStyle{
		wide: true, inlineSummary: true,
		padY: 13, padX: 14, gap: 6, dotSize: 6.5, dotGap: 6,
		labelSize: 12, summarySize: 11, valueSize: min(27, max(24, fontSize+4)), unitSize: 12, readingGap: 5,
	}
	narrowStyle := metricCellStyle{
		wide: false, inlineSummary: false,
		padY: 11, padX: 13, gap: 3, dotSize: 6.5, dotGap: 6,
		labelSize: 12, summarySize: 11, valueSize: max(19, fontSize-4), unitSize: 12, readingGap: 5,
	}
	kit.Card(ctx, p).Radius(kit.RadiusL + 2).Padding(5).Gap(5).Children(func() {
		d.metricCell(ctx, p, msgs, sample, metricStats, 0, wideStyle)
		ui.Row(ctx).FillWidth().Gap(5).AlignItems(ui.Stretch).Children(func() {
			d.metricCell(ctx, p, msgs, sample, metricStats, 1, narrowStyle)
			d.metricCell(ctx, p, msgs, sample, metricStats, 2, narrowStyle)
		})
	})
}

func (d *dashboard) landscapeMetricsCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample, metricStats [3]channelStat) {
	style := metricCellStyle{
		wide: true, inlineSummary: true,
		padY: 6, padX: 10, gap: 1, dotSize: 6, dotGap: 5,
		labelSize: 11.5, summarySize: 10, valueSize: 18.5, unitSize: 11, readingGap: 4,
	}
	kit.Card(ctx, p).Radius(kit.RadiusL).Padding(5).Gap(3).Children(func() {
		for index := range 3 {
			d.metricCell(ctx, p, msgs, sample, metricStats, index, style)
		}
	})
}

func (d *dashboard) metricTab(ctx *ui.Context, p kit.Palette, msgs locales.Messages, channel measurement, sample *device.Sample, stat channelStat, label string, index int, fontSize float32, stacked, compactStats bool) {
	tone := channel.color(p)
	value := channel.format(sample)
	active := d.channel == index

	tab := ui.ButtonBase(ctx.Key(index)).Label(label + " " + value + " " + channel.unit).
		Role(ui.RoleTab).AccessSelected(active).Background(ui.Transparent).Radius(kit.RadiusL).MinWidth(0).
		OnClick(func() { d.selectChartChannel(index) })

	if stacked {
		tab.FillWidth().MinHeight(56).Padding(12, 16).Gap(10).AlignItems(ui.Center).Justify(ui.SpaceBetween)
	} else {
		tab.Grow(1).Basis(0).Column().AlignItems(ui.Start).Padding(14, 16).Gap(5)
	}

	bg, border, borderW, labelColor := p.Surface, p.LineSoft, float32(1), p.Muted
	switch {
	case active:
		bg = p.Surface.Mix(tone, .07)
		border = tone.Alpha(.65)
		borderW = 1.5
		labelColor = tone
	case tab.Pressed() || tab.Hovered() || tab.FocusVisible():
		bg = p.SurfaceHi
		border = p.Line
		labelColor = p.Text
	}
	if p.LiquidGlass && !active {
		kit.ApplyCardShadow(kit.GlassSurface(kit.CardMotion(tab).Radius(kit.RadiusL), p, 0.25, bg, border), p)
	} else {
		cardBg := bg
		if p.LiquidGlass && active {
			if p.Dark {
				cardBg = p.SurfaceHi.Mix(tone, 0.14).Alpha(0.80)
			} else {
				cardBg = p.Surface.Mix(tone, 0.08).Alpha(0.88)
			}
		}
		kit.ApplyCardShadow(kit.CardMotion(tab).Background(tab.AnimateColor("m-bg", cardBg, kit.MotionFast)).Border(borderW, border), p)
	}

	summary := metricSummaryText(msgs, channel, stat, compactStats)
	dotColor := tone.Alpha(.65)
	if active {
		dotColor = tone
	}

	tab.Children(func() {
		if stacked {
			ui.Column(ctx).Grow(1).MinWidth(0).Gap(3).Children(func() {
				metricLabelRow(ctx, dotColor, labelColor, 6, 6, 12, label)
				if summary != "" {
					ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).SingleLine()
				}
			})
			reading(ctx, p, value, channel.unit, fontSize, tone).Shrink(0)
			return
		}
		metricLabelRow(ctx, dotColor, labelColor, 6, 6, 12, label)
		reading(ctx, p, value, channel.unit, fontSize, tone)
		if summary != "" {
			ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).SingleLine()
		}
	})
}

func reading(ctx *ui.Context, p kit.Palette, value, unit string, fontSize float32, tone ui.Color) ui.Element {
	return readingWithUnit(ctx, p, value, unit, fontSize, 12, 5, tone)
}

func readingWithUnit(ctx *ui.Context, p kit.Palette, value, unit string, fontSize, unitSize, gap float32, tone ui.Color) ui.Element {
	return ui.Row(ctx).MinWidth(0).AlignItems(ui.Baseline).Gap(gap).Children(func() {
		ui.Text(ctx, value).FontFeatures("tnum").FontSize(fontSize).FontWeight(700).MinWidth(0).
			SingleLine().TextColor(p.Text)
		ui.Text(ctx, unit).FontSize(unitSize).FontWeight(650).Shrink(0).TextColor(tone)
	})
}
