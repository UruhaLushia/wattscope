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
	selectAt := func(idx int, pkt pd.Packet) {
		if !splitDetail {
			view.detail, view.expanded = pkt, true
			return
		}
		view.selected, view.row = seq, idx
		view.list.Selected = &view.row
		view.list.Choose(idx)
		view.list.ScrollIntoView(idx)
	}
	if idx := findPacketBySequence(visible, seq); idx >= 0 {
		selectAt(idx, visible[idx])
		return
	}
	idx := findPacketBySequence(all, seq)
	if idx < 0 {
		return
	}
	pkt := all[idx]
	if !splitDetail {
		selectAt(-1, pkt)
		return
	}
	filterChanged := false
	view.search = ""
	if pkt.GoodCRC() && view.hideGoodCRC {
		view.hideGoodCRC, filterChanged = false, true
	}
	if pkt.KeepAlive() && view.hideKeepAlive {
		view.hideKeepAlive, filterChanged = false, true
	}
	if filterChanged {
		d.savePDFilters()
	}
	view.selected = seq
	visible = view.visiblePackets(msgs, all)
	if newIdx := findPacketBySequence(visible, seq); newIdx >= 0 {
		selectAt(newIdx, visible[newIdx])
	}
}

type pdFrame struct {
	data          backend.PacketSnapshot
	visible       []pd.Packet
	summary       pdLiveSummary
	capsText      string
	splitDetail   bool
	canReturnLive bool
}

func (d *dashboard) preparePDFrame(msgs locales.Messages, landscape bool, width float32) pdFrame {
	view := &d.packets
	data := d.packetSnapshot()
	visible := view.visiblePackets(msgs, data.Packets)
	summary, capsText := view.summaryState(data.Packets)
	return pdFrame{
		data:          data,
		visible:       visible,
		summary:       summary,
		capsText:      capsText,
		splitDetail:   pdUsesSplitDetail(d.mobile, landscape, width),
		canReturnLive: view.offline == nil && len(visible) > 0 && (view.held != nil || view.selected != 0 || !view.list.AtEnd()),
	}
}

