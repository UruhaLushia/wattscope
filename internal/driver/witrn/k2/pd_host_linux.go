//go:build !android || no_gui

package k2

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/UruhaLushia/wattscope/internal/transport/hid"
)

func (c *connection) CheckSavedPDHost(ctx context.Context) (bool, error) {
	const (
		readSettings = 0x19
		pdHostOffset = 0x61
		pdHostBit    = 0x40
	)
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	var report [ReportSize + 1]byte
	frame := report[1:]
	frame[0], frame[1], frame[4] = 0xff, 0x55, 0x80
	frame[8], frame[9] = readSettings, 5
	binary.LittleEndian.PutUint32(frame[10:], pdHostOffset)
	frame[14] = 1
	frame[62], frame[63] = reportChecksums(frame)

	reply, err := hid.Request(ctx, c.device, report[:], func(reply []byte) bool {
		if len(reply) == ReportSize+1 && reply[0] == 0 {
			reply = reply[1:]
		}
		if len(reply) != ReportSize || reply[0] != 0xff || reply[1] != 0x55 || reply[8] != readSettings || reply[9] != 1 {
			return false
		}
		inner, outer := reportChecksums(reply)
		return reply[62] == inner && reply[63] == outer
	})
	if err != nil {
		return false, err
	}
	if len(reply) == ReportSize+1 {
		reply = reply[1:]
	}
	return reply[10]&pdHostBit != 0, nil
}
