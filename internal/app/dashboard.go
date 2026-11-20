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
	chartMaxLivePts  = 2400
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
	maxWin := d.maxWindowFor(sessionSpan)
	if d.fullSession {
		return max(minWindowSec, sessionSpan)
	}
	if d.windowSec > 0 {
		return clampFloat(d.windowSec, minWindowSec, maxWin)
	}
	return defaultWindowSec
}

func (d *dashboard) resumeLive() {
	d.held = nil
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

func (d *dashboard) frameTheme(ctx *ui.Context) (*ui.Theme, kit.Palette, locales.Messages) {
	if d.mobile {
		d.notifyLanguage.Store(d.locale())
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
	return theme, p, locales.In(d.locale())
}

func (d *dashboard) view(ctx *ui.Context) {
	defer kit.BeginFrame(ctx)()

	d.drainUIQueue()
	d.applyThemeChanges(ctx)
	_, p, msgs := d.frameTheme(ctx)
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

	stage := ui.Box(ctx.Key("theme-stage")).Fill()
	stage.Children(func() {
		if reveal != nil {
			oldTheme, oldP := kit.ResolveTheme(!p.Dark, d.settings.Accent)
			oldP.LiquidGlass = p.LiquidGlass
			oldSnap := d.snapshotForReveal(!p.Dark)
			ui.Box(ctx.Key("theme-reveal-alternate")).Absolute().Fill().ZIndex(0).
				PassThrough().IgnoreInput(true).AccessHidden(true).Cache().Effect(alternate).Children(func() {
				defer kit.BeginStaticSnapshot()()
				ui.Themed(ctx, oldTheme).Fill().Children(func() {
					oldSnap.draw(ctx, oldP, msgs, snapshot)
				})
			})
		}
		host := ui.Box(ctx.Key("workspace-host")).Fill().ZIndex(1)
		if reveal != nil {
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
	position := navigationPlacement(d.mobile, d.settings.Navigation)
	if position == navigationSide {
		width -= sidebarWidth(width)
	}
	width -= 4 * workspacePadding
	if position == navigationBottom {
		height -= kit.BottomNavHeightFor(d.bottomBarStyle()) + mobileHeaderHeight + 16
	} else {
		height -= headerHeight
	}

	status, notice, tone := dashboardStatus(p, msgs, snapshot, d.err)
	name := snapshot.Device.Name
	switch {
	case snapshot.Device.Simulated:
		name = msgs.SimulatedMeter()
	case name == "":
		name = msgs.GuiNoMeter()
	}

	pages := [...]workspacePage{
		pageOverview: {
			label:   msgs.GuiOverview(),
			icon:    kit.IconActivity,
			content: func() { d.overview(ctx, p, msgs, snapshot, width, height, notice) },
		},
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
	d.measurementPanel(ctx, p, msgs, snapshot, live, chartSample, readoutSample, readoutStats, metricStats, cw, width, height, notice)
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

func (d *dashboard) mobileMetricsCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample, metricStats [3]channelStat, fontSize float32) {
	cell := func(index int, wide bool) ui.Element {
		channel := channels[index]
		label := channel.name(msgs)
		tone := channel.color(p)
		value := channel.format(sample)
		active := d.channel == index
		stat := metricStats[index]
		summary := metricSummaryText(msgs, channel, stat, true)

		btn := ui.ButtonBase(ctx.Key(index)).Label(label + " " + value + " " + channel.unit).
			Role(ui.RoleTab).AccessSelected(active).Background(ui.Transparent).Radius(kit.RadiusM).MinWidth(0).
			OnClick(func() { d.selectChartChannel(index) })
		if wide {
			btn.FillWidth().Column().AlignItems(ui.Start).Padding(13, 14).Gap(6)
		} else {
			btn.Grow(1).Basis(0).Column().AlignItems(ui.Start).Padding(11, 13).Gap(3)
		}

		activeMix, pressMix, activeBorderAlpha := float32(.09), float32(.13), float32(.50)
		if p.Dark {
			activeMix, pressMix, activeBorderAlpha = .15, .20, .46
		}
		bg, border, borderW, labelColor := p.Inset, p.LineSoft.Alpha(.65), float32(1), p.Muted
		if active {
			bg = p.Surface.Mix(tone, activeMix)
			if btn.Pressed() {
				bg = p.Surface.Mix(tone, pressMix)
			}
			border = tone.Alpha(activeBorderAlpha)
			borderW = 1.25
			labelColor = tone
		} else if btn.Pressed() {
			bg = p.SurfaceHi.Mix(tone, .09)
			border = tone.Alpha(.38)
			labelColor = p.Text
		} else if btn.Hovered() || btn.FocusVisible() {
			bg = p.SurfaceHi
			border = p.Line
			labelColor = p.Text
		}

		btn.Children(func() {
			var cellTint ui.Color
			if active {
				cellTint = tone.Alpha(0.14)
			}
			kit.GlassPillControl(ctx.Key("bg"), btn, p, kit.RadiusM, cellTint, true, bg, borderW, border, 0.985)
			dotColor := tone.Alpha(.60)
			if active {
				dotColor = tone
			}
			if wide {
				ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Justify(ui.SpaceBetween).Children(func() {
					ui.Row(ctx).AlignItems(ui.Center).Gap(6).Shrink(0).Children(func() {
						kit.StatusDot(ctx, dotColor, 6.5)
						ui.Text(ctx, label).FontSize(12).FontWeight(650).TextColor(labelColor).SingleLine()
					})
					if summary != "" {
						ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).MinWidth(0).SingleLine()
					}
				})
				heroSize := min(27, max(24, fontSize+4))
				reading(ctx, p, value, channel.unit, heroSize, tone)
				return
			}
			ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
				kit.StatusDot(ctx, dotColor, 6.5)
				ui.Text(ctx, label).FontSize(12).FontWeight(650).TextColor(labelColor).SingleLine()
			})
			reading(ctx, p, value, channel.unit, max(19, fontSize-4), tone)
			if summary != "" {
				ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).SingleLine()
			}
		})
		return btn
	}

	kit.Card(ctx, p).Radius(kit.RadiusL + 2).Padding(5).Gap(5).Children(func() {
		cell(0, true)
		ui.Row(ctx).FillWidth().Gap(5).AlignItems(ui.Stretch).Children(func() {
			cell(1, false)
			cell(2, false)
		})
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
	if active {
		bg = p.Surface.Mix(tone, .07)
		border = tone.Alpha(.65)
		borderW = 1.5
		labelColor = tone
	} else if tab.Pressed() {
		bg = p.SurfaceHi
		border = p.Line
		labelColor = p.Text
	} else if tab.Hovered() || tab.FocusVisible() {
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

	tab.Children(func() {
		if stacked {
			ui.Column(ctx).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
					dotColor := tone.Alpha(.65)
					if active {
						dotColor = tone
					}
					kit.StatusDot(ctx, dotColor, 6)
					ui.Text(ctx, label).FontSize(12).FontWeight(650).TextColor(labelColor).SingleLine()
				})
				if summary != "" {
					ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).SingleLine()
				}
			})
			reading(ctx, p, value, channel.unit, fontSize, tone).Shrink(0)
			return
		}
		ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
			dotColor := tone.Alpha(.65)
			if active {
				dotColor = tone
			}
			kit.StatusDot(ctx, dotColor, 6)
			ui.Text(ctx, label).FontSize(12).FontWeight(650).TextColor(labelColor).SingleLine()
		})
		reading(ctx, p, value, channel.unit, fontSize, tone)
		if summary != "" {
			ui.Text(ctx, summary).FontFeatures("tnum").FontSize(11).TextColor(p.Muted).SingleLine()
		}
	})
}

func reading(ctx *ui.Context, p kit.Palette, value, unit string, fontSize float32, tone ui.Color) ui.Element {
	return ui.Row(ctx).MinWidth(0).AlignItems(ui.Baseline).Gap(5).Children(func() {
		ui.Text(ctx, value).FontFeatures("tnum").FontSize(fontSize).FontWeight(700).MinWidth(0).
			SingleLine().TextColor(p.Text)
		ui.Text(ctx, unit).FontSize(12).FontWeight(650).Shrink(0).TextColor(tone)
	})
}
