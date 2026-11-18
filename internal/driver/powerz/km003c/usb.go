package km003c

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
)

const (
	endpointOut    = 0x01
	endpointIn     = 0x81
	bulkPacketSize = 64
	pollInterval   = 40 * time.Millisecond
	vendorID       = 0x5fc9
	productID      = 0x0063
)

var errBulkAborted = errors.New("USB transfer aborted")

func matchesUSBDescriptor(raw []byte) bool {
	return len(raw) >= 18 && raw[0] == 18 && raw[1] == 1 &&
		binary.LittleEndian.Uint16(raw[8:]) == vendorID && binary.LittleEndian.Uint16(raw[10:]) == productID
}

type Driver struct{}

func (Driver) ID() string   { return "powerz-km003c" }
func (Driver) Name() string { return "POWER-Z KM003C" }

type bulkTransport interface {
	bulk(context.Context, byte, []byte) (int, error)
	Close() error
}

type usbConnection struct {
	transport                  bulkTransport
	id                         byte
	nextPoll                   time.Time
	receive                    [4096]byte
	pending                    []device.Event
	nextEvent                  int
	pendingErr                 error
	sentRequest, receivedReply bool
}

func (conn *usbConnection) Close() error {
	return conn.transport.Close()
}

func (conn *usbConnection) Read(ctx context.Context) (device.Sample, error) {
	for {
		event, err := conn.ReadEvent(ctx)
		if err != nil {
			return device.Sample{}, err
		}
		if event.Sample != nil {
			return *event.Sample, nil
		}
	}
}

func (conn *usbConnection) ReadEvent(ctx context.Context) (device.Event, error) {
	if err := ctx.Err(); err != nil {
		return device.Event{}, err
	}
	if conn.nextEvent == len(conn.pending) && conn.pendingErr == nil {
		if err := waitPoll(ctx, conn.nextPoll); err != nil {
			return device.Event{}, err
		}
		conn.nextPoll = time.Now().Add(pollInterval)
		raw, err := conn.poll(ctx)
		if err != nil {
			if ctx.Err() == nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errBulkAborted)) {
				return device.Event{}, fmt.Errorf("%w: %w", device.ErrIgnoredReport, err)
			}
			return device.Event{}, err
		}
		conn.nextEvent = 0
		conn.pending, conn.pendingErr = decodeReport(raw, conn.pending[:0])
		if errors.Is(conn.pendingErr, device.ErrInvalidReport) {
			conn.pendingErr = fmt.Errorf("%w (KM003C reply %d bytes, start % X)", conn.pendingErr, len(raw), raw[:min(16, len(raw))])
		}
	}
	if conn.nextEvent < len(conn.pending) {
		event := conn.pending[conn.nextEvent]
		conn.pending[conn.nextEvent] = device.Event{}
		conn.nextEvent++
		return event, nil
	}
	err := conn.pendingErr
	conn.pendingErr = nil
	return device.Event{}, err
}

func (conn *usbConnection) poll(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	conn.id++
	request := [4]byte{getData, conn.id, (attrADC | attrPD) << 1, 0}
	logger := diagnostics.Logger(ctx)
	if !conn.sentRequest {
		conn.sentRequest = true
		logger.DebugContext(ctx, diagnostics.KM003CRequest, "endpoint", "0x01", "header_bytes", fmt.Sprintf("% X", request))
	}
	written, err := conn.transport.bulk(ctx, endpointOut, request[:])
	if err != nil {
		return nil, fmt.Errorf("KM003C send GetData to endpoint 0x%02x: %w", endpointOut, err)
	}
	if written != len(request) {
		return nil, fmt.Errorf("KM003C send GetData: %w (%d of %d bytes)", io.ErrShortWrite, written, len(request))
	}
	var lastMismatchedID byte
	var mismatched int
	for {
		length, err := conn.readBulk(ctx, conn.receive[:])
		if err != nil {
			if mismatched > 0 {
				return nil, fmt.Errorf("KM003C GetData response: %w (expected transaction 0x%02x, received 0x%02x in last of %d unmatched replies)",
					err, conn.id, lastMismatchedID, mismatched)
			}
			return nil, fmt.Errorf("KM003C read GetData from endpoint 0x%02x: %w", endpointIn, err)
		}
		if length == 0 {
			continue
		}
		raw := conn.receive[:length]
		if !conn.receivedReply {
			conn.receivedReply = true
			logger.DebugContext(ctx, diagnostics.KM003CReply, "endpoint", "0x81", "bytes", length, "header_bytes", fmt.Sprintf("% X", raw[:min(len(raw), 8)]))
		}
		if len(raw) < 4 {
			return nil, fmt.Errorf("%w: KM003C reply header is %d bytes (% X)", device.ErrInvalidReport, len(raw), raw)
		}
		if raw[1] != conn.id {
			lastMismatchedID, mismatched = raw[1], mismatched+1
			continue
		}
		if raw[0]&0x7f == rejectCommand {
			return nil, fmt.Errorf("KM003C rejected GetData (reply % X)", raw[:4])
		}
		if raw[0]&0x7f != putData {
			return nil, fmt.Errorf("%w: unexpected KM003C GetData response (reply % X)", device.ErrInvalidReport, raw[:4])
		}
		return conn.finishReply(ctx, length)
	}
}

func (conn *usbConnection) finishReply(ctx context.Context, length int) ([]byte, error) {
	for length < len(conn.receive) && length%bulkPacketSize == 0 && incompleteReport(conn.receive[:length]) {
		count, err := conn.readBulk(ctx, conn.receive[length:])
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errBulkAborted) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("KM003C read GetData continuation after %d bytes: %w", length, err)
		}
		if count == 0 {
			break
		}
		length += count
	}
	return conn.receive[:length], nil
}

func (conn *usbConnection) readBulk(ctx context.Context, buffer []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	length, err := conn.transport.bulk(ctx, endpointIn, buffer)
	if err != nil {
		return 0, err
	}
	if length < 0 || length > len(buffer) {
		return 0, fmt.Errorf("%w: invalid KM003C USB transfer length %d", device.ErrInvalidReport, length)
	}
	return length, nil
}

func waitPoll(ctx context.Context, nextPoll time.Time) error {
	wait := time.Until(nextPoll)
	if wait <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
	return ctx.Err()
}