func (d *dashboard) pdMoreMenuItems(msgs locales.Messages, snap backend.Snapshot, f pdFrame, compact, landscape bool) []kit.MenuItem {
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
	if len(f.visible) > 0 {
		items = append(items, kit.MenuItem{
			Label:    msgs.ExportPackets(),
			Icon:     kit.IconDownload,
			Disabled: d.busy,
			OnSelect: func() { d.exportPackets(msgs, f.visible) },
		})
	}
	if len(f.data.Packets) > 0 {
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

func (d *dashboard) pdSummaryPill(ctx *ui.Context, p kit.Palette, msgs locales.Messages, f pdFrame, pillH float32) {
	reqText := ""
	if f.summary.contract != nil {
		reqText = f.summary.contract.Summary()
	}
	if f.capsText == "" && reqText == "" {
		return
	}
	label := f.capsText
	if reqText != "" {
		if label != "" {
			label += " · " + reqText
		} else {
			label = reqText
		}
	}
	targetSeq := f.summary.contractSeq
	if targetSeq == 0 {
		targetSeq = f.summary.capsSeq
	}
	tone := kit.ToneAccent
	switch f.summary.contractStage {
	case "ready", "accepted":
		tone = kit.ToneSuccess
	case "rejected", "wait":
		tone = kit.ToneWarning
	}
	bg, fg := kit.BadgeColors(p, tone)
	btn := ui.ButtonBase(ctx.Key("pd-summary")).Height(pillH).MaxWidthPercent(100).Shrink(1).MinWidth(0).
		Padding(0, 10).Gap(6).Radius(kit.PillRadius(p, kit.RadiusS)).AlignItems(ui.Center).
		Background(bg).Label(label).OnClick(func() {
		d.selectPDPacket(msgs, f.visible, f.data.Packets, targetSeq, f.splitDetail)
	})
	kit.ControlMotion(btn).Children(func() {
		ui.Icon(ctx, kit.IconZap).FontSize(12).TextColor(fg).Shrink(0)
		ui.Text(ctx, label).FontSize(11.5).FontWeight(650).FontFeatures("tnum").TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
	})
}

func pdHeaderShowsSummary(landscape bool, position navigationPosition) bool {
	return landscape && position == navigationSide
}

func pdUsesSplitDetail(mobile, landscape bool, width float32) bool {
	return (!mobile && width >= 760) || (landscape && width >= 680)
}

func pdCompactCountLabel(visibleCount int, total uint64) string {
	if uint64(visibleCount) != total {
		return fmt.Sprintf("%d/%d", visibleCount, total)
	}
	return strconv.Itoa(visibleCount)
}

func (d *dashboard) pdToolbar(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, f pdFrame, touch, compact, landscape bool, position navigationPosition) {
	view := &d.packets
	renderCount := func(btnH float32) {
		fullCount := msgs.PacketsCount(len(f.visible))
		if uint64(len(f.visible)) != f.data.Total {
			fullCount = msgs.PacketCount(strconv.Itoa(len(f.visible)), strconv.FormatUint(f.data.Total, 10))
		}
		if landscape {
			countRadius := kit.PillRadius(p, kit.RadiusS)
			ui.Row(ctx).Shrink(0).AlignItems(ui.Center).Gap(5).Children(func() {
				ui.Box(ctx).Height(btnH).Shrink(0).Padding(0, 8).Radius(countRadius).
					Background(p.Inset).Border(1, p.LineSoft).AlignItems(ui.Center).Justify(ui.Center).
					Tooltip(fullCount).Children(func() {
					ui.Text(ctx, pdCompactCountLabel(len(f.visible), f.data.Total)).
						FontFeatures("tnum").FontSize(12).FontWeight(700).SingleLine().Shrink(0)
				})
				if view.offline != nil {
					kit.Badge(ctx, p, msgs.Offline(), kit.ToneAccent)
				}
			})
			return
		}
		ui.Row(ctx).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(ctx, fullCount).FontFeatures("tnum").FontSize(13).FontWeight(700).SingleLine()
			switch {
			case view.offline != nil:
				kit.Badge(ctx, p, msgs.Offline(), kit.ToneAccent)
			case view.held != nil:
				kit.Badge(ctx, p, msgs.Hold(), kit.ToneWarning)
			}
			if f.canReturnLive {
				kit.ActionButton(ctx.Key("follow-live"), p, kit.VariantPrimary, kit.IconArrowDown, msgs.PdLive(), false, d.pdFollowLive).
					Height(max(24, min(28, btnH))).Padding(0, 8)
			}
		})
	}

	renderActions := func(btnH float32) {
		applyActionStyle := func(el ui.Element) {
			el.Height(btnH)
			switch {
			case landscape:
				el.Padding(0, 8).Gap(4)
			case compact:
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
			recording := f.data.Recording
			if d.service != nil && view.held != nil {
				recording = d.service.PacketSnapshot(0, 0).Recording
			}
			applyActionStyle(d.captureButton(ctx, p, msgs, recording, touch))
			applyActionStyle(holdResumeButton(ctx, p, msgs, "hold-pd", view.held != nil, touch, func() {
				if view.held != nil {
					d.pdFollowLive()
				} else {
					held := f.data
					view.held = &held
				}
			}).Disabled(len(f.data.Packets) == 0 && view.held == nil))
		}
		menuOpen := d.menuOpen("pd-more")
		if items := d.pdMoreMenuItems(msgs, snap, f, compact, landscape); len(items) > 0 {
			if landscape {
				kit.IconMenuButton(ctx.Key("more"), p, menuOpen, msgs.GuiMore(), kit.IconMore, btnH, kit.VariantSecondary, touch, items)
			} else {
				moreBtn := kit.MenuButton(ctx.Key("more"), p, menuOpen, msgs.GuiMore(), kit.IconMore, kit.VariantSecondary, touch, items).
					Height(btnH)
				if compact {
					moreBtn.Padding(0, 8)
				}
			}
		} else {
			*menuOpen = false
		}
	}

	renderSearch := func(searchH float32, flexLandscape bool) {
		wrap := ui.Row(ctx).Gap(6).AlignItems(ui.Center).Grow(1)
		if flexLandscape {
			wrap.Gap(4).MinWidth(72).Basis(0)
		} else {
			wrap.MinWidth(100)
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
		const barH = float32(28)
		ui.Row(ctx).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			renderCount(barH)
			renderSearch(barH, true)
			d.pdFilterChips(ctx, p, msgs, barH)
			ui.Row(ctx).Shrink(0).Gap(5).AlignItems(ui.Center).Children(func() {
				renderActions(barH)
			})
			if !f.splitDetail && !pdHeaderShowsSummary(landscape, position) {
				d.pdSummaryPill(ctx, p, msgs, f, 26)
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
			d.pdSummaryPill(ctx, p, msgs, f, 26)
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
			renderSearch(searchH, false)
			if !compact {
				d.pdFilterChips(ctx, p, msgs, searchH)
			}
		})
		if compact {
			ui.Row(ctx).FillWidth().Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
				d.pdFilterChips(ctx, p, msgs, 28)
				d.pdSummaryPill(ctx, p, msgs, f, 28)
			})
		}
	}
}

