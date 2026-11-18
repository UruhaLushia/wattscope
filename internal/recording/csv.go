// Package recording provides incremental, bounded-memory measurement storage.
package recording

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

// CSV is owned by the acquisition service, not shared between goroutines.
type CSV struct {
	file      *os.File
	writer    *csv.Writer
	lastFlush time.Time
	Path      string
	Samples   uint64
}

func Create(path string) (*CSV, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("recording path is empty")
	}
	f, err := createFile(path)
	if err != nil {
		return nil, err
	}
	r := &CSV{file: f, writer: csv.NewWriter(f), Path: f.Name(), lastFlush: time.Now()}
	err = r.writer.Write([]string{"timestamp", "elapsed_s", "segment", "simulated", "voltage_v", "current_a", "power_w", "signed_power_w", "temperature_c", "external_temperature_c", "dp_v", "dn_v", "device_ah", "device_wh", "device_record_s", "device_uptime_s", "group"})
	if err == nil {
		err = r.Flush()
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}

func (r *CSV) Write(s device.Sample) error {
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	optional := func(v *float64) string {
		if v == nil {
			return ""
		}
		return f(*v)
	}
	optionalInt := func(v *uint32) string {
		if v == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*v), 10)
	}
	group := ""
	if s.Group != nil {
		group = strconv.FormatUint(uint64(*s.Group), 10)
	}
	err := r.writer.Write([]string{
		s.Timestamp.Format(time.RFC3339Nano), f(s.Elapsed), strconv.FormatUint(s.Segment, 10), strconv.FormatBool(s.Simulated),
		f(s.Voltage), f(s.Current), f(s.Power), f(s.SignedPower), optional(s.Temperature), optional(s.ExternalTemperature),
		optional(s.DP), optional(s.DN), optional(s.AmpHours), optional(s.WattHours), optionalInt(s.RecordSeconds),
		optionalInt(s.Uptime), group,
	})
	if err != nil {
		return fmt.Errorf("write recording: %w", err)
	}
	r.Samples++
	if time.Since(r.lastFlush) >= time.Second {
		return r.Flush()
	}
	return nil
}

func (r *CSV) Flush() error {
	r.writer.Flush()
	r.lastFlush = time.Now()
	return r.writer.Error()
}

func (r *CSV) Close() error { return errors.Join(r.Flush(), r.file.Sync(), r.file.Close()) }
