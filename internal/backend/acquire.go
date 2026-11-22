package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
)

type recordingError struct{ error }

const (
	directionVoltageOn       = 1.0
	directionVoltageOff      = 0.5
	directionCurrentMin      = 0.005
	directionReverseCurrent  = 0.02
	directionConfirmDuration = 250 * time.Millisecond
	maxDirectionSampleGap    = 500 * time.Millisecond
)

type currentDirectionTracker struct {
	direction device.CurrentDirection
	pending   device.CurrentDirection
	since     time.Time
	last      time.Time
	powered   bool
}

func (tracker *currentDirectionTracker) update(sample *device.Sample, now time.Time) bool {
	if sample == nil || math.IsNaN(sample.Voltage) || math.IsInf(sample.Voltage, 0) ||
		math.IsNaN(sample.Current) || math.IsInf(sample.Current, 0) ||
		sample.Voltage < 0 || sample.Current < 0 {
		*tracker = currentDirectionTracker{}
		return false
	}

	if !tracker.powered {
		if sample.Voltage < directionVoltageOn {
			*tracker = currentDirectionTracker{}
			return false
		}
		tracker.powered = true
	} else if sample.Voltage <= directionVoltageOff {
		*tracker = currentDirectionTracker{}
		return false
	}

	if !tracker.last.IsZero() && now.Sub(tracker.last) > maxDirectionSampleGap {
		tracker.pending = ""
		tracker.since = time.Time{}
	}
	tracker.last = now

	candidate := sample.CurrentDirection
	if sample.Current < directionCurrentMin || !validCurrentDirection(candidate) {
		candidate = ""
	}
	if tracker.direction != "" {
		if candidate == tracker.direction || candidate == "" || sample.Current < directionReverseCurrent {
			tracker.pending = ""
			tracker.since = time.Time{}
			return true
		}
		if tracker.pending != candidate {
			tracker.pending = candidate
			tracker.since = now
			return true
		}
		if now.Sub(tracker.since) >= directionConfirmDuration {
			tracker.direction = candidate
			tracker.pending = ""
			tracker.since = time.Time{}
		}
		return true
	}
	if candidate == "" {
		tracker.pending = ""
		tracker.since = time.Time{}
		return false
	}
	if tracker.pending != candidate {
		tracker.pending = candidate
		tracker.since = now
		return false
	}
	if now.Sub(tracker.since) >= directionConfirmDuration {
		tracker.direction = candidate
		tracker.pending = ""
		tracker.since = time.Time{}
	}
	return tracker.direction != ""
}

func (tracker *currentDirectionTracker) apply(sample *device.Sample, now time.Time) {
	if sample == nil {
		tracker.update(nil, now)
		return
	}
	sample.CurrentDirectionStable = tracker.update(sample, now)
	if sample.CurrentDirectionStable {
		sample.CurrentDirection = tracker.direction
	} else {
		sample.CurrentDirection = ""
	}
}

func validCurrentDirection(direction device.CurrentDirection) bool {
	switch direction {
	case device.CurrentPlugToReceptacle, device.CurrentReceptacleToPlug:
		return true
	default:
		return false
	}
}

