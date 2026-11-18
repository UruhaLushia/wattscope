package backend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

type readdressedMeter struct {
	discoveries [][]device.Info
	opened      []string
}

func (*readdressedMeter) ID() string   { return "test-usb-meter" }
func (*readdressedMeter) Name() string { return "Test USB meter" }

func (meter *readdressedMeter) Discover(context.Context) ([]device.Info, error) {
	if len(meter.discoveries) == 0 {
		return nil, nil
	}
	meters := meter.discoveries[0]
	meter.discoveries = meter.discoveries[1:]
	return meters, nil
}

func (meter *readdressedMeter) Open(_ context.Context, info device.Info) (device.Connection, error) {
	meter.opened = append(meter.opened, info.Path)
	return &meterConnection{first: true, disconnect: len(meter.opened) == 1}, nil
}

type meterConnection struct {
	first, disconnect bool
}

func (conn *meterConnection) Read(ctx context.Context) (device.Sample, error) {
	if conn.first {
		conn.first = false
		return device.Sample{Power: 5}, nil
	}
	if conn.disconnect {
		return device.Sample{}, device.ErrNoDevice
	}
	<-ctx.Done()
	return device.Sample{}, ctx.Err()
}

func (*meterConnection) Close() error { return nil }

func TestServiceReconnectsAfterUSBAddressChange(t *testing.T) {
	meter := &readdressedMeter{}
	deviceAt := func(path string) device.Info {
		return device.Info{ID: meter.ID() + ":" + path, Driver: meter.ID(), Path: path, PathDependentID: true}
	}
	initial, peer := deviceAt("/dev/bus/usb/001/002"), deviceAt("/dev/bus/usb/001/004")
	reconnected, ambiguous := deviceAt("/dev/bus/usb/001/003"), deviceAt("/dev/bus/usb/001/005")
	meter.discoveries = [][]device.Info{{initial, peer}, {peer}, {reconnected, peer, ambiguous}, {reconnected, peer}}
	registry, err := device.NewRegistry(meter)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Options{Open: registry.Opener("", initial.ID), Reconnect: true, ReconnectDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.StartRecording(filepath.Join(t.TempDir(), "reconnect.csv")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
		if !slices.Equal(meter.opened, []string{initial.Path, reconnected.Path}) {
			t.Errorf("opened USB paths = %v", meter.opened)
		}
	})
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("did not reconnect: %+v", service.Snapshot(0))
		case <-updates:
			snap := service.Snapshot(-1)
			if snap.Device.ID == reconnected.ID && snap.State == Streaming {
				if snap.Connections != 2 || len(snap.History) != 2 || snap.History[0].Segment == snap.History[1].Segment ||
					!snap.Recording.Active || snap.Recording.Samples != 2 {
					t.Fatalf("expected one continuous recording with a segment break: %+v", snap)
				}
				return
			}
		}
	}
}

