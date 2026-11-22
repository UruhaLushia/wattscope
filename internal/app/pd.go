package app

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/pdview"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

type packetView struct {
	search        string
	hideGoodCRC   bool
	hideKeepAlive bool
	selected      uint64
	row           int
	list          ui.ListState
	held          *backend.PacketSnapshot
	offline       *backend.PacketSnapshot
	path          string
	expanded      bool
	detail        pd.Packet
	live          backend.PacketSnapshot
	filter        packetFilterKey
	visible       []pd.Packet
	summaryCache  packetSummaryCache
}

type packetFilterKey struct {
	first         *pd.Packet
	count         int
	messages      locales.Messages
	query         string
	hideGoodCRC   bool
	hideKeepAlive bool
}

type packetSummaryKey struct {
	offline     bool
	offlinePtr  *backend.PacketSnapshot
	offlinePath string
	count       int
	firstSeq    uint64
	lastSeq     uint64
}

type packetSummaryCache struct {
	valid       bool
	key         packetSummaryKey
	summary     pdLiveSummary
	capsSummary string
}

func (view *packetView) visiblePackets(msgs locales.Messages, packets []pd.Packet) []pd.Packet {
	key := packetFilterKey{
		count: len(packets), messages: msgs, query: view.search,
		hideGoodCRC: view.hideGoodCRC, hideKeepAlive: view.hideKeepAlive,
	}
	if len(packets) > 0 {
		key.first = &packets[0]
	}
	if view.filter != key {
		view.filter = key
		view.visible = pdview.FilterPackets(msgs, packets, view.search, view.hideGoodCRC, view.hideKeepAlive)
	}
	return view.visible
}

func (view *packetView) summaryKey(packets []pd.Packet) packetSummaryKey {
	key := packetSummaryKey{
		offline:     view.offline != nil,
		offlinePtr:  view.offline,
		offlinePath: view.path,
		count:       len(packets),
	}
	if len(packets) > 0 {
		key.firstSeq = packets[0].Sequence
		key.lastSeq = packets[len(packets)-1].Sequence
	}
	return key
}

func (view *packetView) summaryState(packets []pd.Packet) (pdLiveSummary, string) {
	if len(packets) == 0 {
		view.summaryCache = packetSummaryCache{valid: true, key: view.summaryKey(nil)}
		return pdLiveSummary{}, ""
	}
	key := view.summaryKey(packets)
	if view.summaryCache.valid && view.summaryCache.key == key {
		return view.summaryCache.summary, view.summaryCache.capsSummary
	}
	summary := summarizePDState(packets)
	capsText := pdCapabilitiesSummary(summary.caps)
	view.summaryCache = packetSummaryCache{
		valid:       true,
		key:         key,
		summary:     summary,
		capsSummary: capsText,
	}
	return summary, capsText
}

func (d *dashboard) packetSnapshot() backend.PacketSnapshot {
	view := &d.packets
	if view.offline != nil {
		return *view.offline
	}
	if view.held != nil {
		return *view.held
	}
	if d.service != nil {
		data := d.service.PacketSnapshot(0, 0)
		if data.Oldest != view.live.Oldest || data.Total != view.live.Total {
			view.live = d.service.PacketSnapshot(0, -1)
			return view.live
		}
		data.Packets = view.live.Packets
		return data
	}
	return backend.PacketSnapshot{}
}

type pdLiveSummary struct {
	caps          *pd.SourceCapabilities
	capsSeq       uint64
	contract      *pd.Request
	contractSeq   uint64
	contractStage string
}

func summarizePDState(packets []pd.Packet) pdLiveSummary {
	var out pdLiveSummary
	for i := range slices.Backward(packets) {
		pkt := &packets[i]
		if pkt.Error != "" {
			continue
		}
		if out.caps == nil {
			if caps := pkt.AdvertisedSource(); caps != nil {
				out.caps = caps
				out.capsSeq = pkt.Sequence
			} else if pkt.Analysis != nil && pkt.Analysis.SourceCapabilities != nil {
				out.caps = pkt.Analysis.SourceCapabilities
				out.capsSeq = pkt.Analysis.CapabilitiesSequence
			}
		}
		if out.contract == nil {
			switch {
			case pkt.Request != nil:
				out.contract = pkt.Request
				out.contractSeq = pkt.Sequence
				if pkt.Analysis != nil && pkt.Analysis.Stage != "" {
					out.contractStage = pkt.Analysis.Stage
				} else {
					out.contractStage = "requested"
				}
			case pkt.Analysis != nil && pkt.Analysis.Request != nil:
				out.contract = pkt.Analysis.Request
				out.contractSeq = pkt.Analysis.RequestSequence
				if out.contractSeq == 0 {
					out.contractSeq = pkt.Sequence
				}
				out.contractStage = pkt.Analysis.Stage
			}
		}
		if out.caps != nil && out.contract != nil {
			break
		}
	}
	return out
}

func pdCapabilitiesSummary(caps *pd.SourceCapabilities) string {
	if caps == nil {
		return ""
	}
	features := []string{"PD " + caps.HeaderRevision}
	if caps.PPS {
		features = append(features, "PPS")
	}
	if caps.SPRAVS {
		features = append(features, "SPR AVS")
	}
	if caps.EPR {
		features = append(features, "EPR")
	}
	if caps.FixedMaxW > 0 {
		features = append(features, fmt.Sprintf("%.4g W", caps.FixedMaxW))
	}
	return strings.Join(features, " · ")
}

