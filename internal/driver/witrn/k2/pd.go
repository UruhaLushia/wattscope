package k2

import (
	"encoding/hex"
	"fmt"

	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

func (Driver) DecodePDReport(raw []byte) pd.Packet { return DecodePD(raw) }

// DecodePD interprets the passive FE, length, SOP, PD-message envelope. Unlike
// FF55 telemetry it has no K2 outer checksum. The PD CRC may be absent.
func DecodePD(raw []byte) pd.Packet {
	if len(raw) == ReportSize+1 && raw[0] == 0 {
		raw = raw[1:]
	}
	p := pd.Packet{ReportHex: hex.EncodeToString(raw), Type: "Malformed", CRC: "unverified"}
	if len(raw) < 3 || len(raw) > ReportSize || raw[0] != 0xfe {
		p.Error = "invalid K2 PD envelope"
		return p
	}
	sop := map[byte]string{0xe0: "SOP", 0xc0: "SOP'", 0xa0: "SOP''", 0x80: "SOP'_DEBUG", 0x60: "SOP''_DEBUG"}[raw[2]]
	if sop == "" {
		sop = fmt.Sprintf("0x%02X", raw[2])
	}
	end := int(raw[1]) + 2
	if end < 5 || end > len(raw) {
		p.SOP, p.Error = sop, "invalid K2 PD payload length"
		return p
	}
	decoded := pd.Decode(sop, raw[3:end])
	decoded.ReportHex = p.ReportHex
	return decoded
}
