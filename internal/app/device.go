package app

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

func meterOptionRow(ctx *ui.Context, p kit.Palette, meter device.Info, active, compact bool, onClick func()) {
	identity := meter.Serial
	if identity == "" {
		identity = meter.Path
	}
	rowH, padY, badgeSize := float32(46), float32(8), float32(30)
	if compact {
		rowH, padY, badgeSize = 40, 6, 26
	}
	row := ui.ButtonBase(ctx.Key(meter.ID)).FillWidth().MinHeight(rowH).Padding(padY, 6).Gap(10).
		Radius(kit.RadiusS).AlignItems(ui.Center).Justify(ui.Start).
		Label(meter.Name + " · " + identity).OnClick(onClick)
	bg := kit.InteractiveBg(row, p)
	kit.ControlMotion(row).Background(row.AnimateColor("m-row", bg, kit.MotionFast))
	row.Children(func() {
		bgIcon, fgIcon := p.Inset, p.Muted
		if active {
			bgIcon, fgIcon = p.AccentSoft, p.AccentText
		}
		kit.IconBadge(ctx, kit.IconCpu, badgeSize, bgIcon, fgIcon)
		ui.Column(ctx).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(ctx, meter.Name).FontSize(13).FontWeight(650).SingleLine()
			if identity != "" {
				ui.Text(ctx, identity).FontSize(11).TextColor(p.Muted).SingleLine()
			}
		})
		if active {
			ui.Icon(ctx, kit.IconCheck).FontSize(15).TextColor(p.Green).Shrink(0)
		} else {
			ui.Icon(ctx, kit.IconChevronR).FontSize(15).TextColor(p.Muted).Shrink(0)
		}
	})
}

func (d *dashboard) meterRows(ctx *ui.Context, p kit.Palette, snap backend.Snapshot, compact bool, onPick func()) {
	for index, meter := range d.meters {
		if index > 0 {
			kit.Divider(ctx, p)
		}
		chosen := meter
		active := chosen.ID == snap.Device.ID && snap.State == backend.Streaming
		meterOptionRow(ctx, p, chosen, active, compact, func() {
			d.source.Device = chosen.ID
			d.connect()
			if onPick != nil {
				onPick()
			}
		})
	}
}

func (d *dashboard) deviceSwitcherPopover(ctx *ui.Context, p kit.Palette, msgs locales.Messages, anchor ui.Element) {
	touch := d.touchUI()
	_, height := ctx.Size()
	topInset, _, bottomInset, _ := ctx.SafeArea()
	maxH := max(140, min(height-topInset-bottomInset-84, max(220, height*0.65)))
	kit.Popover(ctx, p, &d.deviceMenuOpen, anchor, func(panel ui.Element) {
		panel.Width(288).MaxWidthPercent(92).MaxHeight(maxH).Padding(12).Gap(8)
		panel.Children(func() {
			d.deviceSwitcherContent(ctx, p, msgs, touch)
		})
	})
}

func (d *dashboard) connectButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, key string, snap backend.Snapshot, reconnectOnStreaming, touch bool, onClick func()) ui.Element {
	connectLabel := msgs.GuiConnect()
	if snap.State == backend.Streaming && reconnectOnStreaming {
		connectLabel = msgs.GuiReconnect()
	}
	return kit.ActionButton(ctx.Key(key), p, kit.VariantPrimary, kit.IconPlay, connectLabel, touch, onClick).
		Disabled(d.busy || d.connectDisabled || (!d.reconnecting && snap.State == backend.Connecting))
}