// findPacketBySequence binary-searches packets ordered by strictly increasing Sequence.
func findPacketBySequence(packets []pd.Packet, seq uint64) int {
	idx := sort.Search(len(packets), func(i int) bool {
		return packets[i].Sequence >= seq
	})
	if idx < len(packets) && packets[idx].Sequence == seq {
		return idx
	}
	return -1
}

func (d *dashboard) savePDFilters() {
	d.settings.HideGoodCRC = d.packets.hideGoodCRC
	d.settings.HideKeepAlive = d.packets.hideKeepAlive
	d.saveSettings()
}

func (d *dashboard) pdFollowLive() {
	d.packets.selected = 0
	d.packets.held = nil
	d.packets.expanded = false
	d.packets.list.ScrollToEnd()
}

func (d *dashboard) selectPDPacket(msgs locales.Messages, visible, all []pd.Packet, seq uint64, splitDetail bool) {
	if seq == 0 {
		return
	}
	view := &d.packets
	if idx := findPacketBySequence(visible, seq); idx >= 0 {
		if splitDetail {
			view.selected = seq
			view.row = idx
			view.list.Selected = &view.row
			view.list.Choose(idx)
			view.list.ScrollIntoView(idx)
		} else {
			view.detail = visible[idx]
			view.expanded = true
		}
		return
	}
	if idx := findPacketBySequence(all, seq); idx >= 0 {
		pkt := all[idx]
		if splitDetail {
			filterChanged := false
			view.search = ""
			if pkt.GoodCRC() && view.hideGoodCRC {
				view.hideGoodCRC = false
				filterChanged = true
			}
			if pkt.KeepAlive() && view.hideKeepAlive {
				view.hideKeepAlive = false
				filterChanged = true
			}
			if filterChanged {
				d.savePDFilters()
			}
			view.selected = seq
			visible = view.visiblePackets(msgs, all)
			if newIdx := findPacketBySequence(visible, seq); newIdx >= 0 {
				view.row = newIdx
				view.list.Selected = &view.row
				view.list.Choose(newIdx)
				view.list.ScrollIntoView(newIdx)
			}
		} else {
			view.detail = pkt
			view.expanded = true
		}
	}
}

func (d *dashboard) pdMoreMenuItems(msgs locales.Messages, snap backend.Snapshot, data backend.PacketSnapshot, visible []pd.Packet, compact, landscape bool) []kit.MenuItem {
	view := &d.packets
	var items []kit.MenuItem
	if view.offline == nil || compact || landscape {
		items = append(items, kit.MenuItem{
			Label:    msgs.GuiOpenCapture(),
			Icon:     kit.IconFolder,
			Disabled: d.busy,
			OnSelect: func() { d.openCapture(msgs) },
		})
	}
	if view.offline == nil && snap.CanCheckPDHost {
		items = append(items, kit.MenuItem{
			Label:    msgs.PdCheckOnline(),
			Icon:     kit.IconZap,
			Disabled: d.busy,
			OnSelect: func() { d.checkPD(msgs) },
		})
	}
	if view.offline == nil && (view.selected != 0 || view.held != nil) {
		items = append(items, kit.MenuItem{
			Label:    msgs.PdLive(),
			Icon:     kit.IconActivity,
			OnSelect: d.pdFollowLive,
		})
	}
	if len(visible) > 0 {
		items = append(items, kit.MenuItem{
			Label:    msgs.ExportPackets(),
			Icon:     kit.IconDownload,
			Disabled: d.busy,
			OnSelect: func() { d.exportPackets(msgs, visible) },
		})
	}
	if len(data.Packets) > 0 {
		items = append(items, kit.MenuItem{
			Label:  msgs.ActionClear(),
			Icon:   kit.IconTrash,
			Danger: true,
			OnSelect: func() {
				if view.offline != nil {
					view.offline.Packets = nil
				} else if d.service != nil {
					d.service.ClearPackets()
					view.held = nil
				}
				view.selected = 0
				d.notify(msgs.PacketsCleared(), "info")
			},
		})
	}
	return items
}

func (d *dashboard) pdFilterChips(ctx *ui.Context, p kit.Palette, msgs locales.Messages, height float32) {
	view := &d.packets
	pdFilterChip(ctx.Key("chip-goodcrc"), p, msgs.GuiHideGoodCrc(), &view.hideGoodCRC, height, d.savePDFilters)
	pdFilterChip(ctx.Key("chip-keepalive"), p, msgs.GuiHideKeepAlive(), &view.hideKeepAlive, height, d.savePDFilters)
}

func (d *dashboard) pdSummaryPill(ctx *ui.Context, p kit.Palette, msgs locales.Messages, all, visible []pd.Packet, summary pdLiveSummary, capsText string, splitDetail bool, pillH float32) {
	reqText := ""
	if summary.contract != nil {
		reqText = summary.contract.Summary()
	}
	if capsText == "" && reqText == "" {
		return
	}
	label := capsText
	if reqText != "" {
		if label != "" {
			label += " · " + reqText
		} else {
			label = reqText
		}
	}
	targetSeq := summary.contractSeq
	if targetSeq == 0 {
		targetSeq = summary.capsSeq
	}
	tone := kit.ToneAccent
	switch summary.contractStage {
	case "ready", "accepted":
		tone = kit.ToneSuccess
	case "rejected", "wait":
		tone = kit.ToneWarning
	}
	bg, fg := kit.BadgeColors(p, tone)
	radius := kit.RadiusS
	if p.LiquidGlass {
		radius = kit.RadiusFull
	}
	btn := ui.ButtonBase(ctx.Key("pd-summary")).Height(pillH).MaxWidthPercent(100).Padding(0, 10).Gap(6).
		Radius(radius).AlignItems(ui.Center).Background(bg).Label(label).
		OnClick(func() { d.selectPDPacket(msgs, visible, all, targetSeq, splitDetail) })
	kit.ControlMotion(btn).Children(func() {
		ui.Icon(ctx, kit.IconZap).FontSize(12).TextColor(fg).Shrink(0)
		ui.Text(ctx, label).FontSize(11.5).FontWeight(650).FontFeatures("tnum").TextColor(fg).SingleLine().MinWidth(0)
	})
}

