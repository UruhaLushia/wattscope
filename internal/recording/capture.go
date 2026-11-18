package recording

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

// Capture stores every received PD packet as NDJSON, independently of UI refresh
// or list filters. Like CSV, it is exclusively owned by the acquisition service.
type Capture struct {
	file      *os.File
	buffer    *bufio.Writer
	encoder   *json.Encoder
	lastFlush time.Time
	Path      string
	Packets   uint64
}

func CreateCapture(path string) (*Capture, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("capture path is empty")
	}
	file, err := createFile(path)
	if err != nil {
		return nil, err
	}
	buffer := bufio.NewWriter(file)
	return &Capture{file: file, buffer: buffer, encoder: json.NewEncoder(buffer), Path: file.Name(), lastFlush: time.Now()}, nil
}

func (c *Capture) Write(packet pd.Packet) error {
	if err := c.encoder.Encode(packet); err != nil {
		return err
	}
	c.Packets++
	if time.Since(c.lastFlush) >= time.Second {
		return c.Flush()
	}
	return nil
}

func (c *Capture) Flush() error { c.lastFlush = time.Now(); return c.buffer.Flush() }
func (c *Capture) Close() error { return errors.Join(c.Flush(), c.file.Sync(), c.file.Close()) }

func ExportCapture(path string, packets []pd.Packet) error {
	return ExportCaptureContext(context.Background(), path, packets)
}

func ExportCaptureContext(ctx context.Context, path string, packets []pd.Packet) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := CreateCapture(path)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, c.file.Close(), os.Remove(c.Path))
		} else {
			err = c.Close()
		}
	}()
	for _, packet := range packets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.Write(packet); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// LoadCapture validates a WattScope NDJSON recording and keeps its last limit
// packets. All lines are checked; decoded content is rebuilt from wire bytes.
// Memory is bounded even for large recording files.
func LoadCapture(reader io.Reader, limit int) ([]pd.Packet, uint64, error) {
	if limit < 1 || limit > 100000 {
		return nil, 0, errors.New("invalid capture history size")
	}
	ring := make([]pd.Packet, limit)
	var index uint64
	count, err := ReadCapture(reader, func(packet pd.Packet) error {
		ring[index%uint64(limit)] = packet
		index++
		return nil
	})
	if err != nil {
		return nil, count, err
	}
	n := int(min(count, uint64(limit)))
	packets := make([]pd.Packet, n)
	start := count - uint64(n)
	for i := range packets {
		packets[i] = ring[(start+uint64(i))%uint64(limit)]
	}
	return packets, count, nil
}

// ReadCapture validates, redecodes and analyzes every packet in order without
// retaining the entire file. Stored analysis is never trusted on import.
func ReadCapture(reader io.Reader, visit func(pd.Packet) error) (uint64, error) {
	var count uint64
	var analyzer pd.Analyzer
	var segment uint64
	var previousSequence uint64
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 256*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var stored pd.Packet
		if err := json.Unmarshal(scanner.Bytes(), &stored); err != nil {
			return count, fmt.Errorf("capture line %d: %w", line, err)
		}
		if stored.Sequence <= previousSequence || stored.Timestamp.IsZero() || len(stored.SOP) > 32 || len(stored.MessageHex) > 1040 || len(stored.ReportHex) > 130 || math.IsNaN(stored.Elapsed) || math.IsInf(stored.Elapsed, 0) || stored.Elapsed < 0 {
			return count, fmt.Errorf("capture line %d: invalid packet metadata", line)
		}
		previousSequence = stored.Sequence
		raw, err := hex.DecodeString(stored.MessageHex)
		if err != nil {
			return count, fmt.Errorf("capture line %d: %w", line, err)
		}
		if _, err := hex.DecodeString(stored.ReportHex); err != nil {
			return count, fmt.Errorf("capture line %d: invalid report hex", line)
		}
		packet := pd.Decode(stored.SOP, raw)
		packet.ReportHex = stored.ReportHex
		if len(raw) == 0 && stored.ReportHex != "" {
			packet.Error = "malformed capture envelope"
		}
		packet.Sequence, packet.Timestamp, packet.Elapsed, packet.Segment, packet.Simulated = stored.Sequence, stored.Timestamp, stored.Elapsed, stored.Segment, stored.Simulated
		if count == 0 || segment != packet.Segment {
			analyzer.Reset()
			segment = packet.Segment
		}
		analyzer.Observe(&packet)
		count++
		if err := visit(packet); err != nil {
			return count, err
		}
	}
	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("capture line %d: %w", line+1, err)
	}
	return count, nil
}
