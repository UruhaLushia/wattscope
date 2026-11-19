package backend

import (
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

type State string

const (
	Stopped    State = "stopped"
	Connecting State = "connecting"
	Waiting    State = "waiting"
	Streaming  State = "streaming"
)

type Snapshot struct {
	State          State           `json:"state"`
	Device         device.Info     `json:"device"`
	Latest         *device.Sample  `json:"latest,omitempty"`
	Statistics     Statistics      `json:"statistics"`
	History        []device.Sample `json:"history,omitempty"`
	HistoryStart   float64         `json:"history_start_s,omitempty"`
	HistoryEnd     float64         `json:"history_end_s,omitempty"`
	HistoryCount   int             `json:"history_count,omitempty"`
	HistoryVersion uint64          `json:"-"`
	ChartRanges    [7]ChartRange   `json:"-"` // power, voltage, current, temperature, D+, D-, external temperature
	InvalidReports uint64          `json:"invalid_reports"`
	OtherReports   uint64          `json:"other_reports"`
	Connections    uint64          `json:"connections"`
	CanCheckPDHost bool            `json:"can_check_pd_host,omitempty"`
	SavedPDHost    *bool           `json:"saved_pd_host,omitempty"`
	Recording      Recording       `json:"recording"`
	Error          string          `json:"error,omitempty"`
	ErrorKind      string          `json:"error_kind,omitempty"`
}

// ChartRange tracks the complete session, including samples outside the view.
type ChartRange struct {
	Min, Max float64
	Valid    bool
}

type ConnectionStatus struct {
	State       State
	DeviceID    string
	Connections uint64
	Error       string
	ErrorKind   string
}

func (s *Service) ConnectionStatus() ConnectionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ConnectionStatus{
		State: s.snapshot.State, DeviceID: s.snapshot.Device.ID,
		Connections: s.snapshot.Connections, Error: s.snapshot.Error, ErrorKind: s.snapshot.ErrorKind,
	}
}

// Subscribe delivers coalesced change notifications, never blocking acquisition.
// Always read Snapshot after a notification. The returned cancel function is
// idempotent and closes the channel. A slow UI cannot lose recording samples.
func (s *Service) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	ch <- struct{}{}
	s.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.subscribers, ch)
			close(ch)
		})
	}
}

func (s *Service) notifyLocked() {
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Snapshot returns owned copies, including optional values. limit=0 omits
// history; a negative limit returns the entire bounded history, oldest first.
func (s *Service) Snapshot(limit int) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked(limit)
}

// SnapshotWindow returns the latest state and just the visible time range,
// independent of the meter's report rate. No sampling-rate guess is required.
func (s *Service) SnapshotWindow(window time.Duration) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ring) == 0 || window <= 0 {
		return s.snapshotLocked(0)
	}
	return s.snapshotLocked(len(s.ring) - s.windowStartLocked(window))
}

// SnapshotWindowSampled returns the latest state and the visible time range
// (or the entire session when window <= 0) as read-only samples that may share
// the append-only history. It uses maxPoints to size buckets and keeps
// endpoints, bucket extrema, and the first transition within each bucket.
func (s *Service) SnapshotWindowSampled(window time.Duration, maxPoints int) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ring) == 0 {
		return s.snapshotLocked(0)
	}
	return s.snapshotSliceLocked(s.windowStartLocked(window), len(s.ring), maxPoints)
}

// SnapshotRangeSampled returns the latest state and read-only samples spanning
// [start, end], using the same bucket selection as SnapshotWindowSampled.
func (s *Service) SnapshotRangeSampled(start, end float64, maxPoints int) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ring) == 0 || end < start {
		return s.snapshotLocked(0)
	}
	startIdx := s.firstSampleAtOrAfterLocked(start)
	endIdx := sort.Search(len(s.ring), func(index int) bool { return s.sampleAtLocked(index).Elapsed > end })
	if startIdx > 0 {
		startIdx--
	}
	if endIdx < len(s.ring) {
		endIdx++
	}
	return s.snapshotSliceLocked(startIdx, endIdx, maxPoints)
}

// ClosestSample returns the sample in the session closest to elapsed seconds.
func (s *Service) ClosestSample(at float64) *device.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ring) == 0 {
		return nil
	}
	idx := s.firstSampleAtOrAfterLocked(at)
	if idx == len(s.ring) {
		idx--
	} else if idx > 0 && at-s.sampleAtLocked(idx-1).Elapsed < s.sampleAtLocked(idx).Elapsed-at {
		idx--
	}
	sample := *s.sampleAtLocked(idx)
	return &sample
}