func (d *dashboard) pdToolbar(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, data backend.PacketSnapshot, visible []pd.Packet, summary pdLiveSummary, capsText string, touch, compact, landscape, splitDetail bool) {
	view := &d.packets
	canReturnLive := view.offline == nil && len(visible) > 0 && (view.held != nil || view.selected != 0 || !view.list.AtEnd())
	renderCount := func(btnH float32) {
		count := msgs.PacketsCount(len(visible))
		if uint64(len(visible)) != data.Total {
			count = msgs.PacketCount(strconv.Itoa(len(visible)), strconv.FormatUint(data.Total, 10))
		}
		ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(ctx, count).FontFeatures("tnum").FontSize(13).FontWeight(700).SingleLine()
			switch {
			case view.offline != nil:
				kit.Badge(ctx, p, msgs.Offline(), kit.ToneAccent)
			case view.held != nil:
				kit.Badge(ctx, p, msgs.Hold(), kit.ToneWarning)
			}
			if canReturnLive {
				kit.ActionButton(ctx.Key("follow-live"), p, kit.VariantPrimary, kit.IconArrowDown, msgs.PdLive(), false, d.pdFollowLive).
					Height(max(24, min(28, btnH))).Padding(0, 8)
			}
		})
	}

	renderActions := func(btnH float32) {
		compactPad := compact || landscape
		applyActionStyle := func(el ui.Element) {
			el.Height(btnH)
			if compactPad {
				el.Padding(0, 9).Gap(5)
			}
		}
		if view.offline != nil {
			if !compact && !landscape {
				applyActionStyle(kit.ActionButton(ctx.Key("open"), p, kit.VariantSecondary, kit.IconFolder, msgs.GuiOpenCapture(), touch, func() {
					d.openCapture(msgs)
				}).Disabled(d.busy))
			}
			applyActionStyle(kit.ActionButton(ctx.Key("return"), p, kit.VariantPrimary, kit.IconClose, msgs.GuiClose(), touch, func() {
				d.packets = packetView{hideGoodCRC: d.settings.HideGoodCRC, hideKeepAlive: d.settings.HideKeepAlive}
			}))
		} else {
			recording := data.Recording
			if d.service != nil && view.held != nil {
				recording = d.service.PacketSnapshot(0, 0).Recording
			}
			applyActionStyle(d.captureButton(ctx, p, msgs, recording, touch))
			label, icon, variant := msgs.ActionHold(), kit.IconPause, kit.VariantSecondary
			if view.held != nil {
				label, icon, variant = msgs.ActionResume(), kit.IconPlay, kit.VariantPrimary
			}
			applyActionStyle(kit.ActionButton(ctx.Key("hold-pd"), p, variant, icon, label, touch, func() {
				if view.held != nil {
					d.pdFollowLive()
				} else {
					view.held = &data
				}
			}).Disabled(len(data.Packets) == 0 && view.held == nil))
		}
		menuOpen := d.menuOpen("pd-more")
		if items := d.pdMoreMenuItems(msgs, snap, data, visible, compact, landscape); len(items) > 0 {
			moreBtn := kit.MenuButton(ctx.Key("more"), p, menuOpen, msgs.GuiMore(), kit.IconMore, kit.VariantSecondary, touch, items).
				Height(btnH)
			if compactPad {
				moreBtn.Padding(0, 8)
			}
		} else {
			*menuOpen = false
		}
	}

	renderSearch := func(searchH, basis float32) {
		wrap := ui.Row(ctx).Gap(6).AlignItems(ui.Center).Grow(1).MinWidth(100)
		if basis > 0 {
			wrap.Basis(basis)
		}
		wrap.Children(func() {
			ui.TextInput(ctx.Key("query"), &view.search).Grow(1).MinWidth(0).Height(searchH).
				Placeholder(msgs.GuiSearchPackets()).Label(msgs.GuiSearchPackets())
			if view.search != "" {
				kit.IconActionButton(ctx.Key("clear-search"), p, kit.IconClose, msgs.ActionClear(), searchH, false, func() {
					view.search = ""
				})
			}
		})
	}

	switch {
	case landscape:
		const barH = float32(30)
		ui.Row(ctx).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			renderCount(barH)
			renderSearch(barH, 120)
			d.pdFilterChips(ctx, p, msgs, barH)
			ui.Row(ctx).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
				renderActions(barH)
			})
			if !splitDetail {
				d.pdSummaryPill(ctx, p, msgs, data.Packets, visible, summary, capsText, splitDetail, 26)
			}
		})
	case compact:
		const actionH = float32(34)
		ui.Row(ctx).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Box(ctx).Shrink(0).Children(func() { renderCount(actionH) })
			ui.Row(ctx).Shrink(0).MaxWidthPercent(100).Wrap().Gap(6).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				renderActions(actionH)
			})
		})
	default:
		ctrlH := kit.ControlHeight(touch)
		kit.ActionBar(ctx).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(ctx).Grow(1).MinWidth(100).Children(func() { renderCount(ctrlH) })
			d.pdSummaryPill(ctx, p, msgs, data.Packets, visible, summary, capsText, splitDetail, 26)
			renderActions(ctrlH)
		})
	}

	if view.path != "" {
		ui.Row(ctx).FillWidth().Padding(5, 10).Gap(8).Radius(kit.RadiusS).Background(p.Inset).Border(1, p.LineSoft).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(ctx, kit.IconFileText).FontSize(13).TextColor(p.Muted).Shrink(0)
			ui.Text(ctx, middleEllipsis(view.path, 64)).Grow(1).MinWidth(0).SingleLine().FontSize(12).
				TextColor(p.Muted).Tooltip(view.path)
		})
	}

	if !landscape {
		searchH := kit.ControlHeight(touch)
		if compact {
			searchH = 36
		}
		ui.Row(ctx).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			renderSearch(searchH, 0)
			if !compact {
				d.pdFilterChips(ctx, p, msgs, searchH)
			}
		})
		if compact {
			ui.Row(ctx).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				d.pdFilterChips(ctx, p, msgs, 28)
				d.pdSummaryPill(ctx, p, msgs, data.Packets, visible, summary, capsText, splitDetail, 28)
			})
		}
	}
}