func TestServiceDoesNotRetryDeclinedAccess(t *testing.T) {
	attempts := 0
	service, err := New(Options{
		Open: func(context.Context) (device.Info, device.Connection, error) {
			attempts++
			return device.Info{}, nil, device.ErrAccessDeclined
		},
		Reconnect: true, ReconnectDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	runErr := service.Run(ctx)
	snap := service.Snapshot(0)
	if !errors.Is(runErr, os.ErrPermission) || attempts != 1 || snap.State != Stopped || snap.ErrorKind != "permission" {
		t.Fatalf("expected permission state after a single attempt, got %v, %d attempts, %+v", runErr, attempts, snap)
	}
}

func historyService(tb testing.TB, size int) *Service {
	tb.Helper()
	service, err := New(Options{
		Open: func(context.Context) (device.Info, device.Connection, error) {
			return device.Info{}, nil, context.Canceled
		},
		HistorySize:       size,
		PacketHistorySize: 1,
	})
	if err != nil {
		tb.Fatal(err)
	}
	return service
}

func TestServiceFullSessionHistoryAndSampledWindow(t *testing.T) {
	svc := historyService(t, -1)
	const total = 10000 // 500s at 20Hz
	groups := [...]uint16{0, 1}
	for index := range total {
		power := 10.0
		group := &groups[0]
		if index >= 5000 {
			group = &groups[1]
		}
		switch index {
		case 4321:
			power = 99.5
		case 7654:
			power = 0.25
		}
		if err := svc.accept(device.Sample{
			Elapsed: float64(index) / 20,
			Voltage: 20.0,
			Current: power / 20.0,
			Power:   power,
			Group:   group,
		}); err != nil {
			t.Fatalf("accept(%d): %v", index, err)
		}
	}

	meta := svc.Snapshot(0)
	if meta.HistoryCount != total {
		t.Fatalf("expected HistoryCount=%d, got %d", total, meta.HistoryCount)
	}
	if meta.HistoryStart != 0 || meta.HistoryEnd < 499.9 {
		t.Fatalf("unexpected history bounds [%v, %v]", meta.HistoryStart, meta.HistoryEnd)
	}

	// Full-session sampled view stays bounded while preserving endpoints, peak, trough, and segment break.
	sampled := svc.SnapshotWindowSampled(0, 1000)
	if len(sampled.History) > 1004 || len(sampled.History) < 500 {
		t.Fatalf("unexpected sampled length %d", len(sampled.History))
	}
	if sampled.History[0].Elapsed != 0 || sampled.History[len(sampled.History)-1].Elapsed != meta.HistoryEnd {
		t.Fatalf("endpoints not preserved: first=%v last=%v want=[0,%v]", sampled.History[0].Elapsed, sampled.History[len(sampled.History)-1].Elapsed, meta.HistoryEnd)
	}
	var sawPeak, sawDip, sawTransition bool
	for index, sample := range sampled.History {
		if index > 0 && sample.Elapsed <= sampled.History[index-1].Elapsed {
			t.Fatalf("sampled history not strictly ordered at %d: %v <= %v", index, sample.Elapsed, sampled.History[index-1].Elapsed)
		}
		sawPeak = sawPeak || sample.Power == 99.5
		sawDip = sawDip || sample.Power == 0.25
		sawTransition = sawTransition || (sample.Elapsed == 250 && sample.Segment == 1)
	}
	if !sawPeak || !sawDip || !sawTransition {
		t.Fatalf("sampled view preserved peak=%v dip=%v transition=%v", sawPeak, sawDip, sawTransition)
	}

	// Windowed query smaller than maxPoints returns exact samples.
	short := svc.SnapshotWindowSampled(10*time.Second, 2400)
	if len(short.History) != 201 {
		t.Fatalf("expected 201 exact samples in 10s window, got %d", len(short.History))
	}
	if short.HistoryStart != 0 || short.HistoryEnd != meta.HistoryEnd {
		t.Fatalf("expected full-session metadata on windowed snapshot, got [%v, %v]", short.HistoryStart, short.HistoryEnd)
	}

	// Range query returns samples covering [start, end] with decimation and exact ClosestSample lookup.
	ranged := svc.SnapshotRangeSampled(100.0, 260.0, 400)
	if len(ranged.History) == 0 || len(ranged.History) > 404 {
		t.Fatalf("unexpected ranged length %d", len(ranged.History))
	}
	if ranged.History[0].Elapsed > 100.0 || ranged.History[len(ranged.History)-1].Elapsed < 260.0 {
		t.Fatalf("ranged slice did not span [100, 260]: got [%v, %v]", ranged.History[0].Elapsed, ranged.History[len(ranged.History)-1].Elapsed)
	}
	closest := svc.ClosestSample(216.05)
	if closest == nil || closest.Power != 99.5 {
		t.Fatalf("expected ClosestSample(216.05) to find peak sample, got %+v", closest)
	}

	svc.ResetStatistics()
	cleared := svc.Snapshot(0)
	if cleared.HistoryCount != 0 || cleared.HistoryEnd != 0 {
		t.Fatalf("expected ResetStatistics to clear history, got count=%d end=%v", cleared.HistoryCount, cleared.HistoryEnd)
	}
}

func TestServiceWrappedHistory(t *testing.T) {
	service := historyService(t, 5)
	for index := range 8 {
		if err := service.accept(device.Sample{Elapsed: float64(index), Power: float64(index)}); err != nil {
			t.Fatal(err)
		}
	}
	want := []float64{3, 4, 5, 6, 7}
	for _, snapshot := range []Snapshot{service.Snapshot(-1), service.SnapshotWindowSampled(0, 5)} {
		elapsed := make([]float64, 0, len(snapshot.History))
		for _, sample := range snapshot.History {
			elapsed = append(elapsed, sample.Elapsed)
		}
		if !slices.Equal(elapsed, want) {
			t.Fatalf("wrapped history = %v, want %v", elapsed, want)
		}
	}
}

func TestServiceSnapshotOwnership(t *testing.T) {
	service := historyService(t, 1)
	temperature := 23.0
	if err := service.accept(device.Sample{Power: 10, Temperature: &temperature}); err != nil {
		t.Fatal(err)
	}
	temperature = 42
	before := service.Snapshot(-1)
	if *before.Latest.Temperature != 23 || *before.History[0].Temperature != 23 {
		t.Fatal("accepted sample shares driver-owned values")
	}
	*before.History[0].Temperature = 41
	if err := service.accept(device.Sample{Power: 20, Temperature: &temperature}); err != nil {
		t.Fatal(err)
	}
	if before.Latest.Power != 10 || *before.Latest.Temperature != 23 {
		t.Fatal("prior snapshot changed after writing another sample")
	}
	after := service.Snapshot(0)
	if after.Latest.Power != 20 || *after.Latest.Temperature != 42 {
		t.Fatalf("latest sample = %+v", after.Latest)
	}
}

func TestSampledHistoryStaysStableAfterWritesAndReset(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
	}{
		{"FullSession", -1},
		{"Bounded", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := historyService(t, test.size)
			for _, sample := range []device.Sample{{Power: 1}, {Elapsed: 1, Power: 2}} {
				if err := service.accept(sample); err != nil {
					t.Fatal(err)
				}
			}
			held := service.SnapshotWindowSampled(0, 0)
			if len(held.History) != 2 || cap(held.History) != 2 {
				t.Fatalf("held history length and capacity = %d and %d", len(held.History), cap(held.History))
			}
			window := service.SnapshotWindowSampled(500*time.Millisecond, 10)
			if len(window.History) != 1 || window.History[0].Power != 2 {
				t.Fatalf("unexpected trailing window: %v", window.History)
			}
			extended := append(held.History, device.Sample{Elapsed: 2, Power: 999})
			if err := service.accept(device.Sample{Elapsed: 2, Power: 3}); err != nil {
				t.Fatal(err)
			}
			if extended[2].Power != 999 {
				t.Fatal("appending to a snapshot affected history storage")
			}
			service.ResetStatistics()
			if err := service.accept(device.Sample{Elapsed: 3, Power: 4}); err != nil {
				t.Fatal(err)
			}
			if held.History[0].Power != 1 || held.History[1].Power != 2 || window.History[0].Power != 2 {
				t.Fatal("history snapshot changed after writes or reset")
			}
		})
	}
}

