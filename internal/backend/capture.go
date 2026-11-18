package backend

import (
	"context"
	"errors"

	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/recording"
)

func (s *Service) CheckSavedPDHost(ctx context.Context) (bool, error) {
	s.mu.Lock()
	checker := s.pdHostChecker
	connection := s.snapshot.Connections
	if checker == nil || s.snapshot.State != Streaming || s.checkingPDHost {
		s.mu.Unlock()
		return false, errors.New("PD host settings check unavailable")
	}
	s.checkingPDHost = true
	s.mu.Unlock()

	enabled, err := checker.CheckSavedPDHost(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkingPDHost = false
	if s.pdHostChecker == nil || s.snapshot.Connections != connection {
		return false, errors.New("meter disconnected during PD host settings check")
	}
	if err == nil {
		s.snapshot.SavedPDHost = &enabled
	} else {
		s.snapshot.SavedPDHost = nil
	}
	s.notifyLocked()
	return enabled, err
}

type CaptureRecording struct {
	Active  bool   `json:"active"`
	Path    string `json:"path,omitempty"`
	Packets uint64 `json:"packets"`
}

type PacketSnapshot struct {
	Packets []pd.Packet `json:"packets"`
	Total   uint64      `json:"total"`
	Invalid uint64      `json:"invalid"`
	Evicted uint64      `json:"evicted"`
	// Oldest is the first retained sequence; a consumer can detect a gap after
	// its last sequence. Clearing the list never reuses sequence numbers.
	Oldest    uint64           `json:"oldest_sequence"`
	Recording CaptureRecording `json:"recording"`
}

// PacketSnapshot returns owned packets after a sequence number, oldest first.
// limit=0 omits packets; a negative limit includes all bounded history.
func (s *Service) PacketSnapshot(after uint64, limit int) PacketSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := PacketSnapshot{Total: s.packetSequence, Invalid: s.invalidPackets, Evicted: s.evictedPackets, Recording: s.captureState}
	if s.capture != nil {
		v.Recording.Packets = s.capture.Packets
	}
	if s.packetCount == 0 {
		return v
	}
	v.Oldest = s.packetRing[s.packetHead].Sequence
	if limit == 0 || after >= s.packetSequence {
		return v
	}
	first := max(v.Oldest, after+1)
	count := int(s.packetSequence - first + 1)
	if limit > 0 {
		count = min(count, limit)
	}
	start := s.packetHead + int(first-v.Oldest)
	v.Packets = make([]pd.Packet, count)
	for index := range v.Packets {
		v.Packets[index] = pd.Clone(s.packetRing[(start+index)%len(s.packetRing)])
	}
	return v
}

func (s *Service) ClearPackets() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.packetRing)
	s.packetHead, s.packetCount = 0, 0
	s.notifyLocked()
}

func (s *Service) acceptPacket(packet pd.Packet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	packet = pd.Clone(packet)
	s.packetSequence++
	packet.Sequence, packet.Segment = s.packetSequence, s.segment
	s.pdAnalyzer.Observe(&packet)
	if packet.Error != "" {
		s.invalidPackets++
	} else {
		s.snapshot.State = Streaming
	}
	if s.packetCount < len(s.packetRing) {
		s.packetRing[(s.packetHead+s.packetCount)%len(s.packetRing)] = packet
		s.packetCount++
	} else {
		s.packetRing[s.packetHead] = packet
		s.packetHead = (s.packetHead + 1) % len(s.packetRing)
		s.evictedPackets++
	}
	if s.capture != nil {
		if err := s.capture.Write(packet); err != nil {
			return &recordingError{err}
		}
	}
	s.notifyLocked()
	return nil
}

func (s *Service) StartCapture(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return errors.New("acquisition has finished; create a new session before capturing")
	}
	if s.capture != nil {
		return errors.New("PD capture already active")
	}
	c, err := recording.CreateCapture(path)
	if err != nil {
		return err
	}
	s.capture = c
	s.captureState = CaptureRecording{Active: true, Path: c.Path}
	s.notifyLocked()
	return nil
}

func (s *Service) stopCaptureLocked() error {
	if s.capture == nil {
		return nil
	}
	s.captureState.Packets = s.capture.Packets
	err := s.capture.Close()
	s.capture = nil
	s.captureState.Active = false
	if err != nil {
		s.snapshot.Error, s.snapshot.ErrorKind = err.Error(), "recording"
	}
	s.notifyLocked()
	return err
}

func (s *Service) StopCapture() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopCaptureLocked()
}