func (d *dashboard) pdEmptyState(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, data backend.PacketSnapshot, touch, landscape, splitDetail bool, bottomPadding float32) {
	view := &d.packets
	title, hint := msgs.PdWaiting(), ""
	hasFilters := len(data.Packets) > 0
	switch {
	case hasFilters:
		title = msgs.PdNoMatch()
	case view.offline != nil:
		title = msgs.EmptyCapture()
	case snap.Device.Supports(device.CapPDDirectUSB):
		hint = strings.TrimRight(msgs.PdDirectUsb(), ".。")
	case snap.Device.ID != "" && !snap.Device.Supports(device.CapPDCapture):
		hint = msgs.GuiUnsupported()
	case snap.SavedPDHost != nil && !*snap.SavedPDHost:
		hint = strings.TrimRight(msgs.PdSavedOnlineOff(), ".。") + " · " + strings.TrimRight(msgs.PdEnableOnline(), ".。")
	case snap.SavedPDHost != nil && *snap.SavedPDHost:
		hint = strings.TrimRight(msgs.PdSavedOnlineOn(), ".。") + " · " + strings.TrimRight(msgs.PdRetrigger(), ".。")
	case snap.CanCheckPDHost:
		hint = strings.TrimRight(msgs.PdInstruction(), ".。")
	}
	box := kit.GlassPanel(ui.Column(ctx).FillWidth().MinHeight(140), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft).Center()
	if splitDetail || landscape {
		box.Grow(1).MinHeight(0)
		if landscape && bottomPadding > 0 {
			box.Margin(0, 0, bottomPadding, 0)
		}
	} else {
		box.Shrink(0).Padding(16, 12)
	}
	box.Children(func() {
		kit.EmptyState(ctx, p, kit.IconUSB, title, hint, func() {
			if hasFilters {
				kit.ActionButton(ctx.Key("reset-filters"), p, kit.VariantPrimary, kit.IconClose, msgs.ActionClear(), touch, func() {
					view.search = ""
					view.hideGoodCRC = false
					view.hideKeepAlive = false
					d.savePDFilters()
				})
				return
			}
			if view.offline == nil {
				kit.ActionButton(ctx.Key("empty-open"), p, kit.VariantSecondary, kit.IconFolder, msgs.GuiOpenCapture(), touch, func() {
					d.openCapture(msgs)
				}).Disabled(d.busy)
				if snap.CanCheckPDHost {
					kit.ActionButton(ctx.Key("empty-check"), p, kit.VariantPrimary, kit.IconZap, msgs.PdCheckOnline(), touch, func() {
						d.checkPD(msgs)
					}).Disabled(d.busy)
				}
			}
		})
	})
}

func (d *dashboard) pdPacketRow(row ui.ListRow, p kit.Palette, msgs locales.Messages, packet pd.Packet, touch, landscape, splitDetail bool) {
	view := &d.packets
	selected := (splitDetail && row.Selected()) || (!splitDetail && view.expanded && packet.Sequence == view.detail.Sequence)
	color, secondary := p.Text, p.Muted
	rowBg := ui.Transparent
	if !splitDetail {
		rowBg = p.Surface
	}
	switch {
	case selected:
		rowBg = p.Surface.Mix(p.Accent, .11)
		color, secondary = p.Text, p.AccentText
	case row.Pressed():
		rowBg = p.Press
		if !splitDetail {
			rowBg = p.SurfaceHi
		}
	case row.Hovered():
		rowBg = p.Hover
		if !splitDetail {
			rowBg = p.SurfaceHi
		}
	}
	switch {
	case !selected && packet.Error != "":
		color = p.Warning
	case !selected && (packet.GoodCRC() || packet.KeepAlive()):
		color = p.Muted
	}
	rowPadY, rowPadX := float32(7), float32(12)
	switch {
	case landscape && splitDetail:
		rowPadY, rowPadX = 7, 10
	case touch:
		rowPadY, rowPadX = 9, 12
	}
	rowEl := ui.Row(row.Context).FillWidth().Padding(rowPadY, rowPadX).Gap(8).AlignItems(ui.Center).
		Radius(kit.RadiusM).Background(rowBg)
	if selected {
		rowEl.Clip()
	}
	if !splitDetail {
		borderColor := p.LineSoft
		if selected {
			borderColor = p.AccentLine
		}
		rowEl.Border(1, borderColor)
	}
	if rowEl.RowClicked() && !splitDetail {
		view.detail = packet
		view.expanded = true
	}
	rowEl.Children(func() {
		if selected {
			ui.Box(row.Context).Absolute().Left(0).Top(4).Bottom(4).Width(3).Radius(kit.RadiusFull).Background(p.Accent).PassThrough()
		}
		ui.Column(row.Context).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(row.Context).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(row.Context, fmt.Sprintf("#%d", packet.Sequence)).
					FontFeatures("tnum").Font("monospace").FontSize(11).TextColor(secondary).Shrink(0)
				ui.Text(row.Context, packet.Type).
					SingleLine().FontWeight(650).FontSize(13).TextColor(color).Grow(1).MinWidth(0)
				ui.Text(row.Context, fmt.Sprintf("%.3fs", packet.Elapsed)).
					FontFeatures("tnum").FontSize(11).TextColor(secondary).Shrink(0)
			})
			meta := packet.SOP
			if packet.PowerRole != "" {
				meta += " · " + packet.PowerRole
			}
			if tag := pdPacketTag(msgs, packet); tag != "" {
				meta += " · " + tag
			}
			if packet.Error != "" {
				meta += " · !"
			}
			ui.Text(row.Context, meta).SingleLine().FontSize(11).TextColor(secondary)
		})
		if !splitDetail {
			ui.Icon(row.Context, kit.IconChevronR).FontSize(14).TextColor(p.Muted).Shrink(0)
		}
	})
}