func (d *dashboard) deviceSwitcherContent(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	var snap backend.Snapshot
	if d.reconnecting && d.reconnectHold.service == d.service && d.reconnectHold.snap.Device.ID != "" {
		snap = d.reconnectHold.snap
	} else if d.service != nil {
		snap = d.service.Snapshot(0)
	}
	ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(ctx, msgs.GuiDevice()).FontSize(13).FontWeight(700)
		ui.Spacer(ctx)
		kit.ActionButton(ctx.Key("pop-scan"), p, kit.VariantGhost, kit.IconRefresh, msgs.GuiFindMeters(), false, func() {
			d.scanMeters(msgs, false)
		}).Height(28).Padding(0, 8).Disabled(d.busy)
	})
	kit.Divider(ctx, p)
	buildList := func() {
		switch {
		case len(d.meters) > 0:
			d.meterRows(ctx, p, snap, !touch, func() { d.deviceMenuOpen = false })
		case snap.Device.ID != "":
			meterOptionRow(ctx, p, snap.Device, snap.State == backend.Streaming, !touch, func() {
				d.deviceMenuOpen = false
			})
		default:
			status := msgs.GuiNoDevices()
			if d.busy {
				status = msgs.Connecting()
			}
			ui.Text(ctx, status).FillWidth().Padding(8, 2).FontSize(13).TextColor(p.Muted).Wrap()
		}
	}
	ui.Scroll(ctx.Key("dev-pop-scroll")).Scrollbars(false).MaxHeight(260).MinHeight(0).FillWidth().Children(buildList)
	kit.Divider(ctx, p)
	ui.Row(ctx).FillWidth().Gap(8).Justify(ui.End).Children(func() {
		btn := d.connectButton(ctx, p, msgs, "pop-connect", snap, true, touch, func() {
			d.deviceMenuOpen = false
			d.connect()
		})
		if touch {
			btn.FillWidth()
		} else {
			btn.Height(28)
		}
	})
}

func deviceCard(ctx *ui.Context, p kit.Palette, key, title string, trailing, body func()) ui.Element {
	return kit.Card(ctx.Key(key), p).Padding(10, 14).Gap(6).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(24).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(ctx, title).FontSize(12.5).FontWeight(700).TextColor(p.Text).SingleLine().Role(ui.RoleHeading).Level(2)
			ui.Spacer(ctx)
			if trailing != nil {
				trailing()
			}
		})
		kit.Divider(ctx, p)
		ui.Column(ctx).FillWidth().MinWidth(0).Children(body)
	})
}

func deviceContentColumn(ctx *ui.Context, build func()) ui.Element {
	return ui.Column(ctx).FillWidth().MaxWidth(1120).AlignSelf(ui.Center).MinWidth(0).Gap(10).Children(build)
}

