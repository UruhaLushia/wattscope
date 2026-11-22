package app

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/preferences"
	"github.com/egoist/mygo/ui"
)

func chartFrameFixture() *dashboard {
	history := make([]device.Sample, 4800)
	for index := range history {
		elapsed := float64(index) / 20
		value := 0.4 + math.Sin(elapsed)*0.8
		history[index] = device.Sample{
			Elapsed: elapsed, Segment: uint64(index / 1800), Voltage: 9 + math.Cos(elapsed*3)*0.2,
			Current: value, Power: value * 9,
		}
	}
	d := &dashboard{language: "en", channel: 0, chartSeries: 7, fullSession: true}
	d.settings = guiSettings{Theme: mygo.ThemeLight, Navigation: navigationSidebar, Accent: kit.DefaultAccentHex}
	d.held = &backend.Snapshot{State: backend.Streaming, Latest: &history[len(history)-1], History: history}
	return d
}

func BenchmarkOverviewFrames(b *testing.B) {
	d := chartFrameFixture()
	tester := ui.NewTester(d.view, 1120, 720)
	tester.Advance(time.Second)
	b.ReportAllocs()
	for b.Loop() {
		tester.Frame()
	}
}

func BenchmarkPDSearchFrames(b *testing.B) {
	packets := make([]pd.Packet, 1200)
	for index := range packets {
		packets[index] = pd.Packet{Sequence: uint64(index + 1), Type: "GoodCRC", SOP: "SOP", MessageHex: "4101"}
		if index%8 == 0 {
			packets[index].Type = "Source_Capabilities"
			packets[index].PDOs = []pd.PDO{{Position: 1, Kind: "Fixed", MinV: 5, MaxV: 5, MaxA: 3, MaxW: 15}}
		}
	}
	d := &dashboard{language: "en", page: pagePD}
	d.settings = guiSettings{Theme: mygo.ThemeLight, Navigation: navigationSidebar, Accent: kit.DefaultAccentHex}
	d.packets = packetView{search: "fixed", offline: &backend.PacketSnapshot{Packets: packets, Total: uint64(len(packets))}}
	tester := ui.NewTester(d.view, 1120, 720)
	tester.Advance(time.Second)
	b.ReportAllocs()
	for b.Loop() {
		tester.Frame()
	}
}

func BenchmarkConnectionLogBuffer(b *testing.B) {
	buffer := &diagnostics.Buffer{}
	logger := slog.New(buffer.Handler())
	for range 400 {
		logger.Info(diagnostics.AppStarted, "driver", "test", "transport", "USB")
		logger.Debug(diagnostics.DriverScanFinished, "driver", "test", "found", 1)
	}
	for _, minimum := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		b.Run(minimum.String(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				buffer.LatestFirst(minimum)
			}
		})
	}
}

func newTestDashboard(t *testing.T, mobile bool) (*dashboard, func()) {
	t.Helper()
	service, err := session.New(session.Options{
		Source:            session.Source{Demo: true},
		Rate:              dashboardRate,
		HistorySize:       -1,
		PacketHistorySize: packetLimit,
	})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	return startTestDashboard(t, service, mobile, 1)
}

func startTestDashboard(t *testing.T, service *backend.Service, mobile bool, minSamples int) (*dashboard, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	prefsKey := "test-" + strings.NewReplacer("/", "-", " ", "_").Replace(t.Name())
	t.Cleanup(func() { _ = preferences.Delete(prefsKey) })
	d := &dashboard{
		ctx:         ctx,
		cancel:      cancel,
		done:        done,
		service:     service,
		language:    "en",
		mobile:      mobile,
		settingsKey: prefsKey,
		settings: guiSettings{
			Theme:       mygo.ThemeLight,
			Accent:      kit.DefaultAccentHex,
			Language:    "en",
			Navigation:  navigationHorizontal,
			SmoothChart: true,
		},
	}
	if !mobile {
		d.settings.Navigation = navigationSidebar
	}
	updates, unsubscribe := service.Subscribe()
	defer unsubscribe()
	go func() {
		defer close(done)
		_ = service.Run(ctx)
	}()
	cleanup := func() {
		cancel()
		<-done
		d.work.Wait()
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-updates:
			if service.Snapshot(0).HistoryCount >= minSamples {
				return d, cleanup
			}
		case <-done:
			cleanup()
			t.Fatalf("acquisition stopped during setup: %s", service.Snapshot(0).Error)
		case <-timer.C:
			cleanup()
			t.Fatalf("expected %d samples during setup, got %d", minSamples, service.Snapshot(0).HistoryCount)
		}
	}
}