func (d *dashboard) pdPacketList(p kit.Palette, msgs locales.Messages, visible []pd.Packet, el ui.Element, touch, landscape, splitDetail bool) {
	view := &d.packets
	if splitDetail {
		el = kit.GlassPanel(el.Clip(), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft)
	} else {
		el = el.Gap(6)
	}
	el.ItemKey(func(index int) any { return visible[index].Sequence }).
		Rows(func(row ui.ListRow) {
			d.pdPacketRow(row, p, msgs, visible[row.Index], touch, landscape, splitDetail)
		}).OnChange(func() {
		if splitDetail && view.row >= 0 && view.row < len(visible) {
			view.selected = visible[view.row].Sequence
		}
	}).OnSubmit(func() {
		if view.row >= 0 && view.row < len(visible) && !splitDetail {
			view.detail, view.expanded = visible[view.row], true
		}
	})
}

func (d *dashboard) pdSplitDetailPane(ctx *ui.Context, p kit.Palette, msgs locales.Messages, all, visible []pd.Packet, summary pdLiveSummary, capsText string, landscape bool, bottomPadding float32) {
	view := &d.packets
	if view.row < 0 || view.row >= len(visible) {
		return
	}
	detailBasis, detailMin, detailPad, detailGap := float32(360), float32(280), float32(16), float32(10)
	if landscape {
		detailBasis, detailMin, detailPad, detailGap = 300, 240, 12, 8
	}
	packet := visible[view.row]
	kit.GlassPanel(ui.Column(ctx).Grow(7).Basis(detailBasis).MinWidth(detailMin).FillHeight().MinHeight(0).
		Padding(detailPad).Gap(detailGap), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft).Children(func() {
		ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			kit.SectionTitle(ctx, msgs.PacketDetails())
			ui.Spacer(ctx)
			ui.Text(ctx, fmt.Sprintf("#%d · %.3f s · %s", packet.Sequence, packet.Elapsed, packet.SOP)).
				FontFeatures("tnum").Font("monospace").FontSize(12).TextColor(p.Muted)
		})
		kit.Divider(ctx, p)
		scroll := ui.Scroll(ctx.Key(packet.Sequence)).Scrollbars(false).Grow(1).MinHeight(0).FillWidth()
		if bottomPadding > 0 {
			scroll.Padding(0, 0, bottomPadding, 0)
		}
		scroll.Children(func() {
			if landscape && (summary.caps != nil || summary.contract != nil) {
				ui.Box(ctx).FillWidth().Padding(0, 0, 8, 0).Children(func() {
					d.pdSummaryPill(ctx, p, msgs, all, visible, summary, capsText, true, 26)
				})
			}
			d.packetDetail(ctx, p, msgs, packet, func(seq uint64) {
				d.selectPDPacket(msgs, visible, all, seq, true)
			}, landscape)
		})
	})
}

func (d *dashboard) pdPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, bottomPadding float32) {
	view := &d.packets
	data := d.packetSnapshot()
	visible := view.visiblePackets(msgs, data.Packets)
	width, _ := ctx.Size()
	touch := d.touchUI()
	landscape := d.isLandscape(ctx)
	splitDetail := (!d.mobile && width >= 760) || (landscape && width >= 680)
	compact := !landscape && kit.NarrowWidth(ctx)
	summary, capsText := view.summaryState(data.Packets)

	d.pdToolbar(ctx, p, msgs, snap, data, visible, summary, capsText, touch, compact, landscape, splitDetail)

	if len(visible) == 0 {
		d.pdEmptyState(ctx, p, msgs, snap, data, touch, landscape, splitDetail, bottomPadding)
		return
	}

	view.row = len(visible) - 1
	if splitDetail && view.selected != 0 {
		if idx := findPacketBySequence(visible, view.selected); idx >= 0 {
			view.row = idx
		}
	}
	if splitDetail {
		view.list.Selected = &view.row
		view.list.Choose(view.row)
	} else {
		view.list.Selected = nil
	}
	view.list.Label = func(i int) string {
		return fmt.Sprintf("#%d %s", visible[i].Sequence, visible[i].Type)
	}
	view.list.FollowEnd = view.offline == nil && view.held == nil && view.selected == 0 && !view.expanded

	if splitDetail {
		listBasis, listMin, listMax := float32(300), float32(260), float32(420)
		if landscape {
			listBasis, listMin, listMax = 240, 200, 340
		}
		ui.Row(ctx).FillWidth().Grow(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
			list := ui.List(ctx.Key("packets"), &view.list, len(visible)).Grow(5).Basis(listBasis).MinWidth(listMin).MaxWidth(listMax).FillHeight().MinHeight(0)
			if bottomPadding > 0 {
				list.Padding(0, 0, bottomPadding, 0)
			}
			d.pdPacketList(p, msgs, visible, list, touch, landscape, splitDetail)
			d.pdSplitDetailPane(ctx, p, msgs, data.Packets, visible, summary, capsText, landscape, bottomPadding)
		})
	} else {
		list := ui.List(ctx.Key("packets"), &view.list, len(visible)).FillWidth().Grow(1).MinHeight(140).
			Padding(0, 0, bottomPadding, 0)
		d.pdPacketList(p, msgs, visible, list, touch, landscape, splitDetail)
	}
}

