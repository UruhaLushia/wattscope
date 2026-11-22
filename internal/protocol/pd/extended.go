package pd

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	maxExtendedSize = 260
	maxChunkSize    = 26
)

type EPRMode struct {
	Action uint8 `json:"action"`
	Data   uint8 `json:"data"`
}

type PPSStatus struct {
	VoltageV     *float64 `json:"voltage_v,omitempty"`
	CurrentA     *float64 `json:"current_a,omitempty"`
	Temperature  string   `json:"temperature"`
	CurrentLimit bool     `json:"current_limit"`
}

type CapabilitiesExtended struct {
	VID      uint16     `json:"vid"`
	PID      uint16     `json:"pid"`
	XID      uint32     `json:"xid"`
	Firmware uint8      `json:"firmware_version"`
	Hardware uint8      `json:"hardware_version"`
	SPR      PowerData  `json:"spr"`
	EPR      *PowerData `json:"epr,omitempty"`
}

type PowerData struct {
	MinimumW   uint8 `json:"minimum_w,omitempty"`
	OperatingW uint8 `json:"operating_w,omitempty"`
	MaximumW   uint8 `json:"maximum_w"`
}

func (ext Extended) payloadSize() (int, error) {
	size := int(ext.DataSize)
	if size > maxExtendedSize {
		return 0, fmt.Errorf("extended PD data size %d exceeds %d bytes", size, maxExtendedSize)
	}
	if !ext.Chunked {
		if ext.ChunkNumber != 0 || ext.RequestChunk {
			return 0, errors.New("invalid unchunked PD header")
		}
		return size, nil
	}
	if ext.ChunkNumber >= maxExtendedSize/maxChunkSize {
		return 0, fmt.Errorf("invalid PD chunk number %d", ext.ChunkNumber)
	}
	if ext.RequestChunk {
		if size != 0 {
			return 0, errors.New("PD chunk request has nonzero data size")
		}
		return 0, nil
	}
	offset := int(ext.ChunkNumber) * maxChunkSize
	if offset >= size && ext.ChunkNumber != 0 {
		return 0, fmt.Errorf("PD chunk %d lies beyond data size %d", ext.ChunkNumber, size)
	}
	return min(maxChunkSize, size-offset), nil
}

func (p *Packet) decodeExtended(payload []byte) {
	switch p.Type {
	case "Source_Capabilities_Extended", "Sink_Capabilities_Extended":
		source := p.Type == "Source_Capabilities_Extended"
		if len(payload) < 24 {
			p.Error = "extended capabilities payload is shorter than 24 bytes"
			return
		}
		caps := CapabilitiesExtended{
			VID: binary.LittleEndian.Uint16(payload), PID: binary.LittleEndian.Uint16(payload[2:]),
			XID: binary.LittleEndian.Uint32(payload[4:]), Firmware: payload[8], Hardware: payload[9],
		}
		if source {
			caps.SPR.MaximumW = payload[23]
			if len(payload) >= 25 {
				caps.EPR = &PowerData{MaximumW: payload[24]}
			}
		} else {
			if payload[10] != 1 {
				return
			}
			caps.SPR = PowerData{MinimumW: payload[18], OperatingW: payload[19], MaximumW: payload[20]}
			caps.EPR = &PowerData{MinimumW: payload[21], OperatingW: payload[22], MaximumW: payload[23]}
		}
		p.Capabilities = &caps
	case "EPR_Source_Capabilities", "EPR_Sink_Capabilities":
		if len(payload) == 0 || len(payload)%4 != 0 || len(payload) > 44 {
			p.Error = fmt.Sprintf("invalid EPR capabilities size %d", len(payload))
			return
		}
		for offset := 0; offset < len(payload); offset += 4 {
			raw := binary.LittleEndian.Uint32(payload[offset:])
			if raw != 0 {
				p.PDOs = append(p.PDOs, decodePDO(offset/4+1, raw, p.Type == "EPR_Source_Capabilities"))
			}
		}
	case "Extended_Control":
		if len(payload) < 2 {
			p.Error = "extended control payload is shorter than two bytes"
			return
		}
		if payload[0] >= 1 && payload[0] <= 4 && payload[1] == 0 {
			p.Extension.Control = [...]string{"", "EPR_Get_Source_Cap", "EPR_Get_Sink_Cap", "EPR_Keep_Alive", "EPR_Keep_Alive_Ack"}[payload[0]]
		}
	case "PPS_Status":
		if len(payload) < 4 {
			p.Error = "PPS status payload is shorter than four bytes"
			return
		}
		status := PPSStatus{
			Temperature:  [...]string{"unsupported", "normal", "warning", "over_temperature"}[payload[3]>>1&3],
			CurrentLimit: payload[3]&8 != 0,
		}
		if voltage := binary.LittleEndian.Uint16(payload); voltage != 0xffff {
			status.VoltageV = new(float64(voltage) / 50)
		}
		if current := payload[2]; current != 0xff {
			status.CurrentA = new(float64(current) / 20)
		}
		p.PPSStatus = &status
	}
}

type extendedAssembly struct {
	kind      string
	size      uint16
	messageID uint8
	data      []byte
}

func (c *conversation) observeExtended(p *Packet) {
	ext := p.Extension
	if ext == nil || ext.RequestChunk {
		delete(c.chunks, p.PowerRole)
		return
	}
	pending := c.chunks[p.PowerRole]
	if !ext.Chunked || len(ext.DataHex)/2 == int(ext.DataSize) {
		delete(c.chunks, p.PowerRole)
		return
	}
	if ext.ChunkNumber == 0 {
		pending = extendedAssembly{kind: p.Type, size: ext.DataSize, messageID: p.MessageID, data: make([]byte, 0, ext.DataSize)}
	}
	offset := int(ext.ChunkNumber) * maxChunkSize
	if pending.kind != p.Type || pending.size != ext.DataSize || len(pending.data) != offset ||
		ext.ChunkNumber != 0 && p.MessageID != (pending.messageID+1)&7 {
		delete(c.chunks, p.PowerRole)
		return
	}
	data, err := hex.AppendDecode(pending.data, []byte(ext.DataHex))
	if err != nil {
		delete(c.chunks, p.PowerRole)
		return
	}
	pending.data = data
	pending.messageID = p.MessageID
	if len(data) < int(pending.size) {
		if c.chunks == nil {
			c.chunks = make(map[string]extendedAssembly)
		}
		c.chunks[p.PowerRole] = pending
		return
	}
	delete(c.chunks, p.PowerRole)
	ext.ReassembledHex = hex.EncodeToString(data)
	p.decodeExtended(data)
}