func (d *dashboard) devicePage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot) {
	touch := d.touchUI()
	info := snap.Device
	title := info.Name
	if title == "" {
		title = msgs.GuiNoMeter()
	}
	status, detail, statusBadgeTone := dashboardStatus(msgs, snap, d.err)
	sub := strings.Trim(info.Driver+" · "+info.Transport, " ·")
	if info.Simulated {
		sub = ""
	}

	renderActions := func() {
		if runtime.GOOS == "linux" && snap.ErrorKind == "permission" {
			kit.InlineMotion(ctx, kit.ActionButton(ctx.Key("meter-permissions"), p, kit.VariantSecondary, kit.IconShield, msgs.GuiMeterAccess(), touch, func() {
				d.goPage(pageSettings)
			}).Height(34).Grow(1))
		}
		d.connectButton(ctx, p, msgs, "connect", snap, !snap.Device.Simulated, touch, d.connect).Height(34).Grow(1)
		kit.ActionButton(ctx.Key("scan"), p, kit.VariantSecondary, kit.IconRefresh, msgs.GuiFindMeters(), touch, func() {
			d.scanMeters(msgs, false)
		}).Disabled(d.busy).Height(34).Grow(1)
	}

	pathRow := info.Path
	if pathRow == "" && info.ID != "" && info.ID != info.Driver {
		pathRow = info.ID
	}
	hasHardwareInfo := info.Serial != "" || pathRow != ""
	hasExtraError := snap.Error != "" && snap.Error != detail

	showMeters := len(d.meters) > 0
	showInfo := hasHardwareInfo || hasExtraError
	showPD := info.ID != "" && !info.Simulated
	showTelemetry := snap.State == backend.Streaming && hasTelemetry(snap.Latest)
	connectedRight := showPD || showTelemetry
	metersOnLeft := showPD && showTelemetry
	hasRight := connectedRight || showMeters || showInfo

	renderMainCard := func() {
		kit.Card(ctx, p).Padding(12, 14).Gap(10).Children(func() {
			ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
				kit.IconBadge(ctx, kit.IconCpu, 38, p.AccentSoft, p.AccentText)
				ui.Column(ctx).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(6).Children(func() {
						ui.Text(ctx, title).FontSize(15).FontWeight(700)
						if status != "" {
							kit.InlineMotion(ctx, kit.Badge(ctx.Key("dev-status"), p, status, statusBadgeTone))
						}
					})
					if sub != "" {
						kit.InlineMotion(ctx, ui.Text(ctx.Key("dev-sub"), sub).FontSize(11.5).TextColor(p.Muted).Wrap())
					}
				})
			})
			if detail != "" {
				kit.SectionMotion(ctx, ui.Text(ctx.Key("dev-detail"), detail).FillWidth().FontSize(11.5).TextColor(p.Muted).Wrap())
			}
			ui.Row(ctx).FillWidth().Wrap().Gap(8).Children(renderActions)
		})
	}
	renderMeters := func() {
		kit.SectionMotion(ctx, deviceCard(ctx, p, "dev-meters", msgs.GuiDiscoveredDevices(), nil, func() {
			d.meterRows(ctx, p, snap, true, nil)
		}))
	}
	renderInfo := func() {
		kit.SectionMotion(ctx, deviceCard(ctx, p, "dev-info", msgs.GuiInfo(), nil, func() {
			items := [2]kit.KVItem{{Label: msgs.SerialHeading(), Value: info.Serial}, {Label: msgs.Interface(), Value: pathRow}}
			kit.KVListCompact(ctx, p, items[:])
			if hasExtraError {
				if hasHardwareInfo {
					kit.Divider(ctx, p)
				}
				ui.Text(ctx, snap.Error).FillWidth().Padding(4, 0).FontSize(12).TextColor(p.Warning).Wrap()
			}
		}))
	}

	deviceContentColumn(ctx, func() {
		ui.Row(ctx).FillWidth().Wrap().Gap(10).AlignItems(ui.Start).Children(func() {
			ui.Column(ctx).Grow(46).Basis(300).MinWidth(270).Gap(10).Children(func() {
				renderMainCard()
				if connectedRight && showInfo {
					renderInfo()
				}
				if metersOnLeft && showMeters {
					renderMeters()
				}
			})
			if hasRight || !d.isPortraitMobile(ctx) {
				ui.Column(ctx).Grow(54).Basis(320).MinWidth(270).Gap(10).Children(func() {
					if connectedRight {
						if showPD {
							kit.SectionMotion(ctx, d.pdCapabilityPanel(ctx, p, msgs, snap, touch))
						}
						if !metersOnLeft && showMeters {
							renderMeters()
						}
						if showTelemetry {
							kit.SectionMotion(ctx, deviceCard(ctx, p, "dev-telemetry", msgs.GuiTelemetry(), nil, func() {
								d.deviceTelemetryGrid(ctx, p, msgs, snap.Latest)
							}))
						}
						return
					}
					if showMeters {
						renderMeters()
					}
					if showInfo {
						renderInfo()
					}
				})
			}
		})
	})
}

func (d *dashboard) deviceSubnav(ctx *ui.Context, p kit.Palette, msgs locales.Messages, position navigationPosition, floatingTop, leftInset, rightInset float32) {
	selected := 0
	if d.page == pageConnectionLog {
		selected = 1
	}
	landscape := d.isLandscape(ctx)
	padTop, padBottom, padX := floatingTop+12, float32(6), float32(20)
	if landscape {
		padTop, padBottom, padX = floatingTop+4, 4, 12
	}
	padStart, padEnd := horizontalContentPadding(d.isPortraitMobile(ctx), landscape, position, padX, leftInset, rightInset)
	ui.Column(ctx.Key("device-subnav")).FillWidth().Shrink(0).Padding(padTop, padEnd, padBottom, padStart).Children(func() {
		deviceContentColumn(ctx, func() {
			ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).AlignContent(ui.Center).Gap(10).Children(func() {
				if !d.mobile {
					ui.Text(ctx, msgs.GuiDevice()).FontSize(18).FontWeight(750).SingleLine().Role(ui.RoleHeading).Level(2)
				}
				kit.SectionTabs(ctx, p, msgs.GuiDevice(), selected, []kit.NavTab{
					{Label: msgs.GuiDeviceInfo(), OnSelect: func() { d.goPage(pageDevice) }},
					{Label: msgs.GuiConnectionLog(), OnSelect: func() { d.goPage(pageConnectionLog) }},
				})
				if d.page == pageConnectionLog {
					entries := d.connectionLogEntries()
					kit.Badge(ctx, p, msgs.GuiItemsCount(len(entries)), kit.TonePlain)
					ui.Spacer(ctx)
					d.connectionLogFilterButton(ctx, p, msgs, d.touchUI(), 30)
				}
			})
		})
	})
}