func pdFilterChip(ctx *ui.Context, p kit.Palette, label string, hide *bool, height float32, onChange func()) {
	active := *hide
	radius := kit.RadiusS
	if p.LiquidGlass {
		radius = kit.RadiusFull
	}
	btn := ui.ButtonBase(ctx).Height(height).Padding(0, 10).Gap(5).
		Radius(radius).AlignItems(ui.Center).Shrink(0).Label(label).
		OnClick(func() {
			*hide = !*hide
			if onChange != nil {
				onChange()
			}
		})
	bg, fg, border := p.Surface, p.Muted, p.Line
	var chipTint ui.Color
	switch {
	case active:
		bg, fg, border = p.AccentSoft, p.AccentText, p.AccentLine
		chipTint = kit.GlassAccentTint(p, 0.26, 0.30)
	case btn.Pressed():
		bg, fg = p.SurfaceHi.Mix(p.Text, .05), p.Text
	case btn.Hovered() || btn.FocusVisible():
		bg, fg = p.SurfaceHi, p.Text
	}
	kit.ControlMotion(btn).Children(func() {
		kit.GlassPillControl(ctx.Key("bg"), btn, p, radius, chipTint, true, bg, 1, border, 0.96)
		if active {
			ui.Icon(ctx, kit.IconCheck).FontSize(13).TextColor(fg).Shrink(0)
		} else {
			ui.Icon(ctx, kit.IconFilter).FontSize(13).TextColor(fg).Shrink(0)
		}
		kit.AnimatedLabel(ctx, label).FontSize(12).FontWeight(600).TextColor(fg).SingleLine()
	})
}

func pdIsNegotiation(packet pd.Packet) bool {
	switch packet.Type {
	case "Source_Capabilities", "Sink_Capabilities", "EPR_Source_Capabilities", "Request", "EPR_Request", "Accept", "Reject", "Wait", "PS_RDY":
		return true
	}
	return false
}

func pdPacketTag(msgs locales.Messages, packet pd.Packet) string {
	if packet.Error != "" {
		return ""
	}
	switch packet.Type {
	case "Source_Capabilities", "Sink_Capabilities", "EPR_Source_Capabilities", "EPR_Sink_Capabilities":
		if len(packet.PDOs) > 0 {
			tag := fmt.Sprintf("%d PDO", len(packet.PDOs))
			if caps := packet.AdvertisedSource(); caps != nil && caps.FixedMaxW > 0 {
				tag += fmt.Sprintf(" · %.4g W", caps.FixedMaxW)
			}
			return tag
		}
	case "Request", "EPR_Request":
		if packet.Request != nil {
			return packet.Request.Summary()
		}
	case "Accept", "PS_RDY":
		if packet.Analysis != nil && packet.Analysis.Request != nil {
			return packet.Analysis.Request.Summary()
		}
		if packet.Analysis != nil && packet.Analysis.RequestSequence != 0 {
			return fmt.Sprintf("#%d", packet.Analysis.RequestSequence)
		}
	case "GoodCRC":
		if packet.Analysis != nil && packet.Analysis.AcknowledgesSequence != 0 {
			return fmt.Sprintf("#%d %s", packet.Analysis.AcknowledgesSequence, packet.Analysis.AcknowledgesType)
		}
	case "Vendor_Defined", "Vendor_Defined_Extended":
		if packet.VDM != nil {
			return fmt.Sprintf("SVID 0x%04X · Cmd %d", packet.VDM.VendorID, packet.VDM.Command)
		}
	case "Revision":
		if packet.RevisionInfo != nil {
			return fmt.Sprintf("PD %s (%s)", packet.RevisionInfo.Revision, packet.RevisionInfo.Version)
		}
	case "Extended_Control":
		if packet.KeepAlive() {
			if packet.Extension.Control == "EPR_Keep_Alive_Ack" {
				return msgs.GuiEprKeepaliveAck()
			}
			return msgs.GuiEprKeepalive()
		}
	case "EPR_Mode", "PPS_Status", "Source_Capabilities_Extended", "Sink_Capabilities_Extended":
		if packet.EPRMode != nil || packet.PPSStatus != nil || packet.Capabilities != nil {
			return pdview.ExplainPacket(msgs, packet).Summary
		}
	}
	return ""
}

