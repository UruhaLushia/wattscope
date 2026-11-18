package tui

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/pdview"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

type packetLayout struct {
	listWidth, listHeight, detailWidth, detailHeight int
	headerHeight                                     int
	side                                             bool
}

const packetHeaderRows = 3

func packetPanes(w, h int, hasSelection, expanded bool) packetLayout {
	l := packetLayout{listWidth: w, headerHeight: packetHeaderRows}
	if hasSelection && w >= 64 && h >= 27 && !expanded {
		l.headerHeight += 5
	}
	l.listHeight = max(2, h-l.headerHeight)
	if expanded {
		return l
	}
	if w >= 110 && l.listHeight >= 12 {
		l.side = true
		l.detailWidth = max(42, w*2/5)
		l.listWidth = w - l.detailWidth - 1
		l.detailHeight = l.listHeight
	} else if l.listHeight >= 12 {
		l.detailWidth = w
		l.detailHeight = min(14, max(7, (h-packetHeaderRows)/3))
		l.listHeight -= l.detailHeight
	}
	return l
}

func filteredPackets(v View) []pd.Packet {
	return pdview.FilterPackets(v.messages(), v.Packets.Packets, v.PacketFilter, v.HideGoodCRC, v.HideKeepAlive)
}

func selectedPacketIndex(packets []pd.Packet, v View) int {
	if len(packets) == 0 {
		return -1
	}
	if v.SelectedPacket == 0 {
		return len(packets) - 1
	}
	for i, packet := range packets {
		if packet.Sequence == v.SelectedPacket {
			return i
		}
	}
	return 0
}

const packetFilterGap = " · "

func packetFilters(width int, v View) []action {
	m := v.messages()
	hint := m.PdFilterHint()
	if v.Filtering {
		return []action{{key: "/", label: strong(m.PdFilterInput(safe(v.PacketFilter)+"▏"), accent)}}
	}
	searchText := hint
	searchActive := v.PacketFilter != ""
	if searchActive {
		searchText = "/ " + m.PdFilter() + ": " + safe(v.PacketFilter)
	}
	crcText := "a ✓ GoodCRC"
	crcLabel := color("a", accent) + " " + color("✓", positive) + " " + color("GoodCRC", primary)
	if v.HideGoodCRC {
		crcText = "a ✕ GoodCRC"
		crcLabel = quietStyle.Render(crcText)
	}
	keepAliveText := "k ✓ KeepAlive"
	keepAliveLabel := color("k", accent) + " " + color("✓", positive) + " " + color("KeepAlive", primary)
	if v.HideKeepAlive {
		keepAliveText = "k ✕ KeepAlive"
		keepAliveLabel = quietStyle.Render(keepAliveText)
	}
	controlsWidth := ansi.StringWidth(crcText + packetFilterGap + keepAliveText + packetFilterGap)
	if width-controlsWidth >= ansi.StringWidth(hint) {
		searchText = ansi.Truncate(searchText, width-controlsWidth, "…")
	}
	searchLabel := quietStyle.Render(searchText)
	if searchActive {
		searchLabel = color(searchText, accent)
	}
	return []action{{key: "/", label: searchLabel}, {key: "a", label: crcLabel}, {key: "k", label: keepAliveLabel}}
}