type connectionLogView struct {
	list     ui.ListState
	revision uint64
	level    slog.Level
	entries  []diagnostics.Entry
}

func (view *connectionLogView) snapshot(logs *diagnostics.Buffer, level slog.Level) []diagnostics.Entry {
	if logs == nil {
		return nil
	}
	revision := logs.Revision()
	if view.revision != revision || view.level != level {
		view.entries = logs.LatestFirst(level)
		view.revision, view.level = revision, level
	}
	return view.entries
}

func (d *dashboard) connectionLogEntries() []diagnostics.Entry {
	if d.snapshotOnly && d.logView.level == d.logLevel && (d.logView.revision != 0 || d.logs == nil) {
		return d.logView.entries
	}
	return d.logView.snapshot(d.logs, d.logLevel)
}

func (d *dashboard) connectionLogFilterButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool, fixedHeight float32) {
	levels := [...]struct {
		minimum slog.Level
		label   string
	}{
		{slog.LevelDebug, msgs.LogFilterDebug()},
		{slog.LevelInfo, msgs.LogFilterInfo()},
		{slog.LevelWarn, msgs.LogFilterWarning()},
		{slog.LevelError, msgs.LogFilterError()},
	}
	items := make([]kit.MenuItem, len(levels))
	for index, option := range levels {
		items[index] = kit.MenuItem{
			Label: option.label, Radio: true, Active: option.minimum == d.logLevel,
			OnSelect: func() {
				d.logLevel = option.minimum
				d.logView.list = ui.ListState{}
			},
		}
	}
	trigger := kit.MenuButton(ctx.Key("log-level"), p, kit.MenuButtonOptions{
		Open:    d.menuOpen("log-level"),
		Variant: kit.VariantSecondary,
		Icon:    kit.IconFilter,
		Label:   msgs.LogLevelFilter(msgs.DiagnosticLevel(d.logLevel)),
		Touch:   touch,
		Items:   items,
	})
	if fixedHeight > 0 {
		trigger.Height(fixedHeight)
	}
}

func (d *dashboard) connectionLogPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, bottomPadding float32) {
	ctx.After(500 * time.Millisecond)
	touch := d.touchUI()
	portraitMobile := d.isPortraitMobile(ctx)
	entries := d.connectionLogEntries()
	deviceContentColumn(ctx, func() {
		ui.Column(ctx).Fill().MinHeight(0).Gap(8).Children(func() {
			if portraitMobile {
				ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
					kit.Badge(ctx, p, msgs.GuiItemsCount(len(entries)), kit.TonePlain)
					ui.Spacer(ctx)
					d.connectionLogFilterButton(ctx, p, msgs, touch, 0)
				})
			}
			card := kit.GlassPanel(ui.Column(ctx).FillWidth().MinHeight(0).Padding(0, 14), p, kit.RadiusL).Grow(1)
			card.Children(func() {
				if len(entries) == 0 {
					title := msgs.LogEmptyLevel()
					if d.logLevel <= slog.LevelInfo {
						title = msgs.GuiNoConnectionLog()
					}
					kit.EmptyState(ctx, p, kit.IconActivity, title, "", nil)
					return
				}
				ui.List(ctx.Key("log-entries"), d.logListState(), len(entries)).FillWidth().Grow(1).MinHeight(0).Scrollbars(false).
					Padding(0, 0, bottomPadding, 0).
					ItemKey(func(index int) any { return entries[index].Sequence }).
					Rows(func(row ui.ListRow) {
						if row.Index > 0 {
							kit.Divider(row.Context, p)
						}
						d.connectionLogRow(row.Context, p, msgs, entries[row.Index], touch)
					})
			})
		})
	}).Grow(1).MinHeight(0)
}

type diagnosticEntryText struct {
	timestamp, level, title, reason, rawError, fields string
}

