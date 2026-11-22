package pdview

import (
	"fmt"
	"strconv"
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
			var text strings.Builder
			text.WriteString(packet.Type)
			for _, value := range [...]string{packet.SOP, packet.PowerRole, packet.DataRole, packet.MessageHex} {
				text.WriteByte(' ')
				text.WriteString(value)
			}
			if packet.Request != nil {
				text.WriteByte(' ')
				text.WriteString(packet.Request.Summary())
			}
			for _, pdo := range packet.PDOs {
				text.WriteByte(' ')
				text.WriteString(pdo.Summary())
			}
			explanation := ExplainPacket(m, packet)
			text.WriteByte(' ')
			text.WriteString(explanation.Summary)
			for _, warning := range explanation.Warnings {
				text.WriteByte(' ')
				text.WriteString(warning)
			}
			text.WriteByte(' ')
			for _, field := range explanation.Fields {
				text.WriteByte(' ')
				text.WriteString(field.Label)
				text.WriteByte(' ')
				text.WriteString(field.Value)
			}
			if !strings.Contains(strings.ToLower(text.String()), query) {
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
	Fields   []Field
}

type Field struct{ Label, Value string }

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
	case "Source_Capabilities", "EPR_Source_Capabilities":
		if len(p.PDOs) > 0 {
			e.Summary = m.PdMeaningSourceCaps(len(p.PDOs))
		}
	case "Sink_Capabilities", "EPR_Sink_Capabilities":
		if len(p.PDOs) > 0 {
			e.Summary = m.PdMeaningSinkCaps(len(p.PDOs))
		}
	case "Source_Capabilities_Extended", "Sink_Capabilities_Extended":
		if caps := p.Capabilities; caps != nil {
			e.Summary = strconv.FormatUint(uint64(caps.SPR.MaximumW), 10) + " W"
			if caps.EPR != nil && caps.EPR.MaximumW != 0 {
				e.Summary += " · EPR PDP " + strconv.FormatUint(uint64(caps.EPR.MaximumW), 10) + " W"
			}
			e.Fields = append(e.Fields,
				Field{"VID / PID / XID", fmt.Sprintf("%04X / %04X / %08X", caps.VID, caps.PID, caps.XID)},
				Field{"FW / HW", fmt.Sprintf("%02X / %02X", caps.Firmware, caps.Hardware)},
			)
			if p.Type == "Sink_Capabilities_Extended" {
				e.Fields = append(e.Fields, Field{"SPR PDP", m.PdPdpRange(strconv.FormatUint(uint64(caps.SPR.MinimumW), 10), strconv.FormatUint(uint64(caps.SPR.OperatingW), 10), strconv.FormatUint(uint64(caps.SPR.MaximumW), 10))})
				if caps.EPR != nil && caps.EPR.MaximumW != 0 {
					e.Fields = append(e.Fields, Field{"EPR PDP", m.PdPdpRange(strconv.FormatUint(uint64(caps.EPR.MinimumW), 10), strconv.FormatUint(uint64(caps.EPR.OperatingW), 10), strconv.FormatUint(uint64(caps.EPR.MaximumW), 10))})
				}
			}
		}
	case "Request", "EPR_Request":
		if p.Request != nil {
			e.Summary = m.PdMeaningRequest(p.Request.Summary())
		}
	case "Accept":
		e.Summary = m.PdMeaningAcceptUnknown()
		if x.RequestSequence != 0 {
			e.Summary = m.PdMeaningAccept(strconv.FormatUint(x.RequestSequence, 10))
		}
	case "PS_RDY":
		e.Summary = m.PdMeaningReadyUnknown()
		if x.RequestSequence != 0 {
			e.Summary = m.PdMeaningReady(strconv.FormatUint(x.RequestSequence, 10))
		}
	case "GoodCRC":
		e.Summary = m.PdMeaningAckUnknown()
		if x.AcknowledgesSequence != 0 {
			e.Summary = m.PdMeaningAck(strconv.FormatUint(x.AcknowledgesSequence, 10), x.AcknowledgesType)
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
		if p.Extension != nil {
			switch p.Extension.Control {
			case "EPR_Get_Source_Cap":
				e.Summary = m.PdMeaningSourceQuery()
			case "EPR_Get_Sink_Cap":
				e.Summary = m.PdMeaningSinkQuery()
			case "EPR_Keep_Alive":
				e.Summary = m.PdMeaningEprKeepalive()
			case "EPR_Keep_Alive_Ack":
				e.Summary = m.PdMeaningEprKeepaliveAck()
			}
		}
	case "EPR_Mode":
		if mode := p.EPRMode; mode != nil {
			switch mode.Action {
			case 1:
				e.Summary = m.PdEprEnter()
				e.Fields = append(e.Fields, Field{m.PdEprSinkPower(), strconv.FormatUint(uint64(mode.Data), 10) + " W"})
			case 2:
				e.Summary = m.PdEprEnterAck()
			case 3:
				e.Summary = m.PdEprEntered()
			case 4:
				e.Summary = m.PdEprEnterFailed()
				e.Fields = append(e.Fields, Field{e.Summary, m.PdEprFailure(strconv.FormatUint(uint64(mode.Data), 10)) + fmt.Sprintf(" (0x%02X)", mode.Data)})
			case 5:
				e.Summary = m.PdEprExit()
			}
		}
	case "PPS_Status":
		if status := p.PPSStatus; status != nil {
			voltage, current := "—", "—"
			if status.VoltageV != nil {
				voltage = fmt.Sprintf("%g V", *status.VoltageV)
			}
			if status.CurrentA != nil {
				current = fmt.Sprintf("%g A", *status.CurrentA)
			}
			e.Summary = m.PdPpsOutput(voltage, current)
			mode := "CV"
			if status.CurrentLimit {
				mode = "CL"
			}
			e.Fields = append(e.Fields, Field{m.ModeLabel(), mode}, Field{m.Temperature(), m.PdPpsTemperature(status.Temperature)})
		}
	}
	if p.RequestedPDO != nil {
		e.Fields = append(e.Fields, Field{m.PdCopiedPdo(), fmt.Sprintf("#%d · %s · 0x%08X", p.RequestedPDO.Position, p.RequestedPDO.Summary(), p.RequestedPDO.Raw)})
	}
	if ext := p.Extension; ext != nil {
		value := m.ByteCount(int(ext.DataSize))
		switch {
		case ext.RequestChunk:
			value = m.PdChunkRequest(strconv.FormatUint(uint64(ext.ChunkNumber), 10))
			e.Summary = value
		case ext.ReassembledHex != "":
			value = m.PdChunkReassembled(strconv.FormatUint(uint64(ext.DataSize), 10))
		case ext.Chunked && len(ext.DataHex)/2 < int(ext.DataSize):
			value = m.PdChunkPartial(strconv.FormatUint(uint64(ext.ChunkNumber), 10), strconv.FormatUint(uint64(ext.DataSize), 10))
			e.Summary = value
		}
		e.Fields = append(e.Fields, Field{m.ExtendedHeader(), value})
		if ext.Control != "" {
			e.Fields = append(e.Fields, Field{p.Type, ext.Control})
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
			e.Details = append(e.Details, m.PdRelatedRequest(strconv.FormatUint(x.RequestSequence, 10), r.Summary()))
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
		case "epr_pdo_mismatch":
			warning = m.PdWarningEprPdoMismatch()
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
		if p.Type == "EPR_Request" {
			e.Warnings = append(e.Warnings, m.PdWarningEprContext())
		} else {
			e.Warnings = append(e.Warnings, m.RequestContextMissing())
		}
	}
	return e
}