func (s *Service) acquire(ctx context.Context, reader device.Connection, started time.Time, simulated bool, logger *slog.Logger) error {
	openedAt := time.Now()
	lastValid := openedAt
	lastSlot := int64(-1)
	receivedSample, loggedInvalid := false, false
	var lastInvalid, lastPollTimeout error
	var currentDirection currentDirectionTracker
	reportTimeout := func(cause error) error {
		summary := fmt.Sprintf("no valid meter reports for %s", s.opts.StaleAfter)
		switch {
		case lastInvalid != nil:
			return fmt.Errorf("%s: %w: last invalid frame: %v", summary, cause, lastInvalid)
		case lastPollTimeout != nil:
			return fmt.Errorf("%s: %w: last USB poll: %v", summary, cause, lastPollTimeout)
		default:
			return fmt.Errorf("%s: %w", summary, cause)
		}
	}
	for {
		readCtx, cancel := context.WithDeadline(ctx, lastValid.Add(s.opts.StaleAfter))
		if err := readCtx.Err(); err != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return reportTimeout(err)
		}
		var event device.Event
		var err error
		if events, ok := reader.(device.EventConnection); ok {
			event, err = events.ReadEvent(readCtx)
		} else {
			var sample device.Sample
			sample, err = reader.Read(readCtx)
			if err == nil {
				event.Sample = &sample
			}
		}
		cancel()
		invalid := errors.Is(err, device.ErrInvalidReport)
		if invalid || errors.Is(err, device.ErrIgnoredReport) {
			if invalid {
				lastInvalid = err
				if !loggedInvalid {
					logger.WarnContext(ctx, diagnostics.InvalidReport, "stage", "decode", "error", err)
					loggedInvalid = true
				}
			} else if errors.Is(err, context.DeadlineExceeded) {
				lastPollTimeout = err
				continue
			}
			s.mu.Lock()
			if invalid {
				s.snapshot.InvalidReports++
				s.snapshot.Error = err.Error()
				s.snapshot.ErrorKind = "report"
			} else {
				s.snapshot.OtherReports++
			}
			s.notifyLocked()
			s.mu.Unlock()
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return reportTimeout(err)
			}
			return err
		}
		now := time.Now()
		if event.PDLinkReset {
			s.mu.Lock()
			s.segment++
			s.previous = nil
			s.mu.Unlock()
		}
		if event.PD != nil {
			packet := *event.PD
			packet.Timestamp, packet.Elapsed, packet.Simulated = now.UTC(), now.Sub(started).Seconds(), simulated
			if packet.Error == "" {
				lastValid = now
				lastInvalid, lastPollTimeout = nil, nil
			}
			if err := s.acceptPacket(packet); err != nil {
				return err
			}
		}
		if event.Sample == nil {
			continue
		}
		v := *event.Sample
		v.Timestamp = now.UTC()
		currentDirection.apply(&v, now)
		if v.Elapsed <= 0 {
			v.Elapsed = now.Sub(started).Seconds()
		}
		lastValid = now
		lastInvalid, lastPollTimeout = nil, nil
		if s.opts.Rate > 0 {
			slot := int64(v.Elapsed * float64(s.opts.Rate))
			if slot == lastSlot {
				continue
			}
			lastSlot = slot
		}
		v.Simulated = simulated
		if !receivedSample {
			logger.InfoContext(ctx, diagnostics.FirstMeasurement, "after_open", now.Sub(openedAt).Round(time.Millisecond))
			receivedSample = true
		}
		if err := s.accept(v); err != nil {
			return &recordingError{err}
		}
	}
}

func (s *Service) accept(v device.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v = device.Clone(v)
	if p := s.previous; p != nil && (different(p.Group, v.Group) || decreased(p.Uptime, v.Uptime) || decreased(p.RecordSeconds, v.RecordSeconds)) {
		s.segment++
	}
	v.Segment = s.segment
	s.snapshot.Statistics.add(v, s.previous, s.opts.StaleAfter.Seconds())
	values := [7]*float64{&v.Power, &v.Voltage, &v.Current, v.Temperature, v.DP, v.DN, v.ExternalTemperature}
	for index, value := range values {
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
			continue
		}
		bounds := &s.snapshot.ChartRanges[index]
		if !bounds.Valid {
			*bounds = ChartRange{Min: *value, Max: *value, Valid: true}
		} else {
			bounds.Min = min(bounds.Min, *value)
			bounds.Max = max(bounds.Max, *value)
		}
	}
	if s.snapshot.Latest == nil {
		s.snapshot.Latest = new(device.Sample)
	}
	*s.snapshot.Latest = v
	s.previous = s.snapshot.Latest
	s.snapshot.State = Streaming
	s.snapshot.Error = ""
	s.snapshot.ErrorKind = ""
	s.snapshot.HistoryVersion++
	var historyIndex int
	if s.opts.HistorySize < 0 || len(s.ring) < s.opts.HistorySize {
		historyIndex = len(s.ring)
		s.ring = append(s.ring, v)
	} else {
		historyIndex = s.head
		s.ring[historyIndex] = v
		s.head++
		if s.head == len(s.ring) {
			s.head = 0
		}
	}
	s.rebuildHistoryBlockLocked(historyIndex)
	if s.recorder != nil {
		if err := s.recorder.Write(v); err != nil {
			return fmt.Errorf("recording failed: %w", err)
		}
	}
	s.notifyLocked()
	return nil
}

func different[T comparable](a, b *T) bool { return a != nil && b != nil && *a != *b }
func decreased(a, b *uint32) bool          { return a != nil && b != nil && *b < *a }

func errorKind(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "permission"
	case errors.Is(err, device.ErrNoDevice):
		return "missing"
	case errors.Is(err, device.ErrMultipleDevices):
		return "selection"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		if _, ok := errors.AsType[*recordingError](err); ok {
			return "recording"
		}
		return "connection"
	}
}