func (d *dashboard) pdEmptyState(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, f pdFrame, touch, landscape bool, bottomPadding float32) {
	view := &d.packets
	title, hint := msgs.PdWaiting(), ""
	hasFilters := len(f.data.Packets) > 0
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
	box := kit.GlassPanel(ui.Column(ctx).FillWidth().MinHeight(140), p, kit.RadiusL, p.Surface, p.LineSoft).Center()
	if f.splitDetail || landscape {
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

func pdNegotiationDotColor(p kit.Palette, packet pd.Packet) ui.Color {
	switch packet.Type {
	case "Accept", "PS_RDY":
		return p.Green
	case "Reject", "Wait":
		return p.Warning
	default:
		return p.Accent
	}
}

type pdRowStyle struct {
	bg, color, secondary ui.Color
	padY, padX, radius   float32
	colGap               float32
	seqSize, typeSize    float32
	metaSize             float32
}

func resolvePDRowStyle(row ui.ListRow, p kit.Palette, packet pd.Packet, touch, splitDetail, compactSplit, selected bool) pdRowStyle {
	s := pdRowStyle{
		color:     p.Text,
		secondary: p.Muted,
		bg:        ui.Transparent,
		padY:      7,
		padX:      12,
		radius:    kit.RadiusM,
		colGap:    2,
		seqSize:   11,
		typeSize:  13,
		metaSize:  11,
	}
	hoverBg, pressBg := p.Hover, p.Press
	if !splitDetail {
		s.bg = p.Surface
		hoverBg, pressBg = p.SurfaceHi, p.SurfaceHi
	}
	switch {
	case selected:
		s.bg = p.Surface.Mix(p.Accent, .11)
		s.color, s.secondary = p.Text, p.AccentText
	case row.Pressed():
		s.bg = pressBg
	case row.Hovered():
		s.bg = hoverBg
	}
	switch {
	case !selected && packet.Error != "":
		s.color = p.Warning
	case !selected && (packet.GoodCRC() || packet.KeepAlive()):
		if compactSplit {
			muted := p.Muted.Alpha(0.82)
			s.color, s.secondary = muted, muted
		} else {
			s.color = p.Muted
		}
	}
	switch {
	case compactSplit:
		s.padY, s.padX, s.radius, s.colGap = 4, 8, kit.RadiusS, 1
		s.seqSize, s.typeSize, s.metaSize = 10.5, 12, 10.5
	case touch:
		s.padY, s.padX = 9, 12
	}
	return s
}

func pdPacketTitleRow(ctx *ui.Context, p kit.Palette, packet pd.Packet, s pdRowStyle, selected, compactSplit bool) {
	ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(ctx, fmt.Sprintf("#%d", packet.Sequence)).
			FontFeatures("tnum").Font("monospace").FontSize(s.seqSize).TextColor(s.secondary).Shrink(0)
		if compactSplit && !selected && pdIsNegotiation(packet) {
			ui.Box(ctx).Size(5, 5).Shrink(0).Radius(kit.RadiusFull).Background(pdNegotiationDotColor(p, packet))
		}
		ui.Text(ctx, packet.Type).
			SingleLine().FontWeight(650).FontSize(s.typeSize).TextColor(s.color).Grow(1).MinWidth(0)
		ui.Text(ctx, fmt.Sprintf("%.3fs", packet.Elapsed)).
			FontFeatures("tnum").FontSize(s.seqSize).TextColor(s.secondary).Shrink(0)
	})
}

