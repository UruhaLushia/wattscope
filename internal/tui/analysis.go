package tui

import (
	"fmt"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/pdview"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/charmbracelet/x/ansi"
)

func packetCapabilities(p pd.Packet, width int, v View) string {
	m := v.messages()
	var caps *pd.SourceCapabilities
	if p.Analysis != nil {
		caps = p.Analysis.SourceCapabilities
	}
	if caps == nil {
		caps = p.AdvertisedSource()
	}
	if caps == nil {
		if p.Error == "" && p.Revision != "" && p.Revision != "reserved" {
			return m.PdObservedProtocol(p.SOP, p.Revision) + " · " + m.PdSourceCapsWaiting()
		}
		return m.PdSourceCapsWaiting()
	}
	features := []string{"USB PD"}
	if caps.HeaderRevision != "reserved" {
		features[0] += " " + caps.HeaderRevision
	}
	if caps.PPS {
		features = append(features, "PPS")
	}
	if caps.SPRAVS {
		features = append(features, "SPR AVS")
	}
	if caps.EPR {
		features = append(features, "EPR")
	}
	power := ""
	if caps.FixedMaxW > 0 {
		power = " · " + m.PdFixedMaxPower(fmt.Sprintf("%.4g", caps.FixedMaxW))
	}
	full := m.PdSourceCapsLabel() + " · " + strings.Join(features, " · ") + power
	if ansi.StringWidth(full) <= width {
		return full
	}
	compact := strings.Join(features, " · ")
	if ansi.StringWidth(compact+power) <= width {
		return compact + power
	}
	return ansi.Truncate(compact, width, "…")
}

func packetIntro(p pd.Packet, w int, v View) []string {
	explanation := pdview.ExplainPacket(v.messages(), p)
	rows := []string{packetIdentity(p, w), fit(ansi.Truncate(explanation.Summary, max(1, w), "…"), w)}
	if p.Request == nil && p.Analysis != nil && p.Analysis.Request != nil {
		rows = append(rows, fit(p.Analysis.Request.Summary(), w))
	}
	return rows
}

func negotiationPanel(p pd.Packet, w int, v View) []string {
	m := v.messages()
	x := p.Analysis
	if x == nil {
		x = &pd.Analysis{}
	}
	step := func(name string, seq uint64, observed bool) string {
		if !observed {
			return quietStyle.Render("○ " + name)
		}
		label := "✓ " + name
		if seq != 0 {
			label += fmt.Sprintf(" #%d", seq)
		}
		return color(label, positive)
	}
	// Keep long sequence numbers in the border; node widths stay predictable.
	timeline := step(m.PdCapabilitiesStep(), 0, x.CapabilitiesSequence != 0 || p.Type == "Source_Capabilities" && p.Error == "")
	timeline += quietStyle.Render(" ── ") + step(m.PdRequestStep(), 0, x.RequestSequence != 0)
	response := step(m.PdAcceptStep(), 0, x.AcceptSequence != 0)
	if x.Stage == "rejected" {
		response = color("! "+m.PdRejectStep(), warning)
	}
	if x.Stage == "wait" {
		response = color("Ⅱ "+m.PdWaitStep(), warning)
	}
	timeline += quietStyle.Render(" ── ") + response + quietStyle.Render(" ── ") + step(m.PdReadyStep(), 0, x.ReadySequence != 0)
	e := pdview.ExplainPacket(m, p)
	description := e.Summary
	if x.Request != nil {
		description = m.PdRelatedRequest(fmt.Sprint(x.RequestSequence), x.Request.Summary())
	}
	annotation := e.Summary
	if len(p.PDOs) > 0 {
		var profiles []string
		for _, pdo := range p.PDOs {
			profiles = append(profiles, pdo.Summary())
		}
		annotation = strings.Join(profiles, " · ")
	} else if x.Stage == "requested" {
		annotation = m.PdAwaitingAccept()
	}
	if x.ResponseSeconds != nil {
		annotation = m.PdResponseTime(fmt.Sprintf("%.1f", *x.ResponseSeconds*1000))
	}
	if x.ReadySeconds != nil {
		annotation += " · " + m.PdReadyTime(fmt.Sprintf("%.1f", *x.ReadySeconds*1000))
	}
	if x.IntervalSeconds != nil && x.ResponseSeconds == nil {
		annotation = m.PdRequestInterval(fmt.Sprintf("%.3f", *x.IntervalSeconds))
		if x.SameRequest {
			annotation += " · " + m.PdSameRdo()
		}
	}
	tone := border
	annotation = quietStyle.Render(annotation)
	if len(e.Warnings) > 0 {
		annotation = color(ansi.Truncate(e.Warnings[0], max(1, w-4), "…"), warning)
		tone = warning
	}
	badge := m.PdAtSelected()
	if x.RequestSequence != 0 {
		badge = fmt.Sprintf("#%d → #%d · %s", x.RequestSequence, p.Sequence, m.PdAtSelected())
	}
	return panel(strong(m.PdNegotiation(), primary), badge, w, 5, []string{timeline, fit(ansi.Truncate(description, max(1, w-4), "…"), w-4), annotation}, tone)
}

func pdoTable(pdos []pd.PDO, w int, v View) []string {
	if len(pdos) == 0 {
		return nil
	}
	m := v.messages()
	if w < 55 {
		var rows []string
		for _, pdo := range pdos {
			rows = append(rows, fmt.Sprintf("PDO %d  %s", pdo.Position, pdo.Summary()))
		}
		return rows
	}
	columns := func(position, kind, voltage, current, power, raw string) string {
		line := fit(position, 3) + " " + fit(kind, 10) + " " + fit(voltage, 13) + " " + fit(current, 8) + " " + fit(power, 8)
		if w >= 70 {
			line += " " + raw
		}
		return fit(line, w)
	}
	rows := []string{quietStyle.Render(columns("PDO", m.PdSupplyType(), m.Voltage(), m.PdSupplyCurrent(), m.PdSupplyPower(), m.RawBytes()))}
	for _, pdo := range pdos {
		voltage := fmt.Sprintf("%g V", pdo.MaxV)
		if pdo.MinV != pdo.MaxV {
			voltage = fmt.Sprintf("%g–%g V", pdo.MinV, pdo.MaxV)
		}
		current := "—"
		if pdo.MaxA > 0 {
			current = fmt.Sprintf("%g A", pdo.MaxA)
		}
		power := "—"
		if pdo.MaxW > 0 {
			power = fmt.Sprintf("%g W", pdo.MaxW)
		}
		if pdo.MaxV == 0 {
			voltage = "—"
		}
		rows = append(rows, columns(fmt.Sprint(pdo.Position), pdo.Kind, voltage, current, power, fmt.Sprintf("%08X", pdo.Raw)))
	}
	return rows
}

func packetAnalysisRows(p pd.Packet, w int, v View) []string {
	m := v.messages()
	e := pdview.ExplainPacket(m, p)
	if len(e.Details) == 0 && len(e.Warnings) == 0 {
		return nil
	}
	rows := []string{strong(m.PdAnalysis(), accent)}
	rows = append(rows, e.Details...)
	for _, warningText := range e.Warnings {
		rows = append(rows, color(warningText, warning))
	}
	var wrapped []string
	for _, row := range rows {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(row, max(1, w), ""), "\n")...)
	}
	return wrapped
}
