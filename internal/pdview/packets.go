package pdview

import (
	"fmt"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/locales"
)

func FilterPackets(m locales.Messages, packets []pd.Packet, search string, hideGoodCRC, hideKeepAlive bool) []pd.Packet {
	query := strings.ToLower(strings.TrimSpace(search))
	if query == "" && !hideGoodCRC && !hideKeepAlive {
		return packets
	}
	var visible []pd.Packet
	for _, packet := range packets {
		if hideGoodCRC && packet.GoodCRC() || hideKeepAlive && packet.KeepAlive() {
			continue
		}
		if query != "" {
			text := packet.Type + " " + packet.SOP + " " + packet.PowerRole + " " + packet.DataRole + " " + packet.MessageHex
			if packet.Request != nil {
				text += " " + packet.Request.Summary()
			}
			for _, pdo := range packet.PDOs {
				text += " " + pdo.Summary()
			}
			explanation := ExplainPacket(m, packet)
			text += " " + explanation.Summary + " " + strings.Join(explanation.Warnings, " ")
			if !strings.Contains(strings.ToLower(text), query) {
				continue
			}
		}
		visible = append(visible, packet)
	}
	return visible
}

type PacketExplanation struct {
	Summary  string
	Details  []string
	Warnings []string
}

// ExplainPacket turns decoded fields and analysis findings into localized text.
func ExplainPacket(m locales.Messages, p pd.Packet) PacketExplanation {
	x := p.Analysis
	if x == nil {
		x = &pd.Analysis{}
	}
	e := PacketExplanation{}
	if p.Error != "" {
		e.Summary = m.PdMeaningInvalid()
		return e
	}
	switch p.Type {
	case "Source_Capabilities":
		e.Summary = m.PdMeaningSourceCaps(len(p.PDOs))
	case "Sink_Capabilities":
		e.Summary = m.PdMeaningSinkCaps(len(p.PDOs))
	case "Request", "EPR_Request":
		if p.Request != nil {
			e.Summary = m.PdMeaningRequest(p.Request.Summary())
		}
	case "Accept":
		e.Summary = m.PdMeaningAcceptUnknown()
		if x.RequestSequence != 0 {
			e.Summary = m.PdMeaningAccept(fmt.Sprint(x.RequestSequence))
		}
	case "PS_RDY":
		e.Summary = m.PdMeaningReadyUnknown()
		if x.RequestSequence != 0 {
			e.Summary = m.PdMeaningReady(fmt.Sprint(x.RequestSequence))
		}
	case "GoodCRC":
		e.Summary = m.PdMeaningAckUnknown()
		if x.AcknowledgesSequence != 0 {
			e.Summary = m.PdMeaningAck(fmt.Sprint(x.AcknowledgesSequence), x.AcknowledgesType)
		}
	case "Reject":
		e.Summary = m.PdMeaningReject()
	case "Wait":
		e.Summary = m.PdMeaningWait()
	case "Soft_Reset":
		e.Summary = m.PdMeaningSoftReset()
	case "PR_Swap":
		e.Summary = m.PdMeaningPowerSwap()
	case "DR_Swap":
		e.Summary = m.PdMeaningDataSwap()
	case "VCONN_Swap":
		e.Summary = m.PdMeaningVconnSwap()
	case "Data_Reset":
		e.Summary = m.PdMeaningDataReset()
	case "Get_Source_Cap", "Get_Source_Cap_Extended":
		e.Summary = m.PdMeaningSourceQuery()
	case "Get_Sink_Cap", "Get_Sink_Cap_Extended":
		e.Summary = m.PdMeaningSinkQuery()
	case "Vendor_Defined", "Vendor_Defined_Extended":
		if p.VDM != nil {
			e.Summary = m.PdMeaningVdm(fmt.Sprintf("0x%04X", p.VDM.VendorID))
		}
	case "Revision":
		if p.RevisionInfo != nil {
			e.Summary = m.PdReportedRevision(p.RevisionInfo.Revision, p.RevisionInfo.Version)
		}
	case "Extended_Control":
		if p.KeepAlive() {
			e.Summary = m.PdMeaningEprKeepalive()
			if p.Extension.DataHex == "0400" {
				e.Summary = m.PdMeaningEprKeepaliveAck()
			}
		}
	}
	if e.Summary == "" {
		e.Summary = m.PdMeaningUnknown()
		if p.Extended {
			e.Summary = m.PdMeaningExtended()
		}
	}
	r := x.Request
	if r == nil {
		r = p.Request
	}
	if r != nil {
		if p.Type != "Request" && p.Type != "EPR_Request" {
			e.Details = append(e.Details, m.PdRelatedRequest(fmt.Sprint(x.RequestSequence), r.Summary()))
		}
		if r.MaximumA > 0 {
			e.Details = append(e.Details, m.PdCurrentLimit(fmt.Sprintf("%g", r.MaximumA)))
		}
		if r.MaximumW > 0 {
			e.Details = append(e.Details, m.PdPowerLimit(fmt.Sprintf("%g", r.MaximumW)))
		}
		if r.GiveBack {
			value := fmt.Sprintf("%g A", r.MinimumA)
			if r.Kind == "Battery" {
				value = fmt.Sprintf("%g W", r.MinimumW)
			}
			e.Details = append(e.Details, m.PdGiveBack(value))
		}
	}
	if x.ResponseSeconds != nil {
		e.Details = append(e.Details, m.PdResponseTime(fmt.Sprintf("%.1f", *x.ResponseSeconds*1000)))
	}
	if x.ReadySeconds != nil {
		e.Details = append(e.Details, m.PdReadyTime(fmt.Sprintf("%.1f", *x.ReadySeconds*1000)))
	}
	if x.IntervalSeconds != nil {
		detail := m.PdRequestInterval(fmt.Sprintf("%.3f", *x.IntervalSeconds))
		if x.SameRequest {
			detail += " · " + m.PdSameRdo()
		}
		e.Details = append(e.Details, detail)
	}
	if x.ResponseSeconds != nil || x.ReadySeconds != nil {
		e.Details = append(e.Details, m.PdHostTiming())
	}
	for _, code := range x.Findings {
		var warning string
		switch code {
		case "source_capabilities_missing":
			warning = m.RequestContextMissing()
		case "pdo_position_out_of_range":
			warning = m.PdWarningPosition()
		case "voltage_out_of_range":
			warning = m.PdWarningVoltage()
		case "current_exceeds_source":
			warning = m.PdWarningCurrent()
		case "power_exceeds_source":
			warning = m.PdWarningPower()
		case "inconsistent_request_limits":
			warning = m.PdWarningLimits()
		case "capability_mismatch":
			warning = m.PdWarningMismatch()
		case "incomplete_previous_request":
			warning = m.PdWarningPartial()
		case "accept_not_observed":
			warning = m.PdWarningMissingAccept()
		case "capture_gap":
			warning = m.PdWarningGap()
		case "retransmission":
			warning = m.PdWarningRetry()
		case "epr_context_unavailable":
			warning = m.PdWarningEprContext()
		case "unsupported_supply":
			warning = m.PdWarningSupply()
		case "unexpected_role":
			warning = m.PdWarningRole()
		}
		if warning != "" {
			e.Warnings = append(e.Warnings, warning)
		}
	}
	if p.Analysis == nil && p.Request != nil && !p.Request.Matched {
		e.Warnings = append(e.Warnings, m.RequestContextMissing())
	}
	return e
}
