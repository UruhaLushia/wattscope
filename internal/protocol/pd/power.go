package pd

import "fmt"

type SourceCapabilities struct {
	Sequence       uint64  `json:"sequence"`
	HeaderRevision string  `json:"header_revision"`
	FixedMaxW      float64 `json:"fixed_max_w,omitempty"`
	PPS            bool    `json:"pps,omitempty"`
	SPRAVS         bool    `json:"spr_avs,omitempty"`
	EPR            bool    `json:"epr,omitempty"`
}

func (p Packet) AdvertisedSource() *SourceCapabilities {
	if p.Type != "Source_Capabilities" || p.SOP != "SOP" || p.PowerRole != "Source" || p.Error != "" || len(p.PDOs) == 0 {
		return nil
	}
	caps := &SourceCapabilities{Sequence: p.Sequence, HeaderRevision: p.Revision}
	first := p.PDOs[0]
	if p.Revision == "3.x" && first.Kind == "Fixed" && first.MaxV == 5 {
		const eprCapable = 1 << 23
		caps.EPR = first.Raw&eprCapable != 0
	}
	for _, pdo := range p.PDOs {
		if pdo.Kind == "Fixed" {
			caps.FixedMaxW = max(caps.FixedMaxW, pdo.MaxW)
		}
		if p.Revision == "3.x" {
			const sprAVSAPDO = 0b1110
			caps.PPS = caps.PPS || pdo.Kind == "PPS"
			caps.SPRAVS = caps.SPRAVS || pdo.Raw>>28 == sprAVSAPDO
		}
	}
	return caps
}

type PDO struct {
	Position int     `json:"position"`
	Raw      uint32  `json:"raw"`
	Kind     string  `json:"kind"`
	MinV     float64 `json:"min_voltage_v"`
	MaxV     float64 `json:"max_voltage_v"`
	MaxA     float64 `json:"max_current_a,omitempty"`
	MaxW     float64 `json:"max_power_w,omitempty"`
}

func DecodePDO(position int, raw uint32) PDO {
	p := PDO{Position: position, Raw: raw}
	switch raw >> 30 {
	case 0:
		p.Kind = "Fixed"
		p.MinV, p.MaxV, p.MaxA = float64(raw>>10&1023)/20, float64(raw>>10&1023)/20, float64(raw&1023)/100
	case 1, 2:
		p.Kind = "Variable"
		p.MinV, p.MaxV = float64(raw>>10&1023)/20, float64(raw>>20&1023)/20
		if raw>>30 == 1 {
			p.Kind, p.MaxW = "Battery", float64(raw&1023)/4
		} else {
			p.MaxA = float64(raw&1023) / 100
		}
	case 3:
		switch raw >> 28 & 3 {
		case 0:
			p.Kind = "PPS"
			p.MinV, p.MaxV, p.MaxA = float64(raw>>8&255)/10, float64(raw>>17&255)/10, float64(raw&127)/20
		case 1:
			p.Kind = "EPR AVS"
			p.MinV, p.MaxV, p.MaxW = float64(raw>>8&255)/10, float64(raw>>17&511)/10, float64(raw&255)
		default:
			p.Kind = "APDO (raw)"
		}
	}
	if p.MaxW == 0 && p.MaxA > 0 {
		p.MaxW = p.MaxV * p.MaxA
	}
	return p
}

func (p PDO) Summary() string {
	if p.MaxV == 0 {
		return fmt.Sprintf("%s 0x%08X", p.Kind, p.Raw)
	}
	voltage := fmt.Sprintf("%g V", p.MaxV)
	if p.MinV != p.MaxV {
		voltage = fmt.Sprintf("%g–%g V", p.MinV, p.MaxV)
	}
	if p.MaxA > 0 {
		return fmt.Sprintf("%s %s / %g A", p.Kind, voltage, p.MaxA)
	}
	return fmt.Sprintf("%s %s / %g W", p.Kind, voltage, p.MaxW)
}

type Request struct {
	Raw        uint32  `json:"raw"`
	Position   int     `json:"object_position"`
	Kind       string  `json:"kind"`
	VoltageV   float64 `json:"voltage_v,omitempty"`
	OperatingA float64 `json:"operating_current_a,omitempty"`
	MaximumA   float64 `json:"maximum_current_a,omitempty"`
	MinimumA   float64 `json:"minimum_current_a,omitempty"`
	OperatingW float64 `json:"operating_power_w,omitempty"`
	MaximumW   float64 `json:"maximum_power_w,omitempty"`
	MinimumW   float64 `json:"minimum_power_w,omitempty"`
	GiveBack   bool    `json:"give_back,omitempty"`
	Mismatch   bool    `json:"capability_mismatch"`
	Matched    bool    `json:"matched_source_capability"`
}

// DecodeRequest only interprets units when a preceding Source_Capabilities
// message identifies the selected supply. The object index alone is not enough.
func DecodeRequest(raw uint32, source []PDO) Request {
	r := Request{Raw: raw, Position: int(raw >> 28 & 15), Kind: "RDO (raw)", Mismatch: raw&0x4000000 != 0}
	if r.Position < 1 || r.Position > len(source) {
		return r
	}
	p := source[r.Position-1]
	r.Kind, r.Matched = p.Kind, true
	switch p.Kind {
	case "Fixed", "Variable":
		r.OperatingA, r.MaximumA = float64(raw>>10&1023)/100, float64(raw&1023)/100
		if raw&0x8000000 != 0 {
			r.GiveBack = true
			r.MinimumA, r.MaximumA = r.MaximumA, 0
		}
		if p.Kind == "Fixed" {
			r.VoltageV = p.MaxV
		}
	case "Battery":
		r.OperatingW, r.MaximumW = float64(raw>>10&1023)/4, float64(raw&1023)/4
		if raw&0x8000000 != 0 {
			r.GiveBack = true
			r.MinimumW, r.MaximumW = r.MaximumW, 0
		}
	case "PPS":
		r.VoltageV, r.OperatingA = float64(raw>>9&4095)/50, float64(raw&127)/20
	case "EPR AVS":
		r.VoltageV, r.OperatingA = float64(raw>>9&4095)/40, float64(raw&127)/20
	default:
		r.Matched = false
	}
	return r
}

func (r Request) Summary() string {
	if !r.Matched {
		return fmt.Sprintf("PDO %d · 0x%08X", r.Position, r.Raw)
	}
	if r.OperatingW > 0 {
		return fmt.Sprintf("PDO %d · %s %g W", r.Position, r.Kind, r.OperatingW)
	}
	if r.VoltageV > 0 {
		return fmt.Sprintf("PDO %d · %s %g V / %g A", r.Position, r.Kind, r.VoltageV, r.OperatingA)
	}
	return fmt.Sprintf("PDO %d · %s %g A", r.Position, r.Kind, r.OperatingA)
}
