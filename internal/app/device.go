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
		Disabled(d.busy || snap.State == backend.Connecting)
}

func (d *dashboard) deviceSwitcherContent(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	var snap backend.Snapshot
	if d.service != nil {
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

	kit.NarrowColumn(ctx, func() {
		kit.Card(ctx, p).Gap(12).Children(func() {
			ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(12).Children(func() {
				kit.IconBadge(ctx, kit.IconCpu, 44, p.AccentSoft, p.AccentText)
				ui.Column(ctx).Grow(1).Basis(180).MinWidth(140).Gap(3).Children(func() {
					ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(ctx, title).FontSize(16).FontWeight(700)
						if status != "" {
							kit.Badge(ctx, p, status, statusBadgeTone)
						}
					})
					if sub != "" {
						ui.Text(ctx, sub).FontSize(12).TextColor(p.Muted).Wrap()
					}
				})
			})
			if detail != "" {
				ui.Text(ctx, detail).FillWidth().FontSize(12).TextColor(p.Muted).Wrap()
			}
			kit.ActionBar(ctx).Children(func() {
				if runtime.GOOS == "linux" && snap.ErrorKind == "permission" {
					kit.ActionButton(ctx.Key("meter-permissions"), p, kit.VariantSecondary, kit.IconShield, msgs.GuiMeterAccess(), touch, func() {
						d.goPage(pageSettings)
					})
				}
				conn := d.connectButton(ctx, p, msgs, "connect", snap, !snap.Device.Simulated, touch, d.connect)
				scan := kit.ActionButton(ctx.Key("scan"), p, kit.VariantSecondary, kit.IconRefresh, msgs.GuiFindMeters(), touch, func() {
					d.scanMeters(msgs, false)
				}).Disabled(d.busy)
				if d.mobile {
					conn.Grow(1)
					scan.Grow(1)
				}
			})
		})

		if len(d.meters) > 0 {
			kit.FormGroup(ctx, p, msgs.GuiDiscoveredDevices(), nil, func() {
				d.meterRows(ctx, p, snap, false, nil)
			})
		}

		pathRow := info.Path
		if pathRow == "" && info.ID != "" && info.ID != info.Driver {
			pathRow = info.ID
		}
		if info.Serial != "" || pathRow != "" || (snap.Error != "" && snap.Error != detail) {
			kit.FormGroup(ctx, p, msgs.GuiInfo(), nil, func() {
				items := [2]kit.KVItem{{Label: msgs.SerialHeading(), Value: info.Serial}, {Label: msgs.Interface(), Value: pathRow}}
				kit.KVList(ctx, p, items[:], d.mobile)
				if snap.Error != "" && snap.Error != detail {
					if info.Serial != "" || pathRow != "" {
						kit.Divider(ctx, p)
					}
					ui.Text(ctx, snap.Error).FillWidth().Padding(8, 0).FontSize(12).TextColor(p.Warning).Wrap()
				}
			})
		}

		if info.ID != "" && !info.Simulated {
			d.pdCapabilityPanel(ctx, p, msgs, snap, touch)
		}

		if snap.State == backend.Streaming && hasTelemetry(snap.Latest) {
			kit.FormGroup(ctx, p, msgs.GuiTelemetry(), nil, func() {
				ui.Column(ctx).FillWidth().Children(func() {
					d.deviceTelemetry(ctx, p, msgs, snap.Latest)
				})
			})
		}
	})
}