func BenchmarkServiceAccept(b *testing.B) {
	service := historyService(b, 1024)
	b.ReportAllocs()
	for b.Loop() {
		if err := service.accept(device.Sample{Voltage: 10, Current: 1, Power: 10}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkServiceSnapshotWindow(b *testing.B) {
	service := historyService(b, 100000)
	service.ring = make([]device.Sample, 0, 100000)
	for index := range 150000 {
		if err := service.accept(device.Sample{Elapsed: float64(index) / 20, Power: float64(index % 157)}); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("Sampled", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			service.SnapshotWindowSampled(0, 1000)
		}
	})
	b.Run("Exact", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			service.SnapshotWindowSampled(60*time.Second, 1500)
		}
	})
}

func BenchmarkFullSessionHistoryMemory(b *testing.B) {
	const samplesPerHour = 20 * 60 * 60
	service := historyService(b, -1)
	service.ring = make([]device.Sample, 0, samplesPerHour)
	for index := range samplesPerHour {
		if err := service.accept(device.Sample{Elapsed: float64(index) / 20, Power: 10}); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("LiveWindow", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			service.SnapshotWindowSampled(time.Minute, 2400)
		}
	})
	b.Run("HoldAfterOneHour", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			service.SnapshotWindowSampled(0, 0)
		}
	})
}
