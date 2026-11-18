package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
)

type recordingError struct{ error }

const (
	minActiveCurrent    = 0.01
	activeCurrentTime   = time.Second
	maxCurrentSampleGap = 500 * time.Millisecond
)

type currentDirectionTracker struct {
	direction device.CurrentDirection
	since     time.Time
	last      time.Time
}

func (tracker *currentDirectionTracker) update(sample *device.Sample, now time.Time) bool {
	if !(sample.Current >= minActiveCurrent) || sample.CurrentDirection == "" {
		*tracker = currentDirectionTracker{}
		return false
	}
	if tracker.direction != sample.CurrentDirection || now.Sub(tracker.last) > maxCurrentSampleGap {
		tracker.direction = sample.CurrentDirection
		tracker.since = now
	}
	tracker.last = now
	return now.Sub(tracker.since) >= activeCurrentTime
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
		v.CurrentDirectionStable = currentDirection.update(&v, now)
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
	if s.snapshot.Latest == nil {
		s.snapshot.Latest = new(device.Sample)
	}
	*s.snapshot.Latest = v
	s.previous = s.snapshot.Latest
	s.snapshot.State = Streaming
	s.snapshot.Error = ""
	s.snapshot.ErrorKind = ""
	s.snapshot.HistoryVersion++
	if s.opts.HistorySize < 0 || len(s.ring) < s.opts.HistorySize {
		s.ring = append(s.ring, v)
	} else {
		s.ring[s.head] = v
		s.head++
		if s.head == len(s.ring) {
			s.head = 0
		}
	}
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
		var recordingErr *recordingError
		if errors.As(err, &recordingErr) {
			return "recording"
		}
		return "connection"
	}
}