func newTestDashboardWithSamples(t *testing.T, samples []device.Sample) (*dashboard, func()) {
	t.Helper()
	conn := &sliceConn{samples: samples}
	svc, err := backend.New(backend.Options{
		Open: func(context.Context) (device.Info, device.Connection, error) {
			return device.Info{Name: "POWER-Z KM003C", Driver: "powerz-km003c"}, conn, nil
		},
		Rate:              0,
		HistorySize:       -1,
		PacketHistorySize: 64,
		Reconnect:         false,
		StaleAfter:        time.Minute,
	})
	if err != nil {
		t.Fatalf("backend.New: %v", err)
	}
	return startTestDashboard(t, svc, false, len(samples))
}

type sliceConn struct {
	samples []device.Sample
}

func (c *sliceConn) Read(ctx context.Context) (device.Sample, error) {
	if len(c.samples) > 0 {
		sample := c.samples[0]
		c.samples = c.samples[1:]
		return sample, nil
	}
	<-ctx.Done()
	return device.Sample{}, ctx.Err()
}

func (c *sliceConn) Close() error { return nil }

func TestAccentContrastAllPresets(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, hex := range kit.AccentPresets {
			_, p := kit.ResolveTheme(dark, hex)
			if lc := math.Abs(kit.ApcaLc(p.AccentText, p.Surface)); lc < 50 {
				t.Fatalf("dark=%v accent=%s: AccentText APCA Lc=%.1f < 50", dark, hex, lc)
			}
			if lc := math.Abs(kit.ApcaLc(p.AccentInk, p.Accent)); lc < 30 {
				t.Fatalf("dark=%v accent=%s: AccentInk APCA Lc=%.1f < 30", dark, hex, lc)
			}
		}
	}
}

