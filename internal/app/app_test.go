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
	"github.com/UruhaLushia/wattscope/internal/pdview"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/preferences"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
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

func TestNormalizeWorkspacePage(t *testing.T) {
	if got := normalizeWorkspacePage(pageMore, true); got != pageMore {
		t.Fatalf("portrait pageMore = %d, want %d", got, pageMore)
	}
	if got := normalizeWorkspacePage(pageMore, false); got != pageOverview {
		t.Fatalf("landscape pageMore = %d, want %d", got, pageOverview)
	}
	for _, page := range []int{pageOverview, pagePD, pageRecords, pageDevice, pageSettings, pageConnectionLog} {
		if got := normalizeWorkspacePage(page, false); got != page {
			t.Fatalf("page %d changed during landscape normalization: %d", page, got)
		}
	}
}

func TestPageScrollsUnderHeader(t *testing.T) {
	noop := func() {}
	noopList := func(float32) {}

	for _, tc := range []struct {
		name           string
		page           workspacePage
		pageID         int
		portraitMobile bool
		want           bool
	}{
		{"landscape fixed overview", workspacePage{fixed: noop}, pageOverview, false, false},
		{"list page", workspacePage{list: noopList}, pagePD, true, false},
		{"desktop device subnav", workspacePage{content: noop}, pageDevice, false, false},
		{"desktop log subnav", workspacePage{content: noop}, pageConnectionLog, false, false},
		{"portrait device page", workspacePage{content: noop}, pageDevice, true, true},
		{"portrait overview", workspacePage{content: noop}, pageOverview, true, true},
		{"desktop settings", workspacePage{content: noop}, pageSettings, false, true},
		{"desktop records", workspacePage{content: noop}, pageRecords, false, true},
	} {
		if got := pageScrollsUnderHeader(tc.page, tc.pageID, tc.portraitMobile); got != tc.want {
			t.Fatalf("%s: pageScrollsUnderHeader = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestChartPlotCanvasKeyAndSecondaryColor(t *testing.T) {
	_, light := kit.ResolveTheme(false, kit.DefaultAccentHex)
	_, dark := kit.ResolveTheme(true, kit.DefaultAccentHex)
	ticks := []chart.Tick{{At: 0, Value: 0}, {At: 1, Value: 10}}

	geomA := chartGeometryKey{rect: ui.Rect{X: 10, Y: 20, W: 300, H: 180}, scale: 2, start: 0, end: 60, primary: 0, series: 7}
	geomMoved := geomA
	geomMoved.rect.X, geomMoved.rect.Y = 40, 120

	base := makeChartPlotCanvasKey(geomA, 600, 360, 2, light, 4, ticks, 0, 7, false)
	moved := makeChartPlotCanvasKey(geomMoved, 600, 360, 2, light, 4, ticks, 0, 7, false)
	if base != moved {
		t.Fatal("origin X/Y translation must not invalidate chartPlotCanvasKey")
	}

	if darkKey := makeChartPlotCanvasKey(geomA, 600, 360, 2, dark, 4, ticks, 0, 7, false); darkKey == base {
		t.Fatal("theme change must invalidate chartPlotCanvasKey")
	}
	if leftAxisKey := makeChartPlotCanvasKey(geomA, 600, 360, 2, light, 4, ticks, 0, 7, true); leftAxisKey == base {
		t.Fatal("hasLeftAxis change must invalidate chartPlotCanvasKey")
	}
	if resized := makeChartPlotCanvasKey(geomA, 602, 360, 2, light, 4, ticks, 0, 7, false); resized == base {
		t.Fatal("pixel dimension change must invalidate chartPlotCanvasKey")
	}

	for _, pal := range []kit.Palette{light, dark} {
		for i, ch := range channels {
			c := secondaryCurveColor(ch, pal, 0.32)
			if c.A != 255 {
				t.Fatalf("channel %d secondaryCurveColor alpha = %d, want 255 for GPU fast path", i, c.A)
			}
		}
	}
}

func TestPDLandscapeLayoutHelpers(t *testing.T) {
	if !pdHeaderShowsSummary(true, navigationSide) {
		t.Fatal("landscape sidebar should render PD summary pill in header")
	}
	if pdHeaderShowsSummary(true, navigationTop) {
		t.Fatal("landscape top-bar must keep PD summary pill out of header")
	}
	if pdHeaderShowsSummary(false, navigationSide) {
		t.Fatal("non-landscape sidebar must not move PD summary pill to header")
	}

	if !pdUsesSplitDetail(true, true, 853) {
		t.Fatal("853dp mobile landscape should use split PD detail")
	}
	if pdUsesSplitDetail(true, true, 679) {
		t.Fatal("679dp mobile landscape should not use split PD detail")
	}
	if pdUsesSplitDetail(true, false, 900) {
		t.Fatal("portrait mobile must not use split PD detail")
	}
	if !pdUsesSplitDetail(false, false, 760) {
		t.Fatal("760dp desktop should use split PD detail")
	}

	_, p := kit.ResolveTheme(false, kit.DefaultAccentHex)
	if got := pdNegotiationDotColor(p, pd.Packet{Type: "PS_RDY"}); got != p.Green {
		t.Fatalf("PS_RDY dot = %v, want Green", got)
	}
	if got := pdNegotiationDotColor(p, pd.Packet{Type: "Wait"}); got != p.Warning {
		t.Fatalf("Wait dot = %v, want Warning", got)
	}
	if got := pdNegotiationDotColor(p, pd.Packet{Type: "Source_Capabilities"}); got != p.Accent {
		t.Fatalf("Source_Capabilities dot = %v, want Accent", got)
	}

	if got := pdCompactCountLabel(74, 74); got != "74" {
		t.Fatalf("pdCompactCountLabel(74, 74) = %q, want 74", got)
	}
	if got := pdCompactCountLabel(37, 74); got != "37/74" {
		t.Fatalf("pdCompactCountLabel(37, 74) = %q, want 37/74", got)
	}

	fields := []pdview.Field{
		{Label: "响应", Value: "18.0 ms"},
		{Label: "就绪", Value: "20.8 ms"},
		{Label: "LongFieldLabel", Value: "This is a much longer value exceeding twenty six runes"},
		{Label: "Empty", Value: ""},
		{Label: "角色", Value: "Source / DFP"},
		{Label: "协议", Value: "PD 3.x · ID 6 · 0x0DA6"},
		{Label: "Tail", Value: "odd"},
	}
	pairedFields := pdPairFields(fields)
	if len(pairedFields) != 4 {
		t.Fatalf("pdPairFields len = %d, want 4: %+v", len(pairedFields), pairedFields)
	}
	if !pairedFields[0].paired || pairedFields[0].left.Label != "响应" || pairedFields[0].right.Label != "就绪" {
		t.Fatalf("row 0 = %+v, want paired 响应 + 就绪", pairedFields[0])
	}
	if pairedFields[1].paired || pairedFields[1].left.Label != "LongFieldLabel" {
		t.Fatalf("row 1 = %+v, want solo LongFieldLabel", pairedFields[1])
	}
	if !pairedFields[2].paired || pairedFields[2].left.Label != "角色" || pairedFields[2].right.Label != "协议" {
		t.Fatalf("row 2 = %+v, want paired 角色 + 协议", pairedFields[2])
	}
	if pairedFields[3].paired || pairedFields[3].left.Label != "Tail" {
		t.Fatalf("row 3 = %+v, want solo Tail", pairedFields[3])
	}

	pdos := []pd.PDO{
		{Position: 1, Kind: "Fixed", MinV: 5, MaxV: 5, MaxA: 3},
		{Position: 2, Kind: "Fixed", MinV: 9, MaxV: 9, MaxA: 3},
		{Position: 3, Kind: "SPR AVS", MinV: 9, MaxV: 20, MaxA15V: 3, MaxA20V: 3},
		{Position: 4, Kind: "PPS", MinV: 3.3, MaxV: 21, MaxA: 5},
	}
	pairedPDOs := pdPairPDOs(pdos)
	if len(pairedPDOs) != 3 {
		t.Fatalf("pdPairPDOs len = %d, want 3: %+v", len(pairedPDOs), pairedPDOs)
	}
	if !pairedPDOs[0].paired || pairedPDOs[0].left.Position != 1 || pairedPDOs[0].right.Position != 2 {
		t.Fatalf("pdo row 0 = %+v, want paired #1+#2", pairedPDOs[0])
	}
	if pairedPDOs[1].paired || pairedPDOs[1].left.Kind != "SPR AVS" {
		t.Fatalf("pdo row 1 = %+v, want solo SPR AVS", pairedPDOs[1])
	}
	if pairedPDOs[2].paired || pairedPDOs[2].left.Position != 4 {
		t.Fatalf("pdo row 2 = %+v, want solo #4", pairedPDOs[2])
	}
}

func TestResolveChartTicksDualSlot(t *testing.T) {
	const chPower, chVoltage, chCurrent = 0, 1, 2
	d := &dashboard{}
	var ran bool
	tt := ui.NewTester(func(ctx *ui.Context) {
		ran = true
		ax0 := chart.Linear{Lo: 0, Hi: 50}
		ax1 := chart.Linear{Lo: 0, Hi: 20}

		res0 := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true)
		res1 := d.resolveChartTicks(ctx, 1, chVoltage, ax1, "V", false, true)
		if len(res0.ticks) == 0 || len(res0.labels) != len(res0.ticks) || res0.rightMargin <= 0 {
			t.Fatalf("slot 0 unexpected ticks: %+v", res0)
		}
		if len(res1.ticks) == 0 || res1.key.channel != chVoltage || res1.key.unit != "V" {
			t.Fatalf("slot 1 unexpected ticks: %+v", res1)
		}
		if d.chartTickMetrics[0].key.channel != chPower || d.chartTickMetrics[1].key.channel != chVoltage {
			t.Fatalf("slots collided: [0]=%+v [1]=%+v", d.chartTickMetrics[0].key, d.chartTickMetrics[1].key)
		}

		// Cache hit preserves exact cached slice & width.
		d.chartTickMetrics[0].rightMargin = 123.5
		if hit := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true); hit.rightMargin != 123.5 {
			t.Fatalf("expected slot 0 cache hit (123.5), got %v", hit.rightMargin)
		}

		// Scale mismatch invalidates cache.
		zoom := ctx.Zoom()
		if zoom <= 0 {
			zoom = 1
		}
		d.chartTickMetrics[0].key.scale = zoom + 1
		scaled := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true)
		if scaled.rightMargin == 123.5 || scaled.key.scale != zoom {
			t.Fatalf("expected scale mismatch invalidation, got width=%v scale=%v", scaled.rightMargin, scaled.key.scale)
		}

		// Channel switch updates slot 0 without clobbering slot 1.
		cur := d.resolveChartTicks(ctx, 0, chCurrent, chart.Linear{Lo: 0, Hi: 5}, "A", false, true)
		if cur.key.channel != chCurrent || d.chartTickMetrics[1].key.channel != chVoltage {
			t.Fatalf("channel switch affected slot 1 or failed: [0]=%+v [1]=%+v", d.chartTickMetrics[0].key, d.chartTickMetrics[1].key)
		}

		// snapshotOnly computes fresh ticks without mutating cached slot.
		d.snapshotOnly = true
		snapRes := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true)
		d.snapshotOnly = false
		if snapRes.key.channel != chPower || d.chartTickMetrics[0].key.channel != chCurrent {
			t.Fatalf("snapshotOnly mutated cache: snap=%+v cached=%+v", snapRes.key, d.chartTickMetrics[0].key)
		}
	}, 400, 200)
	tt.Frame()
	if !ran {
		t.Fatal("tester frame callback did not run")
	}
}