func packetsPage(s backend.Snapshot, w, h int, v View) []string {
	m := v.messages()
	packets := filteredPackets(v)
	selected := selectedPacketIndex(packets, v)
	stateBadge := color("● "+m.PdLive(), positive)
	if v.PDPaused {
		stateBadge = strong("Ⅱ "+m.PdReview(), accent)
	}
	if v.SourcePath != "" {
		stateBadge = strong("● "+m.Offline(), accent)
	}
	rows := []string{apart(quietStyle.Render(m.PacketCount(fmt.Sprint(len(packets)), fmt.Sprint(v.Packets.Total))), stateBadge, w)}
	var filters []string
	for _, item := range packetFilters(w, v) {
		filters = append(filters, item.label)
	}
	rows = append(rows, fit(strings.Join(filters, quietStyle.Render(packetFilterGap)), w))
	capabilities := ""
	if selected >= 0 {
		capabilities = packetCapabilities(packets[selected], w, v)
	}
	rows = append(rows, color(fit(capabilities, w), accent))
	l := packetPanes(w, h, selected >= 0, v.PacketDetails && selected >= 0)
	if l.headerHeight > packetHeaderRows {
		rows = append(rows, negotiationPanel(packets[selected], w, v)...)
	}
	if v.PacketDetails && selected >= 0 {
		detail := packetDetails(packets[selected], w-4, v)
		offset := max(0, min(v.PacketScroll, max(0, len(detail)-(l.listHeight-2))))
		return append(rows, panel(strong(m.PacketDetails(), accent), m.OverlayKeys(), w, l.listHeight, detail[offset:], accent)...)
	}
	list := packetList(s, packets, selected, l.listWidth, l.listHeight, v)
	if l.detailHeight == 0 {
		return append(rows, list...)
	}
	var detail []string
	hint := ""
	if selected >= 0 {
		hint = m.ExpandPacket()
		if l.side {
			detail = packetDetails(packets[selected], l.detailWidth-4, v)
		} else {
			detail = packetPreview(packets[selected], l.detailWidth-4, v)
		}
	}
	preview := panel(strong(m.PacketDetails(), primary), hint, l.detailWidth, l.detailHeight, detail, border)
	if l.side {
		return append(rows, beside(list, preview, 1)...)
	}
	return append(append(rows, list...), preview...)
}

func packetListRange(total, selected, height int) (start, end int) {
	count := max(1, height-4)
	start = max(0, min(selected-count/2, total-count))
	return start, min(total, start+count)
}

func packetEmptyState(s backend.Snapshot, v View) []string {
	m := v.messages()
	switch {
	case len(v.Packets.Packets) > 0:
		return []string{m.PdNoMatch()}
	case v.SourcePath != "":
		return []string{m.EmptyCapture()}
	case v.PDPaused:
		return []string{m.PdResumeLive()}
	case s.State != backend.Streaming:
		status, tone := connectionStatus(s, m)
		return []string{color(status, tone)}
	case s.Device.Supports(device.CapPDDirectUSB):
		return []string{m.PdDirectUsb()}
	case !s.Device.Supports(device.CapPDCapture):
		return []string{m.PdUnsupported()}
	case s.SavedPDHost != nil && *s.SavedPDHost:
		return []string{m.PdWaiting(), m.PdSavedOnlineOn(), m.PdRetrigger()}
	case s.SavedPDHost != nil:
		return []string{m.PdWaiting(), m.PdSavedOnlineOff(), m.PdEnableOnline()}
	case s.Device.Supports(device.CapPDHostMode) && v.Packets.Total == 0:
		return []string{m.PdWaiting(), m.PdOnlineUnknown(), m.PdInstruction()}
	default:
		return []string{m.PdWaiting(), m.PdRetrigger()}
	}
}

func packetTone(p pd.Packet) lipgloss.TerminalColor {
	if p.Error != "" {
		return warning
	}
	if p.GoodCRC() || p.KeepAlive() {
		return muted
	}
	switch p.Type {
	case "Source_Capabilities", "Sink_Capabilities":
		return accent
	case "Accept", "PS_RDY":
		return positive
	case "Reject", "Wait", "Soft_Reset", "Hard_Reset":
		return warning
	default:
		return primary
	}
}