func (d *dashboard) deviceSubnav(ctx *ui.Context, p kit.Palette, msgs locales.Messages, position navigationPosition, floatingTop, leftInset, rightInset float32) {
	selected := 0
	if d.page == pageConnectionLog {
		selected = 1
	}
	landscape := d.isLandscape(ctx)
	padTop, padX := floatingTop+16, float32(20)
	if landscape {
		padTop, padX = floatingTop+10, 12
	}
	padStart, padEnd := horizontalContentPadding(d.isPortraitMobile(ctx), landscape, position, padX, leftInset, rightInset)
	ui.Column(ctx.Key("device-subnav")).FillWidth().Shrink(0).Padding(padTop, padEnd, 6, padStart).Children(func() {
		kit.NarrowColumn(ctx, func() {
			ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(12).Children(func() {
				if !d.mobile {
					ui.Text(ctx, msgs.GuiDevice()).FontSize(18).FontWeight(750).SingleLine().Role(ui.RoleHeading).Level(2)
				}
				kit.SectionTabs(ctx, p, msgs.GuiDevice(), selected, []kit.NavTab{
					{Label: msgs.GuiDeviceInfo(), OnSelect: func() { d.goPage(pageDevice) }},
					{Label: msgs.GuiConnectionLog(), OnSelect: func() { d.goPage(pageConnectionLog) }},
				})
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

func (d *dashboard) connectionLogPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, bottomPadding float32) {
	ctx.After(500 * time.Millisecond)
	touch := d.touchUI()
	entries := d.logView.snapshot(d.logs, d.logLevel)
	kit.NarrowColumn(ctx, func() {
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
		ui.Column(ctx).Fill().MinHeight(0).Gap(8).Children(func() {
			ui.Row(ctx).FillWidth().Wrap().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Row(ctx).AlignItems(ui.Center).Gap(8).Children(func() {
					if !d.isPortraitMobile(ctx) {
						ui.Text(ctx, msgs.GuiConnectionLog()).FontSize(13).FontWeight(650).
							TextColor(p.Text).Role(ui.RoleHeading).Level(2).SingleLine()
					}
					kit.Badge(ctx, p, msgs.GuiItemsCount(len(entries)), kit.TonePlain)
				})
				ui.Spacer(ctx)
				trigger := kit.MenuButton(ctx.Key("log-level"), p, kit.MenuButtonOptions{
					Open:    d.menuOpen("log-level"),
					Variant: kit.VariantSecondary,
					Icon:    kit.IconFilter,
					Label:   msgs.LogLevelFilter(msgs.DiagnosticLevel(d.logLevel)),
					Touch:   touch,
					Items:   items,
				})
				if !touch {
					trigger.Height(28)
				}
			})
			card := kit.GlassPanel(ui.Column(ctx).FillWidth().MinHeight(0).Padding(0, 16), p, kit.RadiusL)
			if len(entries) > 0 {
				card.Grow(1)
			}
			card.Children(func() {
				if len(entries) == 0 {
					title := msgs.LogEmptyLevel()
					if d.logLevel <= slog.LevelInfo {
						title = msgs.GuiNoConnectionLog()
					}
					kit.EmptyState(ctx, p, kit.IconActivity, title, "", nil)
					return
				}
				ui.List(ctx.Key("log-entries"), &d.logView.list, len(entries)).FillWidth().Grow(1).MinHeight(0).Scrollbars(false).
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
	copySize, padY := float32(26), float32(11)
	if touch {
		copySize, padY = 32, 12
	}
	ui.Column(ctx).FillWidth().Shrink(0).Padding(padY, 0).Gap(4).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(copySize).AlignItems(ui.Center).Gap(8).Children(func() {
			if entry.Level == slog.LevelInfo {
				ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
					kit.StatusDot(ctx, p.Muted, 6)
					ui.Text(ctx, text.level).FontSize(11).FontWeight(650).TextColor(p.Muted).SingleLine()
				})
			} else {
				kit.Badge(ctx, p, text.level, tone)
			}
			ui.Text(ctx, text.timestamp).FontSize(11).FontFeatures("tnum").TextColor(p.Muted).SingleLine()
			ui.Spacer(ctx)
			copyBtn := kit.IconActionButton(ctx.Key("copy"), p, kit.IconCopy, msgs.LogCopyEntry(), copySize, func() {
				d.copyText(ctx, msgs, text.clipboardText())
			})
			if !touch && !copyBtn.Hovered() && !copyBtn.Pressed() && !copyBtn.FocusVisible() {
				copyBtn.Opacity(0.58)
			}
		})
		ui.Text(ctx, text.title).FillWidth().FontSize(13).FontWeight(650).Wrap()
		if text.reason != "" {
			weight := 450
			if entry.Level >= slog.LevelWarn {
				weight = 550
			}
			ui.Text(ctx, text.reason).FillWidth().FontSize(12).FontWeight(weight).TextColor(levelColor).Wrap().Selectable()
		}
		if text.rawError != "" {
			ui.Box(ctx).FillWidth().Padding(7, 10).Radius(kit.RadiusS).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
				ui.Text(ctx, text.rawError).FillWidth().Font("monospace").FontSize(11).TextColor(p.Muted).Wrap().Selectable()
			})
		}
		if text.fields != "" {
			ui.Text(ctx, text.fields).FillWidth().FontSize(11).TextColor(p.Muted).Wrap().Selectable()
		}
	})
}

func (d *dashboard) pdCapabilityPanel(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, touch bool) {
	info := snap.Device
	kit.FormGroup(ctx, p, msgs.PdHeading(), func() {
		if snap.CanCheckPDHost {
			kit.ActionButton(ctx.Key("check"), p, kit.VariantSecondary, kit.IconZap, msgs.PdCheckOnline(), touch, func() {
				d.checkPD(msgs)
			}).Height(28).Padding(0, 10).Disabled(d.busy)
		}
	}, func() {
		status, tone := msgs.GuiUnsupported(), kit.TonePlain
		if info.Supports(device.CapPDCapture) {
			status, tone = msgs.GuiSupported(), kit.ToneSuccess
		} else if info.Supports(device.CapPDDirectUSB) {
			status, tone = msgs.PdDirectUsb(), kit.ToneWarning
		}
		kit.SettingRow(ctx, p, msgs.PdCaptureLabel(), "", func() {
			kit.Badge(ctx, p, status, tone)
		})
		if snap.SavedPDHost != nil {
			kit.Divider(ctx, p)
			saved, savedTone := msgs.GuiOff(), kit.TonePlain
			if *snap.SavedPDHost {
				saved, savedTone = msgs.GuiOn(), kit.ToneSuccess
			}
			kit.SettingRow(ctx, p, msgs.GuiPdOnline(), "", func() {
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

func (d *dashboard) deviceTelemetry(ctx *ui.Context, p kit.Palette, msgs locales.Messages, sample *device.Sample) {
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
	kit.KVList(ctx, p, items[:n], d.mobile)
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