func (d *dashboard) pdPacketRow(row ui.ListRow, p kit.Palette, msgs locales.Messages, packet pd.Packet, touch, landscape, splitDetail bool) {
	view := &d.packets
	compactSplit := landscape && splitDetail
	selected := (splitDetail && row.Selected()) || (!splitDetail && view.expanded && packet.Sequence == view.detail.Sequence)
	s := resolvePDRowStyle(row, p, packet, touch, splitDetail, compactSplit, selected)
	rowEl := ui.Row(row.Context).FillWidth().Padding(s.padY, s.padX).Gap(8).AlignItems(ui.Center).
		Radius(s.radius).Background(s.bg)
	if compactSplit {
		rowEl.Margin(0, 0, 1, 0)
	}
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
			barInset := float32(4)
			if compactSplit {
				barInset = 3
			}
			ui.Box(row.Context).Absolute().Left(0).Top(barInset).Bottom(barInset).Width(3).Radius(kit.RadiusFull).Background(p.Accent).PassThrough()
		} else if compactSplit && !row.Hovered() && !row.Pressed() {
			ui.Box(row.Context).Absolute().Left(8).Right(8).Bottom(0).Height(1).Background(p.LineSoft.Alpha(0.45)).PassThrough()
		}
		ui.Column(row.Context).Grow(1).MinWidth(0).Gap(s.colGap).Children(func() {
			pdPacketTitleRow(row.Context, p, packet, s, selected, compactSplit)
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
			ui.Text(row.Context, meta).SingleLine().FontSize(s.metaSize).TextColor(s.secondary)
		})
		if !splitDetail {
			ui.Icon(row.Context, kit.IconChevronR).FontSize(14).TextColor(p.Muted).Shrink(0)
		}
	})
}

func (d *dashboard) pdPacketList(p kit.Palette, msgs locales.Messages, visible []pd.Packet, el ui.Element, touch, landscape, splitDetail bool) {
	view := &d.packets
	if splitDetail {
		el = kit.GlassPanel(el.Clip(), p, kit.RadiusL, p.Surface, p.LineSoft)
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

func pdPacketHeaderMeta(packet pd.Packet) string {
	return fmt.Sprintf("#%d · %.3f s · %s", packet.Sequence, packet.Elapsed, packet.SOP)
}

func (d *dashboard) pdSplitDetailPane(ctx *ui.Context, p kit.Palette, msgs locales.Messages, f pdFrame, landscape bool, position navigationPosition, bottomPadding float32) {
	view := &d.packets
	if view.row < 0 || view.row >= len(f.visible) {
		return
	}
	detailBasis, detailMin, detailGap := float32(360), float32(280), float32(10)
	if landscape {
		detailBasis, detailMin, detailGap = 300, 240, 6
	}
	packet := f.visible[view.row]
	col := ui.Column(ctx).Grow(7).Basis(detailBasis).MinWidth(detailMin).FillHeight().MinHeight(0).Gap(detailGap)
	if landscape {
		col.Padding(10, 12, 8, 12)
	} else {
		col.Padding(16)
	}
	kit.GlassPanel(col, p, kit.RadiusL, p.Surface, p.LineSoft).Children(func() {
		ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if landscape {
				ui.Row(ctx).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(ctx, packet.Type).FontSize(14).FontWeight(750).SingleLine().Shrink(1).MinWidth(0)
					if tag := pdPacketTag(msgs, packet); tag != "" {
						kit.Badge(ctx, p, tag, kit.ToneAccent)
					}
				})
				ui.Text(ctx, pdPacketHeaderMeta(packet)).
					FontFeatures("tnum").Font("monospace").FontSize(11).TextColor(p.Muted).Shrink(0)
			} else {
				kit.SectionTitle(ctx, msgs.PacketDetails())
				ui.Spacer(ctx)
				ui.Text(ctx, pdPacketHeaderMeta(packet)).
					FontFeatures("tnum").Font("monospace").FontSize(12).TextColor(p.Muted)
			}
		})
		kit.Divider(ctx, p)
		scroll := ui.Scroll(ctx.Key(packet.Sequence)).Scrollbars(false).Grow(1).MinHeight(0).FillWidth()
		if bottomPadding > 0 {
			scroll.Padding(0, 0, bottomPadding, 0)
		}
		scroll.Children(func() {
			if landscape && !pdHeaderShowsSummary(landscape, position) && (f.summary.caps != nil || f.summary.contract != nil) {
				ui.Box(ctx).FillWidth().Padding(0, 0, 6, 0).Children(func() {
					d.pdSummaryPill(ctx, p, msgs, f, 22)
				})
			}
			d.packetDetail(ctx, p, msgs, packet, func(seq uint64) {
				d.selectPDPacket(msgs, f.visible, f.data.Packets, seq, true)
			}, landscape, landscape)
		})
	})
}