func (s *Service) windowStartLocked(window time.Duration) int {
	if window <= 0 {
		return 0
	}
	last := s.sampleAtLocked(len(s.ring) - 1).Elapsed
	return s.firstSampleAtOrAfterLocked(last - window.Seconds())
}

func (s *Service) firstSampleAtOrAfterLocked(elapsed float64) int {
	return sort.Search(len(s.ring), func(index int) bool { return s.sampleAtLocked(index).Elapsed >= elapsed })
}

func (s *Service) snapshotSliceLocked(startIdx, endIdx, maxPoints int) Snapshot {
	v := s.snapshotLocked(0)
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(s.ring) {
		endIdx = len(s.ring)
	}
	n := endIdx - startIdx
	if n <= 0 {
		return v
	}
	if maxPoints <= 0 || n <= maxPoints {
		if s.opts.HistorySize < 0 {
			v.History = s.ring[startIdx:endIdx:endIdx]
			return v
		}
		v.History = make([]device.Sample, n)
		copied := copy(v.History, s.ring[s.ringIndexLocked(startIdx):])
		copy(v.History[copied:], s.ring[:n-copied])
		return v
	}
	buckets := max(1, (maxPoints-2)/2)
	out := make([]device.Sample, 0, maxPoints+2)
	out = append(out, *s.sampleAtLocked(startIdx))
	innerStart := startIdx + 1
	innerCount := n - 2
	if innerCount > 0 {
		for b := range buckets {
			lo := innerStart + (b*innerCount)/buckets
			hi := innerStart + ((b+1)*innerCount)/buckets
			if lo >= hi {
				continue
			}
			minIdx, maxIdx := lo, lo
			firstSample := s.sampleAtLocked(lo)
			minP := firstSample.Power
			maxP := minP
			segChange := -1
			prevSeg := firstSample.Segment
			for idx := lo + 1; idx < hi; idx++ {
				sample := s.sampleAtLocked(idx)
				if sample.Power < minP {
					minP, minIdx = sample.Power, idx
				}
				if sample.Power > maxP {
					maxP, maxIdx = sample.Power, idx
				}
				if segChange < 0 && sample.Segment != prevSeg {
					segChange = idx
				}
			}
			selected := [3]int{minIdx, maxIdx, segChange}
			slices.Sort(selected[:])
			previous := -1
			for _, index := range selected {
				if index > previous {
					out = append(out, *s.sampleAtLocked(index))
					previous = index
				}
			}
		}
	}
	out = append(out, *s.sampleAtLocked(endIdx - 1))
	v.History = out
	return v
}

func (s *Service) ringIndexLocked(index int) int {
	index += s.head
	if index >= len(s.ring) {
		index -= len(s.ring)
	}
	return index
}

func (s *Service) sampleAtLocked(index int) *device.Sample {
	return &s.ring[s.ringIndexLocked(index)]
}

func (s *Service) snapshotLocked(limit int) Snapshot {
	v := s.snapshot
	v.CanCheckPDHost = v.State == Streaming && s.pdHostChecker != nil
	if v.SavedPDHost != nil {
		enabled := *v.SavedPDHost
		v.SavedPDHost = &enabled
	}
	v.Device.Capabilities = slices.Clone(v.Device.Capabilities)
	if v.Latest != nil {
		copy := device.Clone(*v.Latest)
		v.Latest = &copy
	}
	if s.recorder != nil {
		v.Recording.Samples = s.recorder.Samples
	}
	count := len(s.ring)
	if count > 0 {
		v.HistoryStart = s.sampleAtLocked(0).Elapsed
		v.HistoryEnd = s.sampleAtLocked(count - 1).Elapsed
		v.HistoryCount = count
	}
	if limit < 0 || limit > count {
		limit = count
	}
	if limit > 0 {
		v.History = make([]device.Sample, limit)
		start := count - limit
		for i := range v.History {
			v.History[i] = device.Clone(*s.sampleAtLocked(start + i))
		}
	}
	return v
}

// ResetStatistics affects host totals and chart history; it does not write to
// the meter or discard any rows already stored by an active recorder.
func (s *Service) ResetStatistics() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Statistics = Statistics{}
	s.snapshot.ChartRanges = [7]ChartRange{}
	s.snapshot.HistoryVersion++
	s.previous = nil
	s.ring = nil
	s.head = 0
	s.notifyLocked()
}