func packetList(s backend.Snapshot, packets []pd.Packet, selected, w, h int, v View) []string {
	m := v.messages()
	if len(packets) == 0 {
		var rows []string
		for _, line := range packetEmptyState(s, v) {
			rows = append(rows, strings.Split(ansi.Wrap(line, max(1, w-4), ""), "\n")...)
		}
		return panel(strong(m.PacketList(), primary), "", w, h, rows, border)
	}
	count := max(1, h-4)
	start, end := packetListRange(len(packets), selected, h)
	rowWidth := w - 4
	if len(packets) > count {
		rowWidth -= 2
	}
	seqWidth := max(4, len(fmt.Sprint(packets[len(packets)-1].Sequence)))
	columns := func(seq, elapsed, sop, role, kind, summary string) string {
		line := fit(seq, seqWidth) + " " + fit(elapsed, 9) + " "
		if w >= 65 {
			line += fit(ansi.Truncate(sop, 5, "…"), 5) + " " + fit(role, 6) + "  "
		}
		if w >= 90 {
			line += fit(ansi.Truncate(kind, 24, "…"), 24) + "  "
			return line + ansi.Truncate(summary, max(0, rowWidth-2-ansi.StringWidth(line)), "…")
		}
		return line + ansi.Truncate(kind, max(0, rowWidth-2-ansi.StringWidth(line)), "…")
	}
	heading := "  " + columns(m.PacketSequence(), fitRight(m.PacketTime()+" (s)", 9), "SOP", m.PacketRole(), m.PacketType(), m.PacketSummary())
	rows := []string{quietStyle.Render(fit(heading, rowWidth))}
	for i := start; i < end; i++ {
		p := packets[i]
		marker := "  "
		tone := packetTone(p)
		if p.Error != "" {
			marker = "! "
		}
		if i == selected {
			marker = "› "
			if p.Error != "" {
				marker = "›!"
			}
		}
		line := marker + columns(fmt.Sprintf("%*d", seqWidth, p.Sequence), fmt.Sprintf("%9.3f", p.Elapsed), safe(p.SOP), safe(p.PowerRole), safe(p.Type), packetSummary(p, v))
		line = color(fit(line, rowWidth), tone)
		if i == selected {
			style := selectionStyle
			if p.Error != "" {
				style = style.Foreground(warning)
			}
			line = style.Render(ansi.Strip(line))
		}
		if len(packets) > count {
			thumb := max(1, count*count/len(packets))
			position := start * (count - thumb) / (len(packets) - count)
			track := borderStyle.Render("│")
			if i-start >= position && i-start < position+thumb {
				track = color("┃", accent)
			}
			line += " " + track
		}
		rows = append(rows, line)
	}
	for len(rows) < h-3 {
		rows = append(rows, "")
	}
	rows = append(rows, quietStyle.Render(ansi.Truncate(m.PdSelectHint(), rowWidth, "…")))
	return panel(strong(m.PacketList(), primary), fmt.Sprintf("%d–%d / %d", start+1, end, len(packets)), w, h, rows, border)
}

func packetSummary(p pd.Packet, v View) string {
	if p.Error != "" {
		return safe(p.Error)
	}
	return pdview.ExplainPacket(v.messages(), p).Summary
}

func packetIdentity(p pd.Packet, w int) string {
	return apart(strong(safe(p.Type), accent), quietStyle.Render(fmt.Sprintf("#%d · %.3fs · %s", p.Sequence, p.Elapsed, safe(p.SOP))), w)
}

func packetBytes(p pd.Packet, w int, v View) []string {
	m := v.messages()
	crc, tone := m.CrcUnverified(), lipgloss.TerminalColor(warning)
	switch p.CRC {
	case "valid":
		crc, tone = m.CrcValid(), positive
	case "invalid":
		crc = m.CrcInvalid()
	case "absent":
		crc, tone = m.CrcAbsent(), muted
	}
	badge := m.ByteCount(len(p.MessageHex)/2) + " · " + m.Crc() + " " + crc
	rows := []string{apart(strong(m.RawBytes(), primary), color(badge, tone), w)}
	return append(rows, hexRows(p.MessageHex, w)...)
}

// The inline preview is deliberately ordered independently of the full field
// list: even a four-line interior must include the selected message's bytes.
func packetPreview(p pd.Packet, w int, v View) []string {
	rows := packetIntro(p, w, v)
	rows = append(rows, packetBytes(p, w, v)...)
	if p.Error != "" {
		rows = append(rows, color(fit(v.messages().PacketError()+": "+safe(p.Error), w), warning))
	}
	e := pdview.ExplainPacket(v.messages(), p)
	for _, notice := range e.Warnings {
		rows = append(rows, color(fit(notice, w), warning))
	}
	rows = append(rows, pdoTable(p.PDOs, w, v)...)
	details := e.Details
	if p.Request == nil && p.Analysis != nil && p.Analysis.Request != nil && len(details) > 0 {
		details = details[1:]
	}
	for _, detail := range details {
		rows = append(rows, quietStyle.Render(fit(detail, w)))
	}
	meta := fmt.Sprintf("0x%04X · PD %s · %s · ID %d", p.Header, safe(p.Revision), strings.Trim(safe(p.PowerRole)+" / "+safe(p.DataRole), " /"), p.MessageID)
	return append(rows, quietStyle.Render(fit(meta, w)))
}

