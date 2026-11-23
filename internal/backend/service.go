// Package backend owns device acquisition, reconnection, statistics and recording.
// Both the terminal and MyGo frontends consume this Go API.
package backend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/recording"
)

type Options struct {
	Open device.OpenFunc
	// Rate is a host-side sampling ceiling, 0 means all reports (max 1000).
	Rate              int
	HistorySize       int
	PacketHistorySize int
	Reconnect         bool
	ReconnectDelay    time.Duration
	StaleAfter        time.Duration
}

type Service struct {
	opts                                           Options
	mu                                             sync.RWMutex
	running                                        bool
	finished                                       bool
	snapshot                                       Snapshot
	ring                                           []device.Sample
	head                                           int
	historyBlocks                                  []sampleBlockSummary
	previous                                       *device.Sample
	segment                                        uint64
	recorder                                       *recording.CSV
	subscribers                                    map[chan struct{}]struct{}
	packetRing                                     []pd.Packet
	packetHead, packetCount                        int
	packetSequence, invalidPackets, evictedPackets uint64
	pdAnalyzer                                     pd.Analyzer
	capture                                        *recording.Capture
	captureState                                   CaptureRecording
	pdHostChecker                                  device.PDHostChecker
	checkingPDHost                                 bool
}

// UIRefreshDigest contains the service state that can affect a dashboard redraw.
type UIRefreshDigest struct {
	Connection         ConnectionStatus
	Streaming          bool
	HasLatest          bool
	CSVRecordingActive bool
	PDRecordingActive  bool
	PDOldest           uint64
	PDTotal            uint64
	CanCheckPDHost     bool
	SavedPDHostSet     bool
	SavedPDHost        bool
}

// UIRefreshDigest returns redraw-relevant state under one read lock.
func (s *Service) UIRefreshDigest() UIRefreshDigest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	digest := UIRefreshDigest{
		Connection: ConnectionStatus{
			State:       s.snapshot.State,
			DeviceID:    s.snapshot.Device.ID,
			Connections: s.snapshot.Connections,
			Error:       s.snapshot.Error,
			ErrorKind:   s.snapshot.ErrorKind,
		},
		Streaming:          s.snapshot.State == Streaming,
		HasLatest:          s.snapshot.Latest != nil,
		CSVRecordingActive: s.snapshot.Recording.Active,
		PDRecordingActive:  s.captureState.Active,
		PDTotal:            s.packetSequence,
		CanCheckPDHost:     s.snapshot.State == Streaming && s.pdHostChecker != nil,
	}
	if s.packetCount > 0 {
		digest.PDOldest = s.packetRing[s.packetHead].Sequence
	}
	if s.snapshot.SavedPDHost != nil {
		digest.SavedPDHostSet = true
		digest.SavedPDHost = *s.snapshot.SavedPDHost
	}
	return digest
}

func New(opts Options) (*Service, error) {
	if opts.Open == nil {
		return nil, errors.New("device opener is required")
	}
	if opts.Rate < 0 || opts.Rate > 1000 {
		return nil, errors.New("rate must be between 0 (all) and 1000")
	}
	if opts.HistorySize == 0 {
		opts.HistorySize = 18000
	}
	if opts.HistorySize > 1000000 {
		return nil, errors.New("history size must be at most 1000000 (or negative for full session)")
	}
	if opts.PacketHistorySize == 0 {
		opts.PacketHistorySize = 2000
	}
	if opts.PacketHistorySize < 1 || opts.PacketHistorySize > 100000 {
		return nil, errors.New("PD history size must be between 1 and 100000")
	}
	if opts.ReconnectDelay == 0 {
		opts.ReconnectDelay = time.Second
	}
	if opts.StaleAfter == 0 {
		opts.StaleAfter = 3 * time.Second
	}
	if opts.ReconnectDelay < 0 || opts.StaleAfter < 100*time.Millisecond {
		return nil, errors.New("invalid reconnect or stale timeout")
	}
	ringCap := 1024
	if opts.HistorySize > 0 && opts.HistorySize < ringCap {
		ringCap = opts.HistorySize
	}
	return &Service{opts: opts, snapshot: Snapshot{State: Stopped}, ring: make([]device.Sample, 0, ringCap), packetRing: make([]pd.Packet, opts.PacketHistorySize), subscribers: make(map[chan struct{}]struct{})}, nil
}

func (s *Service) status(state State, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.State = state
	s.snapshot.Error = ""
	s.snapshot.ErrorKind = ""
	if err != nil {
		s.snapshot.Error = err.Error()
		s.snapshot.ErrorKind = errorKind(err)
	}
	s.notifyLocked()
}