func TestDashboardStatusAndPageOrder(t *testing.T) {
	msgs := locales.In("en")
	cases := []struct {
		name string
		snap backend.Snapshot
		err  error
		want kit.BadgeTone
	}{
		{name: "error", snap: backend.Snapshot{State: backend.Streaming}, err: context.Canceled, want: kit.ToneDanger},
		{name: "missing", snap: backend.Snapshot{ErrorKind: "missing"}, want: kit.TonePlain},
		{name: "permission", snap: backend.Snapshot{ErrorKind: "permission"}, want: kit.ToneWarning},
		{name: "offline", snap: backend.Snapshot{}, want: kit.TonePlain},
		{name: "connecting", snap: backend.Snapshot{State: backend.Connecting}, want: kit.ToneWarning},
		{name: "waiting", snap: backend.Snapshot{State: backend.Waiting}, want: kit.ToneWarning},
		{name: "streaming", snap: backend.Snapshot{State: backend.Streaming}, want: kit.ToneSuccess},
	}
	for _, tc := range cases {
		_, _, got := dashboardStatus(msgs, tc.snap, tc.err)
		if got != tc.want {
			t.Fatalf("%s: dashboardStatus tone = %v, want %v", tc.name, got, tc.want)
		}
	}

	mob := &dashboard{mobile: true}
	if mob.pageOrder(pageOverview) >= mob.pageOrder(pagePD) ||
		mob.pageOrder(pagePD) >= mob.pageOrder(pageRecords) ||
		mob.pageOrder(pageRecords) >= mob.pageOrder(pageMore) ||
		mob.pageOrder(pageMore) >= mob.pageOrder(pageDevice) ||
		mob.pageOrder(pageDevice) >= mob.pageOrder(pageConnectionLog) ||
		mob.pageOrder(pageConnectionLog) >= mob.pageOrder(pageSettings) {
		t.Fatalf("unexpected mobile pageOrder sequence")
	}
	if mob.pageOrder(-1) != 0 || mob.pageOrder(99) != 0 {
		t.Fatalf("expected out-of-bounds mobile pageOrder to return 0, got %d/%d", mob.pageOrder(-1), mob.pageOrder(99))
	}

	desk := &dashboard{mobile: false}
	if desk.pageOrder(pageOverview) >= desk.pageOrder(pagePD) ||
		desk.pageOrder(pagePD) >= desk.pageOrder(pageRecords) ||
		desk.pageOrder(pageRecords) >= desk.pageOrder(pageDevice) ||
		desk.pageOrder(pageDevice) >= desk.pageOrder(pageConnectionLog) ||
		desk.pageOrder(pageConnectionLog) >= desk.pageOrder(pageSettings) ||
		desk.pageOrder(pageSettings) >= desk.pageOrder(pageMore) {
		t.Fatalf("unexpected desktop pageOrder sequence")
	}
	if desk.pageOrder(-1) != 0 || desk.pageOrder(99) != 0 {
		t.Fatalf("expected out-of-bounds desktop pageOrder to return 0, got %d/%d", desk.pageOrder(-1), desk.pageOrder(99))
	}
}