func formatDiagnosticEntry(msgs locales.Messages, entry diagnostics.Entry) diagnosticEntryText {
	text := diagnosticEntryText{
		timestamp: entry.Time.Local().Format("15:04:05.000"),
		level:     msgs.DiagnosticLevel(entry.Level),
		title:     msgs.DiagnosticTitle(entry.Message),
		rawError:  entry.Error,
	}
	fields := make([]string, 0, len(entry.Fields))
	for _, field := range entry.Fields {
		if field.Key == "kind" {
			text.reason = msgs.DiagnosticError(field.Value)
			continue
		}
		fields = append(fields, msgs.DiagnosticField(field.Key, field.Value))
	}
	if text.reason == "" {
		text.reason, text.rawError = text.rawError, ""
	}
	if text.rawError == text.reason {
		text.rawError = ""
	}
	text.fields = strings.Join(fields, " · ")
	return text
}

func (text diagnosticEntryText) clipboardText() string {
	line := text.timestamp + " [" + text.level + "] " + text.title
	if text.reason != "" {
		line += " — " + text.reason
	}
	if text.rawError != "" {
		line += " (" + text.rawError + ")"
	}
	if text.fields != "" {
		line += " | " + text.fields
	}
	return line
}

func (d *dashboard) connectionLogRow(ctx *ui.Context, p kit.Palette, msgs locales.Messages, entry diagnostics.Entry, touch bool) {
	text := formatDiagnosticEntry(msgs, entry)
	tone := kit.TonePlain
	switch {
	case entry.Level >= slog.LevelError:
		tone = kit.ToneDanger
	case entry.Level >= slog.LevelWarn:
		tone = kit.ToneWarning
	case entry.Level < slog.LevelInfo:
		tone = kit.ToneAccent
	}
	_, levelColor := kit.BadgeColors(p, tone)
	ui.Column(ctx).FillWidth().Shrink(0).Padding(8, 0).Gap(3).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(26).AlignItems(ui.Center).Gap(8).Children(func() {
			if entry.Level == slog.LevelInfo {
				ui.Row(ctx).AlignItems(ui.Center).Gap(6).Shrink(0).Children(func() {
					kit.StatusDot(ctx, p.Muted, 6)
					ui.Text(ctx, text.level).FontSize(11).FontWeight(650).TextColor(p.Muted).SingleLine()
				})
			} else {
				kit.Badge(ctx, p, text.level, tone)
			}
			ui.Text(ctx, text.timestamp).FontSize(11).FontFeatures("tnum").TextColor(p.Muted).SingleLine().Shrink(0)
			ui.Text(ctx, text.title).FontSize(13).FontWeight(650).Grow(1).MinWidth(0).Wrap()
			copyBtn := kit.IconActionButton(ctx.Key("copy"), p, kit.IconCopy, msgs.LogCopyEntry(), 26, func() {
				d.copyText(ctx, msgs, text.clipboardText())
			})
			if !touch && !copyBtn.Hovered() && !copyBtn.Pressed() && !copyBtn.FocusVisible() {
				copyBtn.Opacity(0.58)
			}
		})
		if text.reason != "" {
			weight := 450
			if entry.Level >= slog.LevelWarn {
				weight = 550
			}
			ui.Text(ctx, text.reason).FillWidth().FontSize(11.5).FontWeight(weight).TextColor(levelColor).Wrap().Selectable()
		}
		if text.rawError != "" {
			ui.Box(ctx).FillWidth().Padding(5, 8).Radius(kit.RadiusS).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
				ui.Text(ctx, text.rawError).FillWidth().Font("monospace").FontSize(11).TextColor(p.Muted).Wrap().Selectable()
			})
		}
		if text.fields != "" {
			ui.Text(ctx, text.fields).FillWidth().FontSize(11.5).TextColor(p.Muted).Wrap().Selectable()
		}
	})
}

