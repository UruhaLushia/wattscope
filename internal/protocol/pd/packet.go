// Package pd decodes USB Power Delivery messages without depending on a meter
// transport or a frontend. Unknown message types and payloads remain available.
package pd

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"slices"
	"time"
)

type Packet struct {
	Sequence     uint64                `json:"sequence"`
	Timestamp    time.Time             `json:"timestamp"`
	Elapsed      float64               `json:"elapsed_s"`
	Segment      uint64                `json:"segment"`
	Simulated    bool                  `json:"simulated"`
	SOP          string                `json:"sop"`
	Header       uint16                `json:"header"`
	Revision     string                `json:"revision"`
	PowerRole    string                `json:"power_role,omitempty"`
	DataRole     string                `json:"data_role,omitempty"`
	MessageID    uint8                 `json:"message_id"`
	Type         string                `json:"type"`
	Extended     bool                  `json:"extended"`
	Objects      []uint32              `json:"objects,omitempty"`
	PDOs         []PDO                 `json:"pdos,omitempty"`
	Request      *Request              `json:"request,omitempty"`
	RequestedPDO *PDO                  `json:"requested_pdo,omitempty"`
	EPRMode      *EPRMode              `json:"epr_mode,omitempty"`
	PPSStatus    *PPSStatus            `json:"pps_status,omitempty"`
	VDM          *VDM                  `json:"vdm,omitempty"`
	Extension    *Extended             `json:"extension,omitempty"`
	Capabilities *CapabilitiesExtended `json:"capabilities_extended,omitempty"`
	RevisionInfo *RevisionInfo         `json:"revision_info,omitempty"`
	Analysis     *Analysis             `json:"analysis,omitempty"`
	MessageHex   string                `json:"message_hex"`
	ReportHex    string                `json:"report_hex,omitempty"`
	CRC          string                `json:"crc"` // absent, valid, invalid, or unverified
	Error        string                `json:"error,omitempty"`
}

type RevisionInfo struct {
	Revision string `json:"revision"`
	Version  string `json:"version"`
}

type Extended struct {
	DataSize       uint16 `json:"data_size"`
	Chunked        bool   `json:"chunked"`
	ChunkNumber    uint8  `json:"chunk_number"`
	RequestChunk   bool   `json:"request_chunk"`
	DataHex        string `json:"data_hex"`
	ReassembledHex string `json:"reassembled_hex,omitempty"`
	Control        string `json:"control,omitempty"`
}

type VDM struct {
	VendorID       uint16 `json:"vendor_id"`
	Structured     bool   `json:"structured"`
	Version        uint8  `json:"version"`
	ObjectPosition uint8  `json:"object_position"`
	CommandType    uint8  `json:"command_type"`
	Command        uint8  `json:"command"`
}

var controlTypes = [...]string{
	"Reserved", "GoodCRC", "GotoMin", "Accept", "Reject", "Ping", "PS_RDY",
	"Get_Source_Cap", "Get_Sink_Cap", "DR_Swap", "PR_Swap", "VCONN_Swap", "Wait",
	"Soft_Reset", "Data_Reset", "Data_Reset_Complete", "Not_Supported",
	"Get_Source_Cap_Extended", "Get_Status", "FR_Swap", "Get_PPS_Status",
	"Get_Country_Codes", "Get_Sink_Cap_Extended", "Get_Source_Info", "Get_Revision",
}

var dataTypes = map[uint8]string{
	1: "Source_Capabilities", 2: "Request", 3: "BIST", 4: "Sink_Capabilities",
	5: "Battery_Status", 6: "Alert", 7: "Get_Country_Info", 8: "Enter_USB",
	9: "EPR_Request", 10: "EPR_Mode", 11: "Source_Info", 12: "Revision", 15: "Vendor_Defined",
}

var extendedTypes = map[uint8]string{
	1: "Source_Capabilities_Extended", 2: "Status", 3: "Get_Battery_Cap", 4: "Get_Battery_Status",
	5: "Battery_Capabilities", 6: "Get_Manufacturer_Info", 7: "Manufacturer_Info",
	8: "Security_Request", 9: "Security_Response", 10: "Firmware_Update_Request",
	11: "Firmware_Update_Response", 12: "PPS_Status", 13: "Country_Info", 14: "Country_Codes",
	15: "Sink_Capabilities_Extended", 16: "Extended_Control", 17: "EPR_Source_Capabilities",
	18: "EPR_Sink_Capabilities", 30: "Vendor_Defined_Extended",
}