func TestDurationFormatting(t *testing.T) {
	for _, tc := range []struct {
		seconds float64
		want    string
	}{
		{-2, "0s"}, {0, "0s"}, {45, "45s"}, {65, "1m5s"}, {8178, "2h16m18s"}, {30708, "8h31m48s"},
	} {
		if got := formatDuration(tc.seconds); got != tc.want {
			t.Fatalf("formatDuration(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
	for _, tc := range []struct {
		seconds          float64
		wantVal, wantUnt string
	}{
		{12.4, "12.4", "s"}, {59.9, "59.9", "s"}, {60, "1m0s", ""}, {8178, "2h16m18s", ""},
	} {
		if gotVal, gotUnit := formatSessionDuration(tc.seconds); gotVal != tc.wantVal || gotUnit != tc.wantUnt {
			t.Fatalf("formatSessionDuration(%v) = (%q, %q), want (%q, %q)", tc.seconds, gotVal, gotUnit, tc.wantVal, tc.wantUnt)
		}
	}
}

func TestConnectionLogRetention(t *testing.T) {
	buffer := &diagnostics.Buffer{}
	logger := slog.New(buffer.Handler())
	levels := [...]slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn}
	for range 300 {
		for _, lvl := range levels {
			logger.Log(t.Context(), lvl, diagnostics.AppStarted, "driver", "test")
		}
	}
	for _, test := range []struct {
		minimum slog.Level
		count   int
	}{
		{slog.LevelDebug, 200},
		{slog.LevelDebug + 1, 200},
		{slog.LevelInfo, 200},
		{slog.LevelWarn, 100},
		{slog.LevelError, 0},
	} {
		entries := buffer.LatestFirst(test.minimum)
		if len(entries) != test.count {
			t.Fatalf("minimum %s: got %d entries, want %d", test.minimum, len(entries), test.count)
		}
		expected := uint64(900)
		for _, entry := range entries {
			for levels[(expected-1)%uint64(len(levels))] < test.minimum {
				expected--
			}
			if entry.Sequence != expected || entry.Level < test.minimum {
				t.Fatalf("minimum %s: got entry %d (%s), want %d", test.minimum, entry.Sequence, entry.Level, expected)
			}
			expected--
		}
	}
	entries := buffer.LatestFirst(slog.LevelInfo)
	entries[0].Fields[0].Value = "modified"
	if got := buffer.LatestFirst(slog.LevelInfo)[0].Fields[0].Value; got != "test" {
		t.Fatalf("log snapshot shares buffer fields: %q", got)
	}
}

func TestChartHistoryRefreshAfterReset(t *testing.T) {
	d, cleanup := newTestDashboardWithSamples(t, []device.Sample{{Power: 1}, {Elapsed: 2, Power: 2}})
	defer cleanup()
	live := d.service.Snapshot(0)
	history := d.sampledChartHistory(live, chartHistoryQuery{}).History
	if len(history) != 2 {
		t.Fatalf("full history has %d samples", len(history))
	}
	again := d.sampledChartHistory(live, chartHistoryQuery{}).History
	if len(again) != 2 || &again[0] != &history[0] {
		t.Fatal("same samples and window should reuse history")
	}
	zoomed := d.sampledChartHistory(live, chartHistoryQuery{duration: time.Second}).History
	if len(zoomed) != 1 || zoomed[0].Power != 2 {
		t.Fatalf("unexpected zoomed history: %v", zoomed)
	}
	d.service.ResetStatistics()
	if reset := d.sampledChartHistory(d.service.Snapshot(0), chartHistoryQuery{}); len(reset.History) != 0 {
		t.Fatal("reset retained cached history")
	}
}

func TestReadoutThrottleCadence(t *testing.T) {
	d := &dashboard{}
	latest := device.Sample{Elapsed: 0.05, Power: 10.0}
	snap := backend.Snapshot{State: backend.Streaming, Latest: &latest}

	var shown []float64
	tt := ui.NewTesterWith(func(ctx *ui.Context) {
		sample, _, _ := d.throttledReadout(ctx, snap, snap)
		if sample != nil {
			shown = append(shown, sample.Power)
		}
	}, 200, 100, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})

	latest = device.Sample{Elapsed: 0.10, Power: 11.0}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	latest = device.Sample{Elapsed: 0.15, Power: 12.0}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if got := shown[len(shown)-1]; got != 10.0 {
		t.Fatalf("expected throttled readout to hold 10.0 within 250ms window, got %v", got)
	}
	tt.Advance(200 * time.Millisecond)
	if got := shown[len(shown)-1]; got != 12.0 {
		t.Fatalf("expected readout to refresh to 12.0 after 250ms, got %v", got)
	}
}

func TestTrendAxisStartsFromZero(t *testing.T) {
	pos := trendAxis(16.2, 17.8)
	if pos.Lo != 0 || pos.Hi < 17.8 {
		t.Fatalf("expected positive axis [0, >=17.8], got [%v, %v]", pos.Lo, pos.Hi)
	}
	if ticks := pos.Ticks(3); len(ticks) < 2 || ticks[0].Value != 0 {
		t.Fatalf("expected first tick at 0, got %+v", ticks)
	}
	if idle := trendAxis(0, 0); idle.Lo != 0 || idle.Hi <= 0 {
		t.Fatalf("expected zero idle axis [0, >0], got [%v, %v]", idle.Lo, idle.Hi)
	}
	if neg := trendAxis(-2.4, -1.8); neg.Hi != 0 || neg.Lo >= -2.4 {
		t.Fatalf("expected negative axis with 0 top, got [%v, %v]", neg.Lo, neg.Hi)
	}
	if mixed := trendAxis(-1.2, 2.5); mixed.Lo >= -1.2 || mixed.Hi <= 2.5 {
		t.Fatalf("expected bidirectional axis to cover [-1.2, 2.5], got [%v, %v]", mixed.Lo, mixed.Hi)
	}
}

func TestPDFilterRefresh(t *testing.T) {
	packets := []pd.Packet{{Sequence: 1, Header: 0x0141, Type: "GoodCRC"}, {Sequence: 2, Type: "Source_Capabilities"}}
	view := packetView{}
	msgs := locales.In("en")
	if got := view.visiblePackets(msgs, packets); len(got) != 2 {
		t.Fatalf("default filter has %d packets", len(got))
	}
	view.hideGoodCRC = true
	if got := view.visiblePackets(msgs, packets); len(got) != 1 || got[0].Sequence != 2 {
		t.Fatalf("unexpected packet filter result: %v", got)
	}
	view.search = "nonexistent"
	if got := view.visiblePackets(msgs, packets); len(got) != 0 {
		t.Fatalf("search still has %d packets", len(got))
	}
	view.search = ""
	if got := view.visiblePackets(msgs, []pd.Packet{{Sequence: 3, Type: "Request"}, {Sequence: 4, Header: 0x0141, Type: "GoodCRC"}}); len(got) != 1 || got[0].Sequence != 3 {
		t.Fatalf("unexpected result after replacing packets: %v", got)
	}
	if got := view.visiblePackets(msgs, nil); len(got) != 0 {
		t.Fatal("clear retained cached packets")
	}
}

func TestFindPacketBySequenceAndSummaryCache(t *testing.T) {
	pkts := []pd.Packet{
		{Sequence: 2, Type: "GoodCRC"},
		{
			Sequence: 5, Type: "Source_Capabilities", SOP: "SOP", PowerRole: "Source", Revision: "3.0",
			PDOs: []pd.PDO{{Position: 1, Kind: "Fixed", MinV: 5, MaxV: 5, MaxA: 3, MaxW: 15}},
		},
		{
			Sequence: 9, Type: "Request",
			Request: &pd.Request{Position: 1, Kind: "Fixed", VoltageV: 5, OperatingA: 3, MaximumA: 3, Matched: true},
		},
	}

	if idx := findPacketBySequence(nil, 5); idx != -1 {
		t.Fatalf("expected -1 for empty slice, got %d", idx)
	}
	for wantIdx, pkt := range pkts {
		if idx := findPacketBySequence(pkts, pkt.Sequence); idx != wantIdx {
			t.Fatalf("seq %d: expected index %d, got %d", pkt.Sequence, wantIdx, idx)
		}
	}
	for _, missing := range []uint64{0, 1, 3, 7, 10} {
		if idx := findPacketBySequence(pkts, missing); idx != -1 {
			t.Fatalf("missing seq %d: expected -1, got %d", missing, idx)
		}
	}

	var view packetView
	s1, c1 := view.summaryState(pkts)
	if s1.caps == nil || s1.contract == nil || c1 == "" {
		t.Fatalf("expected populated summary and caps text, got %+v %q", s1, c1)
	}
	s2, c2 := view.summaryState(pkts)
	if s2.caps != s1.caps || s2.contract != s1.contract || c2 != c1 {
		t.Fatal("expected summaryState cache hit to reuse cached pointers/text")
	}

	// Switching to offline source with identical sequence numbers must invalidate cache.
	view.offline = &backend.PacketSnapshot{Packets: pkts}
	view.path = "offline-a.pd"
	s3, _ := view.summaryState(pkts)
	if s3.caps == s1.caps {
		t.Fatal("expected source switch to invalidate summaryState cache")
	}

	// Empty slice clears summary.
	sEmpty, cEmpty := view.summaryState(nil)
	if sEmpty.caps != nil || sEmpty.contract != nil || cEmpty != "" {
		t.Fatalf("expected empty slice summaryState to reset, got %+v %q", sEmpty, cEmpty)
	}
}

func TestLayoutAndNavigationMetrics(t *testing.T) {
	if got := navigationPlacement(true, false, navigationSidebar); got != navigationBottom {
		t.Fatalf("portrait mobile nav = %v, want navigationBottom", got)
	}
	if got := navigationPlacement(true, true, navigationSidebar); got != navigationSide {
		t.Fatalf("landscape sidebar nav = %v, want navigationSide", got)
	}
	if got := navigationPlacement(false, false, navigationHorizontal); got != navigationTop {
		t.Fatalf("desktop horizontal nav = %v, want navigationTop", got)
	}

	if got := sidebarWidth(fullSidebarMinWidth - 1); got != compactSidebarWidth {
		t.Fatalf("narrow sidebarWidth = %v, want %v", got, compactSidebarWidth)
	}
	if got := sidebarWidth(fullSidebarMinWidth); got != navigationSidebarWidth {
		t.Fatalf("full sidebarWidth = %v, want %v", got, navigationSidebarWidth)
	}

	if got := kit.CompactSidebarSideMargin(412); got != kit.CompactSidebarMargin {
		t.Fatalf("compact side margin = %v, want %v", got, kit.CompactSidebarMargin)
	}
	if got := kit.CompactSidebarSideMargin(720); got != kit.DesktopSidebarMargin {
		t.Fatalf("desktop side margin = %v, want %v", got, kit.DesktopSidebarMargin)
	}
	if got := kit.DesktopSidebarRailWidth(true, 412); got != kit.DesktopCompactLandscapeRailW {
		t.Fatalf("compact landscape rail = %v, want %v", got, kit.DesktopCompactLandscapeRailW)
	}
	if got := kit.DesktopSidebarRailWidth(true, 720); got != kit.DesktopCompactRailWidth {
		t.Fatalf("compact desktop rail = %v, want %v", got, kit.DesktopCompactRailWidth)
	}
	if got := kit.DesktopSidebarRailWidth(false, 412); got != kit.DesktopSidebarWidth {
		t.Fatalf("full rail = %v, want %v", got, kit.DesktopSidebarWidth)
	}

	if got := kit.BottomNavReserveFor(34, false, kit.BottomNavCapsule); got != 0 {
		t.Fatalf("hidden bottom nav reserve = %v, want 0", got)
	}
	if got := kit.BottomNavReserveFor(34, true, kit.BottomNavCapsule); got != 34+10+kit.BottomNavHeight {
		t.Fatalf("capsule bottom nav reserve = %v, want %v", got, 34+10+kit.BottomNavHeight)
	}
	if got := kit.BottomNavReserveFor(0, true, kit.BottomNavStandard); got != kit.BottomNavDockMargin+kit.BottomNavStandardHeight {
		t.Fatalf("standard bottom nav reserve = %v, want %v", got, kit.BottomNavDockMargin+kit.BottomNavStandardHeight)
	}

	start, end := horizontalContentPadding(true, false, navigationBottom, 16, 44, 12)
	if start != 60 || end != 28 {
		t.Fatalf("portrait horizontal padding = (%v, %v), want (60, 28)", start, end)
	}
	start, end = horizontalContentPadding(false, true, navigationSide, 16, 44, 12)
	if start != 16 || end != 28 {
		t.Fatalf("landscape sidebar horizontal padding = (%v, %v), want (16, 28)", start, end)
	}
	start, end = horizontalContentPadding(false, true, navigationTop, 16, 44, 12)
	if start != 60 || end != 28 {
		t.Fatalf("landscape top-bar horizontal padding = (%v, %v), want (60, 28)", start, end)
	}
	start, end = horizontalContentPadding(false, false, navigationTop, 16, 44, 12)
	if start != 16 || end != 16 {
		t.Fatalf("desktop horizontal padding = (%v, %v), want (16, 16)", start, end)
	}
}