func (d *dashboard) pdCapabilityPanel(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, touch bool) ui.Element {
	info := snap.Device
	return deviceCard(ctx, p, "dev-pd", msgs.PdHeading(), func() {
		if snap.CanCheckPDHost {
			kit.InlineMotion(ctx, kit.ActionButton(ctx.Key("check"), p, kit.VariantSecondary, kit.IconZap, msgs.PdCheckOnline(), touch, func() {
				d.checkPD(msgs)
			}).Height(26).Padding(0, 9).Disabled(d.busy || d.reconnecting))
		}
	}, func() {
		status, tone := msgs.GuiUnsupported(), kit.TonePlain
		if info.Supports(device.CapPDCapture) {
			status, tone = msgs.GuiSupported(), kit.ToneSuccess
		} else if info.Supports(device.CapPDDirectUSB) {
			status, tone = msgs.PdDirectUsb(), kit.ToneWarning
		}
		kit.SettingRowCompact(ctx, p, msgs.PdCaptureLabel(), "", func() {
			kit.Badge(ctx, p, status, tone)
		})
		if snap.SavedPDHost != nil {
			kit.Divider(ctx, p)
			saved, savedTone := msgs.GuiOff(), kit.TonePlain
			if *snap.SavedPDHost {
				saved, savedTone = msgs.GuiOn(), kit.ToneSuccess
			}
			kit.SettingRowCompact(ctx, p, msgs.GuiPdOnline(), "", func() {
				kit.Badge(ctx, p, saved, savedTone)
			})
		}
	})
}

func hasTelemetry(s *device.Sample) bool {
	if s == nil {
		return false
	}
	return s.WattHours != nil || s.AmpHours != nil || s.RecordMinCurrent != nil ||
		s.Group != nil || s.Uptime != nil || s.RecordSeconds != nil ||
		displayedCurrentDirection(s) != ""
}

func (d *dashboard) deviceTelemetryGrid(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample) {
	var items [7]kit.KVItem
	n := 0
	add := func(label, value string) {
		items[n] = kit.KVItem{Label: label, Value: value}
		n++
	}
	if value, ok := optional(sample.WattHours); ok {
		add(msgs.GuiMeterEnergy(), fmt.Sprintf("%.4f Wh", value))
	}
	if value, ok := optional(sample.AmpHours); ok {
		add(msgs.GuiMeterCharge(), fmt.Sprintf("%.4f Ah", value))
	}
	if value, ok := optional(sample.RecordMinCurrent); ok {
		add(msgs.GuiMinCurrent(), fmt.Sprintf("%.3f mA", value*1000))
	}
	if sample.Group != nil {
		add(msgs.GuiGroup(), strconv.FormatUint(uint64(*sample.Group), 10))
	}
	if sample.Uptime != nil {
		add(msgs.GuiUptime(), formatDuration(float64(*sample.Uptime)))
	}
	if sample.RecordSeconds != nil {
		add(msgs.GuiTimer(), formatDuration(float64(*sample.RecordSeconds)))
	}
	if dir := displayedCurrentDirectionLabel(msgs, sample); dir != "" {
		add(msgs.GuiDirection(), dir)
	}
	if n == 0 {
		return
	}
	renderTile := func(item kit.KVItem) {
		ui.Column(ctx).Grow(1).Basis(0).MinWidth(0).Padding(6, 10).Gap(2).
			Radius(kit.RadiusS).Background(p.Inset).Children(func() {
			ui.Text(ctx, item.Label).FontSize(10.5).FontWeight(500).TextColor(p.Muted).SingleLine()
			ui.Text(ctx, item.Value).FontSize(13).FontWeight(600).FontFeatures("tnum").SingleLine()
		})
	}
	ui.Column(ctx).FillWidth().Padding(2, 0, 0, 0).Gap(6).Children(func() {
		for i := 0; i < n; i += 2 {
			ui.Row(ctx).FillWidth().Gap(6).Children(func() {
				renderTile(items[i])
				if i+1 < n {
					renderTile(items[i+1])
				} else {
					ui.Box(ctx).Grow(1).Basis(0)
				}
			})
		}
	})
}

func (d *dashboard) scanMeters(msgs locales.Messages, silent bool) {
	source := d.source
	d.runJob(func(ctx context.Context) (func(), string, error) {
		registry, err := source.Registry()
		if err != nil {
			diagnostics.Logger(ctx).WarnContext(ctx, diagnostics.ScanFailed, "error", err)
			if silent {
				return nil, "", nil
			}
			return nil, "", err
		}
		meters, err := registry.Discover(ctx, source.Driver)
		message := ""
		if len(meters) == 0 && !silent {
			message = msgs.GuiNoDevices()
		}
		return func() { d.meters = meters }, message, err
	})
}