func (d *dashboard) pdPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, f pdFrame, position navigationPosition, bottomPadding float32) {
	view := &d.packets
	touch := d.touchUI()
	landscape := d.isLandscape(ctx)
	compact := !landscape && kit.NarrowWidth(ctx)

	d.pdToolbar(ctx, p, msgs, snap, f, touch, compact, landscape, position)

	if len(f.visible) == 0 {
		d.pdEmptyState(ctx, p, msgs, snap, f, touch, landscape, bottomPadding)
		return
	}

	view.row = len(f.visible) - 1
	if f.splitDetail && view.selected != 0 {
		if idx := findPacketBySequence(f.visible, view.selected); idx >= 0 {
			view.row = idx
		}
	}
	if f.splitDetail {
		view.list.Selected = &view.row
		view.list.Choose(view.row)
	} else {
		view.list.Selected = nil
	}
	view.list.Label = func(i int) string {
		return fmt.Sprintf("#%d %s", f.visible[i].Sequence, f.visible[i].Type)
	}
	view.list.FollowEnd = view.offline == nil && view.held == nil && view.selected == 0 && !view.expanded

	renderFollowFAB := func() {
		if !landscape || !f.canReturnLive {
			return
		}
		ui.Box(ctx.Key("pd-follow-fab")).Absolute().Right(8).Bottom(8).ZIndex(2).Children(func() {
			btn := kit.ActionButton(ctx.Key("follow-live"), p, kit.VariantPrimary, kit.IconArrowDown, msgs.PdLive(), false, d.pdFollowLive).
				Height(26).Padding(0, 9)
			if p.Shadow.A > 0 {
				btn.Shadow(0, 2, 8, 0, p.Shadow)
			}
		})
	}
	renderListBox := func(box ui.Element) {
		box.Children(func() {
			list := ui.List(ctx.Key("packets"), &view.list, len(f.visible)).Fill()
			switch {
			case f.splitDetail && landscape:
				list.Padding(4, 4, max(4, bottomPadding), 4)
			case bottomPadding > 0 || !f.splitDetail:
				list.Padding(0, 0, bottomPadding, 0)
			}
			d.pdPacketList(p, msgs, f.visible, list, touch, landscape, f.splitDetail)
			renderFollowFAB()
		})
	}

	if f.splitDetail {
		listBasis, listMin, listMax, rowGap := float32(300), float32(260), float32(420), float32(12)
		if landscape {
			listBasis, listMin, listMax, rowGap = 260, 220, 360, 8
		}
		ui.Row(ctx).FillWidth().Grow(1).MinHeight(0).Gap(rowGap).AlignItems(ui.Stretch).Children(func() {
			renderListBox(ui.Box(ctx).Grow(5).Basis(listBasis).MinWidth(listMin).MaxWidth(listMax).FillHeight().MinHeight(0))
			d.pdSplitDetailPane(ctx, p, msgs, f, landscape, position, bottomPadding)
		})
	} else {
		renderListBox(ui.Box(ctx).FillWidth().Grow(1).MinHeight(140))
	}
}

func pdFilterChip(ctx *ui.Context, p kit.Palette, label string, hide *bool, height float32, onChange func()) {
	active := *hide
	radius := kit.PillRadius(p, kit.RadiusS)
	padX, gap, iconSize, textSize := float32(10), float32(5), float32(13), float32(12)
	if height <= 28 {
		padX, gap, iconSize, textSize = 8, 4, 12, 11.5
	}
	btn := ui.ButtonBase(ctx).Height(height).Padding(0, padX).Gap(gap).
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
			ui.Icon(ctx, kit.IconCheck).FontSize(iconSize).TextColor(fg).Shrink(0)
		} else {
			ui.Icon(ctx, kit.IconFilter).FontSize(iconSize).TextColor(fg).Shrink(0)
		}
		kit.AnimatedLabel(ctx, label).FontSize(textSize).FontWeight(600).TextColor(fg).SingleLine()
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
		ui.Text(ctx, pdPacketHeaderMeta(d.packets.detail)).
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
		d.packetDetail(ctx, p, msgs, d.packets.detail, selectPacket, d.isLandscape(ctx), false)
	})
}

