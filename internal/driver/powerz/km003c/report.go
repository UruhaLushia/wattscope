package km003c

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

const (
	rejectCommand = 0x06
	getData       = 0x0c
	putData       = 0x41
	attrADC       = 0x01
	attrPD        = 0x10
)

func incompleteReport(raw []byte) bool {
	for offset := 4; ; {
		if len(raw)-offset < 4 {
			return true
		}
		header := binary.LittleEndian.Uint32(raw[offset:])
		offset += 4 + int(header>>22)
		if offset > len(raw) {
			return true
		}
		if header&0x8000 == 0 {
			return false
		}
	}
}

func decodeReport(raw []byte, events []device.Event) ([]device.Event, error) {
	if len(raw) < 4 || raw[0]&0x7f != putData {
		return events, fmt.Errorf("%w: invalid KM003C response header", device.ErrInvalidReport)
	}
	raw = raw[4:]
	if len(raw) == 0 {
		return events, device.ErrIgnoredReport
	}
	for {
		if len(raw) < 4 {
			return events, fmt.Errorf("%w: incomplete KM003C data header", device.ErrInvalidReport)
		}
		header := binary.LittleEndian.Uint32(raw)
		size := int(header >> 22)
		raw = raw[4:]
		if len(raw) < size {
			return events, fmt.Errorf("%w: KM003C attribute 0x%04x declares %d bytes, received %d",
				device.ErrInvalidReport, header&0x7fff, size, len(raw))
		}
		payload := raw[:size]
		switch header & 0x7fff {
		case attrADC:
			value, err := decodeADC(payload)
			if err != nil {
				return events, err
			}
			events = append(events, device.Event{Sample: &value})
		case attrPD:
			var err error
			events, err = appendPD(events, payload)
			if err != nil {
				return events, err
			}
		}
		raw = raw[size:]
		if header&0x8000 == 0 {
			break
		}
	}
	if len(events) == 0 {
		return events, device.ErrIgnoredReport
	}
	return events, nil
}

func decodeADC(raw []byte) (device.Sample, error) {
	if len(raw) != 44 {
		return device.Sample{}, fmt.Errorf("%w: KM003C ADC block has %d bytes", device.ErrInvalidReport, len(raw))
	}
	voltage := float64(int32(binary.LittleEndian.Uint32(raw))) / 1e6
	current := float64(int32(binary.LittleEndian.Uint32(raw[4:]))) / 1e6
	sample := measurement(voltage, current)
	temp := float64(int16(binary.LittleEndian.Uint16(raw[24:]))) / 128
	cc1 := float64(binary.LittleEndian.Uint16(raw[26:])) / 1e4
	cc2 := float64(binary.LittleEndian.Uint16(raw[28:])) / 1e4
	dp := float64(binary.LittleEndian.Uint16(raw[30:])) / 1e4
	dn := float64(binary.LittleEndian.Uint16(raw[32:])) / 1e4
	sample.Temperature, sample.CC1, sample.CC2, sample.DP, sample.DN = &temp, &cc1, &cc2, &dp, &dn
	return sample, nil
}

func measurement(voltage, signedCurrent float64) device.Sample {
	voltage = max(0, voltage)
	sample := device.Sample{
		Voltage: voltage, Current: math.Abs(signedCurrent),
		Power: voltage * math.Abs(signedCurrent), SignedPower: voltage * signedCurrent,
	}
	if signedCurrent > 0 {
		sample.CurrentDirection = device.CurrentReceptacleToPlug
	} else if signedCurrent < 0 {
		sample.CurrentDirection = device.CurrentPlugToReceptacle
	}
	return sample
}

func appendPD(events []device.Event, raw []byte) ([]device.Event, error) {
	if len(raw) < 12 {
		return events, fmt.Errorf("%w: truncated KM003C PD status", device.ErrInvalidReport)
	}
	for raw = raw[12:]; len(raw) != 0; {
		if len(raw) < 6 {
			return events, fmt.Errorf("%w: truncated KM003C PD event", device.ErrInvalidReport)
		}
		if raw[0] == 0x45 {
			switch raw[5] {
			case 0x11, 0x12, 0x21, 0x22:
				events = append(events, device.Event{PDLinkReset: true})
			}
			raw = raw[6:]
			continue
		}
		size := int(raw[0]&0x3f) - 5
		if raw[0]&0x80 == 0 || size < 0 || len(raw)-6 < size {
			return events, fmt.Errorf("%w: invalid KM003C PD event length", device.ErrInvalidReport)
		}
		sopNames := [...]string{"SOP", "SOP'", "SOP''", "SOP'_DEBUG", "SOP''_DEBUG"}
		address := int(raw[5])
		sop := ""
		if address < len(sopNames) {
			sop = sopNames[address]
		} else {
			sop = fmt.Sprintf("0x%02X", address)
		}
		packet := pd.Decode(sop, raw[6:6+size])
		events = append(events, device.Event{PD: &packet})
		raw = raw[6+size:]
	}
	return events, nil
}