func (d *dashboard) packetDialog(ctx *ui.Context, p kit.Palette, msgs locales.Messages) {
	touch := d.touchUI()
	if d.mobile {
		kit.BottomSheet(ctx.Key("packet-sheet"), p, &d.packets.expanded, 0.86, msgs.GuiClose(), func() {
			d.packetModalContent(ctx, p, msgs, touch)
		})
		return
	}
	kit.Dialog(ctx.Key("packet-dialog"), p, &d.packets.expanded, 720, 640, func() {
		d.packetModalContent(ctx, p, msgs, touch)
	})
}

func (d *dashboard) packetModalContent(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	data := d.packetSnapshot()
	all := data.Packets
	visible := d.packets.visiblePackets(msgs, all)
	btnH := min(kit.ControlHeight(touch), 32)
	idx := findPacketBySequence(visible, d.packets.detail.Sequence)
	selectPacket := func(seq uint64) {
		d.selectPDPacket(msgs, visible, all, seq, false)
	}
	ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(ctx, msgs.PacketDetails()).FontSize(16).FontWeight(700).SingleLine().Shrink(0)
		ui.Spacer(ctx).MinWidth(0)
		ui.Text(ctx, fmt.Sprintf("#%d · %.3f s · %s", d.packets.detail.Sequence, d.packets.detail.Elapsed, d.packets.detail.SOP)).
			FontFeatures("tnum").Font("monospace").FontSize(12).TextColor(p.Muted).SingleLine().MinWidth(0)
		if len(visible) > 1 {
			kit.IconActionButton(ctx.Key("prev-pkt"), p, kit.IconChevronL, "‹", btnH, false, func() {
				if idx > 0 {
					d.packets.detail = visible[idx-1]
				}
			}).Disabled(idx <= 0)
			kit.IconActionButton(ctx.Key("next-pkt"), p, kit.IconChevronR, "›", btnH, false, func() {
				if idx >= 0 && idx+1 < len(visible) {
					d.packets.detail = visible[idx+1]
				}
			}).Disabled(idx < 0 || idx+1 >= len(visible))
		}
		kit.IconActionButton(ctx.Key("close"), p, kit.IconClose, msgs.GuiClose(), btnH, false, func() {
			d.packets.expanded = false
		})
	})
	kit.Divider(ctx, p)
	ui.Scroll(ctx.Key("dialog-body")).Scrollbars(false).Grow(1).MinHeight(0).FillWidth().Children(func() {
		d.packetDetail(ctx, p, msgs, d.packets.detail, selectPacket, d.isLandscape(ctx))
	})
}