func pdCompactKVRow(ctx *ui.Context, p kit.Palette, label, value string) {
	if value == "" {
		return
	}
	ui.Row(ctx).FillWidth().MinHeight(26).Padding(4, 0).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(ctx, label).FontSize(11.5).FontWeight(600).TextColor(p.Muted).MinWidth(64).Shrink(0)
		ui.Text(ctx, value).FontSize(12).FontFeatures("tnum").Grow(1).MinWidth(0).Wrap().Selectable()
	})
}

type pdPairedRow[T any] struct {
	left   T
	right  T
	paired bool
}

func pdPairItems[T any](items []T, include, isShort func(T) bool) []pdPairedRow[T] {
	out := make([]pdPairedRow[T], 0, len(items))
	var pending *T
	flushPending := func() {
		if pending != nil {
			out = append(out, pdPairedRow[T]{left: *pending})
			pending = nil
		}
	}
	for i := range items {
		cur := items[i]
		if !include(cur) {
			continue
		}
		if !isShort(cur) {
			flushPending()
			out = append(out, pdPairedRow[T]{left: cur})
			continue
		}
		if pending == nil {
			pending = &items[i]
			continue
		}
		out = append(out, pdPairedRow[T]{left: *pending, right: cur, paired: true})
		pending = nil
	}
	flushPending()
	return out
}

func isShortPDField(f pdview.Field) bool {
	return len([]rune(f.Label)) <= 8 && len([]rune(f.Value)) <= 26 && !strings.Contains(f.Value, "\n")
}

func pdPairFields(rows []pdview.Field) []pdPairedRow[pdview.Field] {
	return pdPairItems(rows, func(f pdview.Field) bool { return f.Value != "" }, isShortPDField)
}

func pdLandscapeKVCell(ctx *ui.Context, p kit.Palette, f pdview.Field) {
	ui.Row(ctx).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(ctx, f.Label).FontSize(11).FontWeight(600).TextColor(p.Muted).Shrink(0)
		ui.Text(ctx, f.Value).FontSize(11.5).FontWeight(550).FontFeatures("tnum").Grow(1).MinWidth(0).Wrap().Selectable()
	})
}

func pdPairedGrid[T any](ctx *ui.Context, p kit.Palette, rows []pdPairedRow[T], minH, rowPadV, pairGap float32, renderSolo, renderCell func(T)) {
	for i, g := range rows {
		if i > 0 {
			kit.Divider(ctx, p)
		}
		if !g.paired {
			renderSolo(g.left)
			continue
		}
		row := ui.Row(ctx).FillWidth().Padding(rowPadV, 0).Gap(pairGap).AlignItems(ui.Center)
		if minH > 0 {
			row.MinHeight(minH)
		}
		row.Children(func() {
			renderCell(g.left)
			kit.VerticalDivider(ctx, p, 16)
			renderCell(g.right)
		})
	}
}

func pdLandscapeKVGrid(ctx *ui.Context, p kit.Palette, rows []pdview.Field) {
	pdPairedGrid(ctx, p, pdPairFields(rows), 26, 4, 10,
		func(f pdview.Field) { pdCompactKVRow(ctx, p, f.Label, f.Value) },
		func(f pdview.Field) { pdLandscapeKVCell(ctx, p, f) },
	)
}

func isShortPDO(offer pd.PDO) bool {
	if offer.Kind == "SPR AVS" {
		return false
	}
	return len([]rune(offer.Summary())) <= 26
}

func pdPairPDOs(pdos []pd.PDO) []pdPairedRow[pd.PDO] {
	return pdPairItems(pdos, func(pd.PDO) bool { return true }, isShortPDO)
}

func pdLandscapePDOCell(ctx *ui.Context, p kit.Palette, offer pd.PDO, activePos int, fullRow bool) {
	active := activePos > 0 && offer.Position == activePos
	row := ui.Row(ctx).AlignItems(ui.Center).Gap(6).Radius(kit.RadiusS).Padding(3, 6)
	if fullRow {
		row.FillWidth()
	} else {
		row.Grow(1).Basis(0).MinWidth(0)
	}
	posColor := p.Muted
	if active {
		row.Background(p.AccentSoft)
		posColor = p.AccentText
	}
	row.Children(func() {
		ui.Text(ctx, fmt.Sprintf("#%d", offer.Position)).FontFeatures("tnum").Font("monospace").FontSize(10.5).TextColor(posColor).Width(20).Shrink(0)
		desc := ui.Text(ctx, offer.Summary()).Grow(1).MinWidth(0).FontSize(11.5).FontFeatures("tnum").Wrap()
		if active {
			desc.FontWeight(600).TextColor(p.AccentText)
			ui.Icon(ctx, kit.IconCheck).FontSize(12).TextColor(p.AccentText).Shrink(0)
		}
	})
}

