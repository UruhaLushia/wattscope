package app

import (
	"fmt"
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
}

type packetFilterKey struct {
	first         *pd.Packet
	count         int
	messages      locales.Messages
	query         string
	hideGoodCRC   bool
	hideKeepAlive bool
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

func (d *dashboard) pdPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, bottomPadding float32) {
	view := &d.packets
	data := d.packetSnapshot()
	visible := view.visiblePackets(msgs, data.Packets)
	width, _ := ctx.Size()
	touch := d.touchUI(ctx)
	splitDetail := width >= 760 && !d.mobile
	compact := kit.NarrowWidth(ctx)

	follow := func() {
		view.selected = 0
		view.held = nil
		view.list.ScrollToEnd()
	}

	moreItems := func() []kit.MenuItem {
		var items []kit.MenuItem
		if view.offline == nil || compact {
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
		if view.offline == nil && view.selected != 0 {
			items = append(items, kit.MenuItem{
				Label:    msgs.PdLive(),
				Icon:     kit.IconActivity,
				OnSelect: follow,
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
					d.notify(msgs.PacketsCleared(), "", "info")
				},
			})
		}
		return items
	}

	renderCount := func() {
		count := msgs.PacketsCount(len(visible))
		if uint64(len(visible)) != data.Total {
			count = msgs.PacketCount(fmt.Sprint(len(visible)), fmt.Sprint(data.Total))
		}
		ui.Row(ctx).Grow(1).MinWidth(100).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(ctx, count).FontFeatures("tnum").FontSize(13).FontWeight(700).SingleLine()
			switch {
			case view.offline != nil:
				kit.Badge(ctx, p, msgs.Offline(), kit.ToneAccent)
			case view.held != nil:
				kit.Badge(ctx, p, msgs.Hold(), kit.ToneWarning)
			}
		})
	}

	renderActions := func() {
		if view.offline != nil {
			if !compact {
				kit.ActionButton(ctx.Key("open"), p, kit.VariantSecondary, kit.IconFolder, msgs.GuiOpenCapture(), touch, func() {
					d.openCapture(msgs)
				}).Disabled(d.busy)
			}
			kit.ActionButton(ctx.Key("return"), p, kit.VariantPrimary, kit.IconClose, msgs.GuiClose(), touch, func() {
				d.packets = packetView{}
			})
		} else {
			recording := data.Recording
			if d.service != nil && view.held != nil {
				recording = d.service.PacketSnapshot(0, 0).Recording
			}
			d.captureButton(ctx, p, msgs, recording, touch)
			label, icon, variant := msgs.ActionHold(), kit.IconPause, kit.VariantSecondary
			if view.held != nil {
				label, icon, variant = msgs.ActionResume(), kit.IconPlay, kit.VariantPrimary
			}
			kit.ActionButton(ctx.Key("hold-pd"), p, variant, icon, label, touch, func() {
				if view.held != nil {
					follow()
				} else {
					view.held = &data
				}
			}).Disabled(len(data.Packets) == 0 && view.held == nil)
		}
		menuOpen := d.menuOpen("pd-more")
		if items := moreItems(); len(items) > 0 {
			kit.MenuButton(ctx.Key("more"), p, menuOpen, msgs.GuiMore(), kit.IconMore, kit.VariantSecondary, touch, items)
		} else {
			*menuOpen = false
		}
	}

	if compact {
		ui.Column(ctx).FillWidth().Gap(8).Children(func() {
			ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				renderCount()
			})
			ui.Row(ctx).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(renderActions)
		})
	} else {
		kit.ActionBar(ctx).AlignItems(ui.Center).Gap(8).Children(func() {
			renderCount()
			renderActions()
		})
	}

	if view.path != "" {
		ui.Row(ctx).FillWidth().Padding(6, 10).Gap(8).Radius(kit.RadiusS).Background(p.Inset).Border(1, p.LineSoft).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(ctx, kit.IconFileText).FontSize(13).TextColor(p.Muted).Shrink(0)
			ui.Text(ctx, middleEllipsis(view.path, 64)).Grow(1).MinWidth(0).SingleLine().FontSize(12).
				TextColor(p.Muted).Tooltip(view.path)
		})
	}

	ui.Row(ctx).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.TextInput(ctx.Key("query"), &view.search).Grow(1).MinWidth(0).Height(kit.ControlHeight(touch)).
			Placeholder(msgs.GuiSearchPackets()).Label(msgs.GuiSearchPackets())
		if !compact {
			pdFilterChip(ctx.Key("chip-goodcrc"), p, msgs.GuiHideGoodCrc(), &view.hideGoodCRC, touch)
			pdFilterChip(ctx.Key("chip-keepalive"), p, msgs.GuiHideKeepAlive(), &view.hideKeepAlive, touch)
		} else {
			kit.MenuButton(ctx.Key("filters"), p, d.menuOpen("pd-filters"), msgs.PdFilter(), kit.IconFilter, kit.VariantSecondary, touch, []kit.MenuItem{
				{
					Label:    msgs.GuiHideGoodCrc(),
					Check:    true,
					KeepOpen: true,
					Active:   view.hideGoodCRC,
					OnSelect: func() { view.hideGoodCRC = !view.hideGoodCRC },
				},
				{
					Label:    msgs.GuiHideKeepAlive(),
					Check:    true,
					KeepOpen: true,
					Active:   view.hideKeepAlive,
					OnSelect: func() { view.hideKeepAlive = !view.hideKeepAlive },
				},
			})
		}
	})

	if len(visible) == 0 {
		title, hint := msgs.PdWaiting(), ""
		switch {
		case len(data.Packets) > 0:
			title = msgs.PdNoMatch()
		case view.offline != nil:
			title = msgs.EmptyCapture()
		case snap.Device.Supports(device.CapPDDirectUSB):
			hint = strings.TrimRight(msgs.PdDirectUsb(), ".。")
		case snap.Device.ID != "" && !snap.Device.Supports(device.CapPDCapture):
			hint = msgs.GuiUnsupported()
		}
		box := kit.GlassPanel(ui.Column(ctx).FillWidth().MinHeight(180), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft).Center()
		if splitDetail {
			box.Grow(1)
		} else {
			box.Shrink(0).Padding(16, 12)
		}
		box.Children(func() {
			kit.EmptyState(ctx, p, kit.IconUSB, title, hint, func() {
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
		return
	}

	view.row = len(visible) - 1
	if splitDetail && view.selected != 0 {
		for index, packet := range visible {
			if packet.Sequence == view.selected {
				view.row = index
				break
			}
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
	view.list.FollowEnd = view.selected == 0

	buildList := func(el ui.Element) {
		if splitDetail {
			el = kit.GlassPanel(el.Clip(), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft)
		} else {
			el = el.Gap(8)
		}
		el.ItemKey(func(index int) any { return visible[index].Sequence }).
			Rows(func(row ui.ListRow) {
				packet := visible[row.Index]
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
				if touch {
					rowPadY, rowPadX = 11, 14
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
					view.selected = packet.Sequence
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
			}).OnChange(func() {
			if splitDetail && view.row >= 0 && view.row < len(visible) {
				view.selected = visible[view.row].Sequence
			}
		}).OnSubmit(func() {
			if view.row >= 0 && view.row < len(visible) && !splitDetail {
				view.selected = visible[view.row].Sequence
				view.detail, view.expanded = visible[view.row], true
			}
		})
	}

	if splitDetail {
		ui.Row(ctx).FillWidth().Grow(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
			list := ui.List(ctx.Key("packets"), &view.list, len(visible)).Grow(5).Basis(300).MinWidth(260).MaxWidth(420).FillHeight().MinHeight(0)
			buildList(list)

			if view.row >= 0 && view.row < len(visible) {
				packet := visible[view.row]
				kit.GlassPanel(ui.Column(ctx).Grow(7).Basis(360).MinWidth(280).FillHeight().MinHeight(0).
					Padding(16).Gap(10), p, kit.RadiusL, 0.25, p.Surface, p.LineSoft).Children(func() {
					ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						kit.SectionTitle(ctx, msgs.PacketDetails())
						ui.Spacer(ctx)
						ui.Text(ctx, fmt.Sprintf("#%d · %.3f s · %s", packet.Sequence, packet.Elapsed, packet.SOP)).
							FontFeatures("tnum").Font("monospace").FontSize(12).TextColor(p.Muted)
					})
					kit.Divider(ctx, p)
					ui.Scroll(ctx.Key(packet.Sequence)).Scrollbars(false).Grow(1).MinHeight(0).FillWidth().Children(func() {
						d.packetDetail(ctx, p, msgs, packet)
					})
				})
			}
		})
	} else {
		list := ui.List(ctx.Key("packets"), &view.list, len(visible)).FillWidth().Grow(1).MinHeight(140).
			Padding(0, 0, bottomPadding, 0)
		buildList(list)
	}
}

func pdFilterChip(ctx *ui.Context, p kit.Palette, label string, hide *bool, touch bool) {
	active := *hide
	radius := kit.RadiusS
	if p.LiquidGlass {
		radius = kit.RadiusFull
	}
	btn := ui.ButtonBase(ctx).Height(kit.ControlHeight(touch)).Padding(0, 10).Gap(5).
		Radius(radius).AlignItems(ui.Center).Shrink(0).Label(label).
		OnClick(func() { *hide = !*hide })
	bg, fg, border := p.Surface, p.Muted, p.Line
	var chipTint ui.Color
	if active {
		bg, fg, border = p.AccentSoft, p.AccentText, p.AccentLine
		chipTint = kit.GlassAccentTint(p, 0.26, 0.30)
	} else if btn.Pressed() {
		bg, fg = p.SurfaceHi.Mix(p.Text, .05), p.Text
	} else if btn.Hovered() || btn.FocusVisible() {
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
			return fmt.Sprintf("%d PDO", len(packet.PDOs))
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
	touch := d.touchUI(ctx)
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
	ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(ctx, msgs.PacketDetails()).FontSize(16).FontWeight(700)
		ui.Spacer(ctx)
		ui.Text(ctx, fmt.Sprintf("#%d · %.3f s · %s", d.packets.detail.Sequence, d.packets.detail.Elapsed, d.packets.detail.SOP)).
			FontFeatures("tnum").Font("monospace").FontSize(12).TextColor(p.Muted)
		kit.IconActionButton(ctx.Key("close"), p, kit.IconClose, msgs.GuiClose(), kit.ControlHeight(touch), false, func() {
			d.packets.expanded = false
		})
	})
	kit.Divider(ctx, p)
	ui.Scroll(ctx.Key("dialog-body")).Scrollbars(false).Grow(1).MinHeight(0).FillWidth().Children(func() {
		d.packetDetail(ctx, p, msgs, d.packets.detail)
	})
}

func (d *dashboard) packetDetail(ctx *ui.Context, p kit.Palette, msgs locales.Messages, packet pd.Packet) {
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
					if step.sequence == 0 {
						kit.Badge(ctx, p, "○ "+step.label, kit.TonePlain)
					} else {
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
				if caps := packet.AdvertisedSource(); caps != nil {
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
					ui.Row(ctx).FillWidth().Padding(6, 0).Children(func() {
						ui.Text(ctx, strings.Join(features, " · ")).FontSize(11).FontWeight(650).TextColor(p.AccentText)
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
				kit.KVRow(ctx, p, r.Label, r.Value, d.mobile, nil)
			}
		})

		d.rawBytes(ctx, p, msgs.RawBytes(), packet.CRC, packet.MessageHex, msgs)
		if ext := packet.Extension; ext != nil && ext.ReassembledHex != "" {
			d.rawBytes(ctx, p, msgs.PdChunkReassembled(fmt.Sprint(ext.DataSize)), "", ext.ReassembledHex, msgs)
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
				d.notify(msgs.GuiCopied(), "", "success")
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