func recognizedSOP(sop string) bool {
	switch sop {
	case "SOP", "SOP'", "SOP''", "SOP'_DEBUG", "SOP''_DEBUG":
		return true
	}
	return false
}

// Decode accepts a PD header followed by its payload, with an optional
// trailing CRC32. It retains malformed packets, recording the reason in Error.
// Analyzer reassembles extended messages that span multiple packets.
func Decode(sop string, raw []byte) Packet {
	p := Packet{SOP: sop, MessageHex: hex.EncodeToString(raw), CRC: "absent", Type: "Malformed"}
	if len(raw) < 2 {
		p.Error = "PD header is shorter than two bytes"
		return p
	}
	if !recognizedSOP(sop) {
		p.Error = "unrecognized SOP token"
	}
	p.Header = binary.LittleEndian.Uint16(raw)
	h := p.Header
	p.MessageID = uint8(h >> 9 & 7)
	p.Extended = h&0x8000 != 0
	count, kind := int(h>>12&7), uint8(h&31)
	p.Revision = [...]string{"1.0", "2.0", "3.x", "reserved"}[h>>6&3]
	if sop == "SOP" {
		p.PowerRole = [...]string{"Sink", "Source"}[h>>8&1]
		p.DataRole = [...]string{"UFP", "DFP"}[h>>5&1]
	} else {
		p.PowerRole = [...]string{"Port", "Cable"}[h>>8&1]
	}
	expected := 2 + 4*count
	dataEnd := expected
	var extension Extended
	if p.Extended {
		p.CRC = "unverified"
		if h>>6&3 < 2 {
			p.Error = "extended PD message has invalid revision " + p.Revision
			return p
		}
		if len(raw) < 4 {
			p.Error = "extended PD message is missing its extended header"
			return p
		}
		eh := binary.LittleEndian.Uint16(raw[2:])
		extension = Extended{DataSize: eh & 511, Chunked: eh&0x8000 != 0,
			ChunkNumber: uint8(eh >> 11 & 15), RequestChunk: eh&0x400 != 0}
		p.Type = extendedTypes[kind]
		if p.Type == "" {
			p.Type = fmt.Sprintf("Reserved_%d", kind)
		}
		p.Extension = &extension
		payloadSize, err := extension.payloadSize()
		if err != nil {
			p.Error = err.Error()
			return p
		}
		dataEnd = 4 + payloadSize
		if extension.Chunked && expected != 2+((payloadSize+2+3)/4)*4 {
			p.Error = fmt.Sprintf("extended PD chunk size %d does not match %d data objects", payloadSize, count)
			return p
		}
		if !extension.Chunked {
			padded := expected > dataEnd && expected-dataEnd <= 3 && (len(raw) == expected || len(raw) == expected+4)
			if !padded {
				expected = dataEnd
				count = 0
			}
		}
	}
	if len(raw) != expected && len(raw) != expected+4 {
		if p.Extended && !extension.Chunked {
			p.Error = fmt.Sprintf("PD length %d does not match extended payload size %d", len(raw), extension.DataSize)
		} else {
			p.Error = fmt.Sprintf("PD length %d does not match %d data objects", len(raw), count)
		}
		p.CRC = "unverified"
		return p
	}
	if p.Extended {
		p.CRC = "absent"
	} else {
		p.Type = fmt.Sprintf("Reserved_%d", kind)
		if count > 0 {
			if name, ok := dataTypes[kind]; ok {
				p.Type = name
			}
		} else if int(kind) < len(controlTypes) {
			p.Type = controlTypes[kind]
		}
	}
	if len(raw) == expected+4 {
		p.CRC = "valid"
		if crc32.ChecksumIEEE(raw[:expected]) != binary.LittleEndian.Uint32(raw[expected:]) {
			p.CRC, p.Error = "invalid", "PD CRC32 mismatch"
		}
	}
	for i := 0; i < count; i++ {
		p.Objects = append(p.Objects, binary.LittleEndian.Uint32(raw[2+i*4:]))
	}
	if p.Extended {
		payload := raw[4:dataEnd]
		extension.DataHex = hex.EncodeToString(payload)
		if p.Error == "" && !extension.RequestChunk && len(payload) == int(extension.DataSize) {
			p.decodeExtended(payload)
		}
		return p
	}
	switch p.Type {
	case "Source_Capabilities", "Sink_Capabilities":
		for i, object := range p.Objects {
			p.PDOs = append(p.PDOs, decodePDO(i+1, object, p.Type == "Source_Capabilities"))
		}
	case "Request", "EPR_Request":
		expectedObjects := 1
		if p.Type == "EPR_Request" {
			expectedObjects = 2
		}
		if len(p.Objects) != expectedObjects {
			p.Error = fmt.Sprintf("%s requires %d data objects", p.Type, expectedObjects)
			break
		}
		request := DecodeRequest(p.Objects[0], nil)
		if p.Type == "EPR_Request" && request.Position >= 1 && request.Position <= 11 && p.Objects[1] != 0 {
			pdo := DecodePDO(request.Position, p.Objects[1])
			p.RequestedPDO = &pdo
			request = decodeRequest(p.Objects[0], &pdo)
			request.Matched = false
		}
		p.Request = &request
	case "EPR_Mode":
		if len(p.Objects) == 1 {
			p.EPRMode = &EPRMode{Action: uint8(p.Objects[0] >> 24), Data: uint8(p.Objects[0] >> 16)}
		}
	case "Revision":
		if p.Revision == "3.x" && len(p.Objects) == 1 {
			object := p.Objects[0]
			p.RevisionInfo = &RevisionInfo{
				Revision: fmt.Sprintf("%d.%d", object>>28, object>>24&15),
				Version:  fmt.Sprintf("%d.%d", object>>20&15, object>>16&15),
			}
		}
	case "Vendor_Defined":
		if len(p.Objects) > 0 {
			v := p.Objects[0]
			p.VDM = &VDM{VendorID: uint16(v >> 16), Structured: v&0x8000 != 0,
				Version: uint8(v >> 13 & 3), ObjectPosition: uint8(v >> 8 & 7), CommandType: uint8(v >> 6 & 3), Command: uint8(v & 31)}
		}
	}
	return p
}