func pdLandscapePDOGrid(ctx *ui.Context, p kit.Palette, pdos []pd.PDO, activePos int) {
	pdPairedGrid(ctx, p, pdPairPDOs(pdos), 0, 2, 6,
		func(offer pd.PDO) {
			ui.Box(ctx).FillWidth().Padding(2, 0).Children(func() {
				pdLandscapePDOCell(ctx, p, offer, activePos, true)
			})
		},
		func(offer pd.PDO) { pdLandscapePDOCell(ctx, p, offer, activePos, false) },
	)
}

type pdDetailMetrics struct {
	outerGap, warnPadY, warnPadX, warnSize                       float32
	stepGap, stepBtnH, stepPadX                                  float32
	pdoPadY, pdoPadX, pdoRowPadY, pdoRowGap, pdoPosSize, pdoPosW float32
	pdoDescSize, kvPadX                                          float32
}

func resolvePDDetailMetrics(landscape bool) pdDetailMetrics {
	if landscape {
		return pdDetailMetrics{
			outerGap: 7, warnPadY: 5, warnPadX: 8, warnSize: 11.5,
			stepGap: 4, stepBtnH: 20, stepPadX: 7,
			pdoPadY: 4, pdoPadX: 10, pdoRowPadY: 4, pdoRowGap: 8, pdoPosSize: 10.5, pdoPosW: 22, pdoDescSize: 11.5, kvPadX: 10,
		}
	}
	return pdDetailMetrics{
		outerGap: 12, warnPadY: 8, warnPadX: 10, warnSize: 12,
		stepGap: 6, stepBtnH: 22, stepPadX: 8,
		pdoPadY: 6, pdoPadX: 12, pdoRowPadY: 7, pdoRowGap: 10, pdoPosSize: 11, pdoPosW: 24, pdoDescSize: 12, kvPadX: 12,
	}
}