func packetDetails(p pd.Packet, w int, v View) []string {
	m := v.messages()
	rows := []string{strong(safe(p.Type), accent), quietStyle.Render(fmt.Sprintf("#%d · %.3fs · %s", p.Sequence, p.Elapsed, safe(p.SOP)))}
	rows = append(rows, pdview.ExplainPacket(m, p).Summary)
	if p.Error != "" {
		rows = append(rows, color(m.PacketError()+": "+safe(p.Error), warning))
	}
	rows = append(rows, "")
	rows = append(rows, packetBytes(p, w, v)...)
	rows = append(rows, "")
	rows = append(rows, packetAnalysisRows(p, w, v)...)
	if len(p.Objects) > 0 {
		rows = append(rows, "", strong(m.DataObjects(), primary))
	}
	rows = append(rows, pdoTable(p.PDOs, w, v)...)
	if p.VDM != nil {
		rows = append(rows, fmt.Sprintf("SVID 0x%04X · Structured %t", p.VDM.VendorID, p.VDM.Structured))
		if p.VDM.Structured {
			rows = append(rows, fmt.Sprintf("VDM cmd %d / type %d / ver %d", p.VDM.Command, p.VDM.CommandType, p.VDM.Version))
		}
	}
	if p.Extension != nil {
		e := p.Extension
		rows = append(rows, strong(m.ExtendedHeader(), primary), m.ExtendedChunk(fmt.Sprint(e.DataSize), fmt.Sprint(e.ChunkNumber), fmt.Sprint(e.RequestChunk)), quietStyle.Render(m.ExtendedNote()))
	}
	if len(p.PDOs) == 0 {
		for i, object := range p.Objects {
			rows = append(rows, fmt.Sprintf("DO%d 0x%08X", i+1, object))
		}
	}
	rows = append(rows, "", strong(m.ProtocolFields(), primary))
	labelWidth := 0
	for _, label := range []string{m.Header(), m.Revision(), m.PowerRole(), m.DataRole(), m.MessageId()} {
		labelWidth = max(labelWidth, ansi.StringWidth(label))
	}
	add := func(label, value string) {
		rows = append(rows, quietStyle.Render(fit(label, labelWidth))+"  "+safe(value))
	}
	add(m.Header(), fmt.Sprintf("0x%04X", p.Header))
	add(m.Revision(), p.Revision)
	add(m.PowerRole(), p.PowerRole)
	add(m.DataRole(), p.DataRole)
	add(m.MessageId(), fmt.Sprint(p.MessageID))
	if p.Simulated {
		rows = append(rows, color(m.Simulated(), warning))
	}
	if p.ReportHex != "" {
		rows = append(rows, "", quietStyle.Render(m.RawReport()))
		rows = append(rows, hexRows(p.ReportHex, w)...)
	}
	var wrapped []string
	for _, row := range rows {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(row, max(1, w), true), "\n")...)
	}
	return wrapped
}

func hexRows(text string, w int) []string {
	raw, err := hex.DecodeString(text)
	if err != nil {
		return []string{safe(text)}
	}
	if len(raw) == 0 {
		return []string{quietStyle.Render("—")}
	}
	step := max(1, min(16, (w-6)/3))
	var rows []string
	for start := 0; start < len(raw); start += step {
		var bytes []string
		for _, b := range raw[start:min(start+step, len(raw))] {
			bytes = append(bytes, fmt.Sprintf("%02X", b))
		}
		if len(bytes) > 8 {
			bytes[8] = " " + bytes[8]
		}
		rows = append(rows, quietStyle.Render(fmt.Sprintf("%04X", start))+"  "+strings.Join(bytes, " "))
	}
	return rows
}