func TestWorkspaceLayoutAndDesktopSafeArea(t *testing.T) {
	d := &dashboard{language: "en"}
	d.settings = guiSettings{Theme: mygo.ThemeLight, Navigation: navigationSidebar, Accent: kit.DefaultAccentHex}

	var layOff, layOn workspaceLayout
	var renderGlass bool
	tt := ui.NewTesterWith(func(ctx *ui.Context) {
		d.settings.LiquidGlass = false
		layOff = d.computeWorkspaceLayout(ctx)
		d.settings.LiquidGlass = true
		layOn = d.computeWorkspaceLayout(ctx)
		d.settings.LiquidGlass = renderGlass
		p, msgs := d.frameTheme(ctx)
		d.draw(ctx, p, msgs, backend.Snapshot{})
	}, 1000, 700, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})

	tt.Frame()
	if layOff.floatingTop != headerHeight || layOn.floatingTop != layOff.floatingTop {
		t.Fatalf("desktop floatingTop mismatch: off=%v on=%v want=%v", layOff.floatingTop, layOn.floatingTop, float32(headerHeight))
	}
	brand0, ok := tt.Find("WattScope")
	if !ok {
		t.Fatal("missing WattScope header text")
	}
	ov0, ok := tt.Find("Overview")
	if !ok {
		t.Fatal("missing Overview sidebar tab")
	}

	renderGlass = true
	tt.Frame()
	brandGlass, _ := tt.Find("WattScope")
	ovGlass, _ := tt.Find("Overview")
	if brandGlass != brand0 || ovGlass != ov0 {
		t.Fatalf("Sidebar LiquidGlass toggle shifted layout: brand %v->%v, ov %v->%v", brand0, brandGlass, ov0, ovGlass)
	}

	// Verify TopBar mode also keeps 0.0px shift across LiquidGlass toggle.
	d.settings.Navigation = navigationHorizontal
	renderGlass = false
	tt.Frame()
	topBrand0, _ := tt.Find("WattScope")
	topOv0, _ := tt.Find("Overview")
	renderGlass = true
	tt.Frame()
	topBrandGlass, _ := tt.Find("WattScope")
	topOvGlass, _ := tt.Find("Overview")
	if topBrandGlass != topBrand0 || topOvGlass != topOv0 {
		t.Fatalf("TopBar LiquidGlass toggle shifted layout: brand %v->%v, ov %v->%v", topBrand0, topBrandGlass, topOv0, topOvGlass)
	}

	d.settings.Navigation = navigationSidebar
	tt.Frame()
	tt.SetInsets(12, 8, 0, 16)
	tt.Frame()
	if layOff.topInset != 12 || layOff.floatingTop != headerHeight || layOn.floatingTop != layOff.floatingTop {
		t.Fatalf("safe-area layout mismatch: off=%+v on=%+v", layOff, layOn)
	}
	brand1, ok := tt.Find("WattScope")
	if !ok {
		t.Fatal("missing WattScope header text with safe-area")
	}
	ov1, ok := tt.Find("Overview")
	if !ok {
		t.Fatal("missing Overview sidebar tab with safe-area")
	}
	dbX, dbY := brand1.X-brandGlass.X, brand1.Y-brandGlass.Y
	doX, doY := ov1.X-ovGlass.X, ov1.Y-ovGlass.Y
	if dbX <= 0 || dbY <= 0 || dbX != doX || dbY != doY {
		t.Fatalf("header and desktop-body drifted under SafeArea: brandDelta=(%v,%v) bodyDelta=(%v,%v)", dbX, dbY, doX, doY)
	}
	if ov1.Y < brand1.Y+brand1.H {
		t.Fatalf("sidebar overlapped header under SafeArea: ov.Y=%v, brandBottom=%v", ov1.Y, brand1.Y+brand1.H)
	}
}