func (d *dashboard) packetDetail(ctx *ui.Context, p kit.Palette, msgs locales.Messages, packet pd.Packet, onSelect func(uint64), landscape, omitHeader bool) {
	explanation := pdview.ExplainPacket(msgs, packet)
	negotiation := pdIsNegotiation(packet)
	m := resolvePDDetailMetrics(landscape)

	ui.Column(ctx).FillWidth().Gap(m.outerGap).Children(func() {
		if !omitHeader {
			ui.Row(ctx).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(ctx, packet.Type).FontSize(15).FontWeight(750)
				if tag := pdPacketTag(msgs, packet); tag != "" {
					kit.Badge(ctx, p, tag, kit.ToneAccent)
				}
			})
		}

		renderWarning := func(msg string) {
			ui.Row(ctx).FillWidth().Padding(m.warnPadY, m.warnPadX).Radius(kit.RadiusS).Background(p.WarningSoft).Children(func() {
				ui.Text(ctx, msg).FillWidth().FontSize(m.warnSize).TextColor(p.Warning).Wrap()
			})
		}
		if packet.Error != "" {
			renderWarning(packet.Error)
		}
		for _, warning := range explanation.Warnings {
			renderWarning(strings.TrimRight(warning, ".。"))
		}

		if negotiation && packet.Analysis != nil {
			analysis := packet.Analysis
			ui.Row(ctx).FillWidth().Wrap().Gap(m.stepGap).Children(func() {
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
						kit.Badge(ctx, p, "○ "+step.label, kit.TonePlain).Height(m.stepBtnH)
					case onSelect != nil && step.sequence != packet.Sequence:
						seq := step.sequence
						kit.BadgeButton(ctx.Key(fmt.Sprintf("step-%d-%s", seq, step.label)), p, fmt.Sprintf("✓ %s #%d", step.label, seq), kit.ToneSuccess, m.stepBtnH, func() {
							onSelect(seq)
						}).Padding(0, m.stepPadX)
					default:
						kit.Badge(ctx, p, fmt.Sprintf("✓ %s #%d", step.label, step.sequence), kit.ToneSuccess).Height(m.stepBtnH)
					}
				}
			})
		}

		if len(packet.PDOs) > 0 {
			activePos := 0
			if packet.Analysis != nil && packet.Analysis.Request != nil {
				activePos = packet.Analysis.Request.Position
			}
			ui.Column(ctx).FillWidth().Padding(m.pdoPadY, m.pdoPadX).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
				if summary := pdCapabilitiesSummary(packet.AdvertisedSource()); summary != "" {
					ui.Row(ctx).FillWidth().Padding(m.pdoRowPadY, 0).Children(func() {
						ui.Text(ctx, summary).FontSize(11).FontWeight(650).TextColor(p.AccentText)
					})
					kit.Divider(ctx, p)
				}
				if landscape && len(packet.PDOs) >= 2 {
					pdLandscapePDOGrid(ctx, p, packet.PDOs, activePos)
				} else {
					for index, offer := range packet.PDOs {
						if index > 0 {
							kit.Divider(ctx, p)
						}
						ui.Row(ctx).FillWidth().Padding(m.pdoRowPadY, 0).Gap(m.pdoRowGap).AlignItems(ui.Center).Children(func() {
							ui.Text(ctx, fmt.Sprintf("#%d", offer.Position)).FontFeatures("tnum").Font("monospace").FontSize(m.pdoPosSize).TextColor(p.Muted).Width(m.pdoPosW)
							ui.Text(ctx, offer.Summary()).Grow(1).MinWidth(0).FontSize(m.pdoDescSize).FontFeatures("tnum").Wrap()
							if activePos > 0 && offer.Position == activePos {
								kit.Badge(ctx, p, msgs.PdRequestStep(), kit.ToneAccent)
							}
						})
					}
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
			skipRedundantReq := landscape && omitHeader && (packet.Type == "Accept" || packet.Type == "PS_RDY")
			if req != nil && packet.Type != "Request" && packet.Type != "EPR_Request" && !skipRedundantReq {
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

		ui.Column(ctx).FillWidth().Padding(2, m.kvPadX).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
			if landscape {
				pdLandscapeKVGrid(ctx, p, rows)
				return
			}
			var kvBuf [16]kit.KVItem
			items := kvBuf[:0]
			if len(rows) > len(kvBuf) {
				items = make([]kit.KVItem, 0, len(rows))
			}
			for _, r := range rows {
				items = append(items, kit.KVItem{Label: r.Label, Value: r.Value})
			}
			kit.KVList(ctx, p, items, d.mobile)
		})

		d.rawBytes(ctx, p, msgs.RawBytes(), packet.CRC, packet.MessageHex, msgs, landscape)
		if ext := packet.Extension; ext != nil && ext.ReassembledHex != "" {
			d.rawBytes(ctx, p, msgs.PdChunkReassembled(strconv.FormatUint(uint64(ext.DataSize), 10)), "", ext.ReassembledHex, msgs, landscape)
		}
		if packet.ReportHex != "" && packet.ReportHex != packet.MessageHex {
			d.rawBytes(ctx, p, msgs.RawReport(), "", packet.ReportHex, msgs, landscape)
		}
	})
}

func (d *dashboard) rawBytes(ctx *ui.Context, p kit.Palette, label, crcState, data string, msgs locales.Messages, landscape bool) {
	padY, padX, gap, titleSize, btnH, btnPadX := float32(10), float32(12), float32(6), float32(12), float32(26), float32(8)
	if landscape {
		padY, padX, gap, titleSize, btnH, btnPadX = 6, 10, 4, 11.5, 22, 7
	}
	ui.Column(ctx).FillWidth().Padding(padY, padX).Gap(gap).Radius(kit.RadiusM).Background(p.Inset).Border(1, p.LineSoft).Children(func() {
		ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(ctx, label).FontSize(titleSize).FontWeight(650)
			switch crcState {
			case "valid":
				kit.Badge(ctx, p, msgs.Crc()+" "+msgs.CrcValid(), kit.ToneSuccess)
			case "invalid":
				kit.Badge(ctx, p, msgs.Crc()+" "+msgs.CrcInvalid(), kit.ToneDanger)
			}
			ui.Spacer(ctx)
			kit.ActionButton(ctx.Key("copy-"+label), p, kit.VariantGhost, kit.IconCopy, msgs.GuiCopy(), false, func() {
				d.copyText(ctx, msgs, data)
			}).Height(btnH).Padding(0, btnPadX)
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