func (d *dashboard) packetDetail(ctx *ui.Context, p kit.Palette, msgs locales.Messages, packet pd.Packet, onSelect func(uint64), landscape bool) {
	explanation := pdview.ExplainPacket(msgs, packet)
	negotiation := pdIsNegotiation(packet)

	ui.Column(ctx).FillWidth().Gap(12).Children(func() {
		ui.Row(ctx).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(ctx, packet.Type).FontSize(15).FontWeight(750)
			if tag := pdPacketTag(msgs, packet); tag != "" {
				kit.Badge(ctx, p, tag, kit.ToneAccent)
			}
		})

		if packet.Error != "" {
			ui.Row(ctx).FillWidth().Padding(8, 10).Radius(kit.RadiusS).Background(p.WarningSoft).Children(func() {
				ui.Text(ctx, packet.Error).FillWidth().FontSize(12).TextColor(p.Warning).Wrap()
			})
		}
		for _, warning := range explanation.Warnings {
			ui.Row(ctx).FillWidth().Padding(8, 10).Radius(kit.RadiusS).Background(p.WarningSoft).Children(func() {
				ui.Text(ctx, strings.TrimRight(warning, ".。")).FillWidth().FontSize(12).TextColor(p.Warning).Wrap()
			})
		}

		if negotiation && packet.Analysis != nil {
			analysis := packet.Analysis
			ui.Row(ctx).FillWidth().Wrap().Gap(6).Children(func() {
				for _, step := range []struct {
					label    string
					sequence uint64
				}{
					{msgs.PdCapabilitiesStep(), analysis.CapabilitiesSequence},
					{msgs.PdRequestStep(), analysis.RequestSequence},
					{msgs.PdAcceptStep(), analysis.AcceptSequence},
					{msgs.PdReadyStep(), analysis.ReadySequence},
				} {
					switch {
					case step.sequence == 0:
						kit.Badge(ctx, p, "○ "+step.label, kit.TonePlain)
					case onSelect != nil && step.sequence != packet.Sequence:
						seq := step.sequence
						stepLabel := fmt.Sprintf("✓ %s #%d", step.label, seq)
						bg, fg := kit.BadgeColors(p, kit.ToneSuccess)
						radius := kit.RadiusS
						if p.LiquidGlass {
							radius = kit.RadiusFull
						}
						btn := ui.ButtonBase(ctx.Key(fmt.Sprintf("step-%d-%s", seq, step.label))).Height(22).Shrink(0).Padding(0, 8).
							AlignItems(ui.Center).Justify(ui.Center).Radius(radius).Background(bg).Label(stepLabel).
							OnClick(func() { onSelect(seq) })
						kit.ControlMotion(btn).Children(func() {
							ui.Text(ctx, stepLabel).FontSize(11).FontWeight(650).TextColor(fg).SingleLine().Shrink(0)
						})
					default:
						kit.Badge(ctx, p, fmt.Sprintf("✓ %s #%d", step.label, step.sequence), kit.ToneSuccess)
					}
				}
			})
		}

		if len(packet.PDOs) > 0 {
			activePos := 0
			if packet.Analysis != nil && packet.Analysis.Request != nil {
				activePos = packet.Analysis.Request.Position
			}
			ui.Column(ctx).FillWidth().Padding(6, 12).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
				if summary := pdCapabilitiesSummary(packet.AdvertisedSource()); summary != "" {
					ui.Row(ctx).FillWidth().Padding(6, 0).Children(func() {
						ui.Text(ctx, summary).FontSize(11).FontWeight(650).TextColor(p.AccentText)
					})
					kit.Divider(ctx, p)
				}
				for index, offer := range packet.PDOs {
					if index > 0 {
						kit.Divider(ctx, p)
					}
					ui.Row(ctx).FillWidth().Padding(7, 0).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(ctx, fmt.Sprintf("#%d", offer.Position)).FontFeatures("tnum").Font("monospace").FontSize(11).TextColor(p.Muted).Width(24)
						ui.Text(ctx, offer.Summary()).Grow(1).MinWidth(0).FontSize(12).FontFeatures("tnum").Wrap()
						if activePos > 0 && offer.Position == activePos {
							kit.Badge(ctx, p, msgs.PdRequestStep(), kit.ToneAccent)
						}
					})
				}
			})
		}

		var rows []pdview.Field
		if negotiation && packet.Analysis != nil {
			analysis := packet.Analysis
			req := packet.Request
			if req == nil {
				req = analysis.Request
			}
			if req != nil && packet.Type != "Request" && packet.Type != "EPR_Request" {
				rows = append(rows, pdview.Field{Label: msgs.PdRequestStep(), Value: fmt.Sprintf("#%d · %s", analysis.RequestSequence, req.Summary())})
			}
			if analysis.ResponseSeconds != nil {
				rows = append(rows, pdview.Field{Label: msgs.GuiNegotiationResponse(), Value: fmt.Sprintf("%.1f ms", *analysis.ResponseSeconds*1000)})
			}
			if analysis.ReadySeconds != nil {
				rows = append(rows, pdview.Field{Label: msgs.GuiNegotiationReady(), Value: fmt.Sprintf("%.1f ms", *analysis.ReadySeconds*1000)})
			}
		}
		if vdm := packet.VDM; vdm != nil {
			rows = append(rows, pdview.Field{Label: "VDM", Value: fmt.Sprintf("SVID 0x%04X · Cmd %d · Type %d · v%d", vdm.VendorID, vdm.Command, vdm.CommandType, vdm.Version)})
		}
		rows = append(rows, explanation.Fields...)
		if len(packet.Objects) > 0 && len(packet.PDOs) == 0 && packet.Request == nil && !packet.Extended {
			objs := make([]string, len(packet.Objects))
			for i, o := range packet.Objects {
				objs[i] = fmt.Sprintf("0x%08X", o)
			}
			rows = append(rows, pdview.Field{Label: msgs.DataObjects(), Value: strings.Join(objs, " ")})
		}
		if role := strings.Trim(packet.PowerRole+" / "+packet.DataRole, " /"); role != "" {
			rows = append(rows, pdview.Field{Label: msgs.GuiRole(), Value: role})
		}
		rows = append(rows, pdview.Field{Label: msgs.GuiProtocol(), Value: fmt.Sprintf("PD %s · ID %d · 0x%04X", packet.Revision, packet.MessageID, packet.Header)})

		ui.Column(ctx).FillWidth().Padding(2, 12).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
			for i, r := range rows {
				if i > 0 {
					kit.Divider(ctx, p)
				}
				kit.KVRow(ctx, p, r.Label, r.Value, d.mobile && !landscape, nil)
			}
		})

		d.rawBytes(ctx, p, msgs.RawBytes(), packet.CRC, packet.MessageHex, msgs)
		if ext := packet.Extension; ext != nil && ext.ReassembledHex != "" {
			d.rawBytes(ctx, p, msgs.PdChunkReassembled(strconv.FormatUint(uint64(ext.DataSize), 10)), "", ext.ReassembledHex, msgs)
		}
		if packet.ReportHex != "" && packet.ReportHex != packet.MessageHex {
			d.rawBytes(ctx, p, msgs.RawReport(), "", packet.ReportHex, msgs)
		}
	})
}

func (d *dashboard) rawBytes(ctx *ui.Context, p kit.Palette, label, crcState, data string, msgs locales.Messages) {
	ui.Column(ctx).FillWidth().Padding(10, 12).Gap(6).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
		ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(ctx, label).FontSize(12).FontWeight(650)
			switch crcState {
			case "valid":
				kit.Badge(ctx, p, msgs.Crc()+" "+msgs.CrcValid(), kit.ToneSuccess)
			case "invalid":
				kit.Badge(ctx, p, msgs.Crc()+" "+msgs.CrcInvalid(), kit.ToneDanger)
			}
			ui.Spacer(ctx)
			kit.ActionButton(ctx.Key("copy-"+label), p, kit.VariantGhost, kit.IconCopy, msgs.GuiCopy(), false, func() {
				ctx.Services().WriteClipboard(data)
				d.notify(msgs.GuiCopied(), "success")
			}).Height(26).Padding(0, 8)
		})
		var lines strings.Builder
		for offset := 0; offset+1 < len(data); offset += 2 {
			if offset > 0 && offset%32 == 0 {
				lines.WriteByte('\n')
			} else if offset > 0 {
				lines.WriteByte(' ')
			}
			lines.WriteString(data[offset : offset+2])
		}
		ui.Text(ctx, lines.String()).FillWidth().Font("monospace").FontSize(11).TextColor(p.Muted).Selectable()
	})
}