func (p Packet) GoodCRC() bool { return !p.Extended && p.Header>>12&7 == 0 && p.Header&31 == 1 }

func (p Packet) KeepAlive() bool {
	return p.Error == "" && p.Extension != nil &&
		(p.Extension.Control == "EPR_Keep_Alive" || p.Extension.Control == "EPR_Keep_Alive_Ack")
}

func Clone(p Packet) Packet {
	p.Objects = slices.Clone(p.Objects)
	p.PDOs = slices.Clone(p.PDOs)
	if p.Request != nil {
		v := *p.Request
		p.Request = &v
	}
	if p.RequestedPDO != nil {
		value := *p.RequestedPDO
		p.RequestedPDO = &value
	}
	if p.EPRMode != nil {
		value := *p.EPRMode
		p.EPRMode = &value
	}
	if p.PPSStatus != nil {
		value := *p.PPSStatus
		p.PPSStatus = &value
		if value.VoltageV != nil {
			p.PPSStatus.VoltageV = new(*value.VoltageV)
		}
		if value.CurrentA != nil {
			p.PPSStatus.CurrentA = new(*value.CurrentA)
		}
	}
	if p.Capabilities != nil {
		value := *p.Capabilities
		p.Capabilities = &value
		if value.EPR != nil {
			p.Capabilities.EPR = new(*value.EPR)
		}
	}
	if p.VDM != nil {
		v := *p.VDM
		p.VDM = &v
	}
	if p.Extension != nil {
		v := *p.Extension
		p.Extension = &v
	}
	if p.RevisionInfo != nil {
		v := *p.RevisionInfo
		p.RevisionInfo = &v
	}
	if p.Analysis != nil {
		p.Analysis = cloneAnalysis(p.Analysis)
	}
	return p
}