// Run blocks until canceled or a terminal error. A Service represents one
// session and Run can be called once. UI mutations and snapshots are safe.
func (s *Service) Run(ctx context.Context) (result error) {
	s.mu.Lock()
	if s.running || s.finished {
		s.mu.Unlock()
		return errors.New("acquisition already started; create a new session to run again")
	}
	s.running = true
	s.mu.Unlock()
	logger := diagnostics.Logger(ctx)
	logger.InfoContext(ctx, diagnostics.AcquisitionStarted, "reconnect", s.opts.Reconnect, "read_timeout", s.opts.StaleAfter)
	defer func() {
		s.mu.Lock()
		storageErr := errors.Join(s.stopRecordingLocked(), s.stopCaptureLocked())
		if storageErr != nil {
			storageErr = &recordingError{storageErr}
		}
		result = errors.Join(result, storageErr)
		s.running = false
		s.finished = true
		s.snapshot.State = Stopped
		if result != nil {
			s.snapshot.Error = result.Error()
			s.snapshot.ErrorKind = errorKind(result)
		}
		s.notifyLocked()
		s.mu.Unlock()
		if result != nil {
			logger.WarnContext(ctx, diagnostics.AcquisitionStopped, "kind", errorKind(result), "error", result)
		} else {
			logger.InfoContext(ctx, diagnostics.AcquisitionStopped)
		}
	}()
	started := time.Now()
	s.status(Connecting, nil)
	type connectionFailure struct{ stage, driver, path, reason string }
	var lastFailure connectionFailure
	for attempt := 1; ctx.Err() == nil; attempt++ {
		logger.DebugContext(ctx, diagnostics.ConnectionAttempt, "attempt", attempt)
		info, reader, err := s.opts.Open(ctx)
		if info.ID != "" {
			s.mu.Lock()
			s.snapshot.Device = info
			s.mu.Unlock()
		}
		var closeErr error
		if err == nil && reader == nil {
			err = errors.New("device opener returned a nil reader")
		}
		opened := err == nil
		if opened {
			lastFailure = connectionFailure{}
			meterLog := logger.With("driver", info.Driver, "transport", info.Transport, "path", info.Path, "attempt", attempt)
			meterLog.InfoContext(ctx, diagnostics.MeterOpened)
			s.mu.Lock()
			s.snapshot.Device = info
			s.snapshot.Connections++
			s.pdHostChecker, _ = reader.(device.PDHostChecker)
			s.segment++
			s.pdAnalyzer.Reset()
			s.previous = nil
			s.mu.Unlock()
			s.status(Waiting, nil)
			err = s.acquire(ctx, reader, started, info.Simulated, meterLog)
			s.mu.Lock()
			s.pdHostChecker = nil
			s.snapshot.SavedPDHost = nil
			s.mu.Unlock()
			closeErr = reader.Close()
		} else if reader != nil {
			closeErr = reader.Close()
		}
		if ctx.Err() != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				err = nil
			}
			return errors.Join(err, closeErr)
		}
		err = errors.Join(err, closeErr)
		if err == nil {
			return nil
		}
		var storageErr *recordingError
		retry := s.opts.Reconnect && !errors.As(err, &storageErr) && !errors.Is(err, device.ErrAccessDeclined)
		stage := "discover"
		if opened {
			stage = "read"
		} else if info.ID != "" {
			stage = "open"
		}
		failure := connectionFailure{stage: stage, driver: info.Driver, path: info.Path, reason: err.Error()}
		if failure != lastFailure {
			logger.WarnContext(ctx, diagnostics.ConnectionFailed, "stage", stage, "driver", info.Driver,
				"transport", info.Transport, "path", info.Path, "attempt", attempt, "retry", retry,
				"kind", errorKind(err), "error", err)
			lastFailure = failure
		}
		if !retry {
			return err
		}
		s.status(Waiting, err)
		logger.DebugContext(ctx, diagnostics.RetryScheduled, "attempt", attempt, "retry_delay", s.opts.ReconnectDelay)
		timer := time.NewTimer(s.opts.ReconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

type Recording struct {
	Active  bool   `json:"active"`
	Path    string `json:"path,omitempty"`
	Samples uint64 `json:"samples"`
}

func (s *Service) StartRecording(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return errors.New("acquisition has finished; create a new session before recording")
	}
	if s.recorder != nil {
		return errors.New("recording already active")
	}
	r, err := recording.Create(path)
	if err != nil {
		return err
	}
	s.recorder = r
	s.snapshot.Recording = Recording{Active: true, Path: r.Path}
	s.notifyLocked()
	return nil
}

func (s *Service) stopRecordingLocked() error {
	if s.recorder == nil {
		return nil
	}
	s.snapshot.Recording.Samples = s.recorder.Samples
	err := s.recorder.Close()
	s.recorder = nil
	s.snapshot.Recording.Active = false
	if err != nil {
		s.snapshot.Error = "finish recording: " + err.Error()
		s.snapshot.ErrorKind = "recording"
	}
	s.notifyLocked()
	return err
}

func (s *Service) StopRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopRecordingLocked()
}
