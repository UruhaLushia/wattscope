package app

import (
	"context"
	"fmt"
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

func TestChartPlotCanvasKeyAndSecondaryColor(t *testing.T) {
	_, light := kit.ResolveTheme(false, kit.DefaultAccentHex)
	_, dark := kit.ResolveTheme(true, kit.DefaultAccentHex)
	ticks := []chart.Tick{{At: 0, Value: 0}, {At: 1, Value: 10}}
	geom := chartGeometryKey{rect: ui.Rect{X: 10, Y: 20, W: 300, H: 180}, scale: 2, end: 60, series: 7}
	moved := geom
	moved.rect.X, moved.rect.Y = 40, 120

	base := makeChartPlotCanvasKey(geom, 600, 360, 2, light, 4, ticks, 0, 7, false)
	if makeChartPlotCanvasKey(moved, 600, 360, 2, light, 4, ticks, 0, 7, false) != base {
		t.Fatal("origin X/Y translation must not invalidate chartPlotCanvasKey")
	}
	if makeChartPlotCanvasKey(geom, 600, 360, 2, dark, 4, ticks, 0, 7, false) == base ||
		makeChartPlotCanvasKey(geom, 600, 360, 2, light, 4, ticks, 0, 7, true) == base ||
		makeChartPlotCanvasKey(geom, 602, 360, 2, light, 4, ticks, 0, 7, false) == base {
		t.Fatal("theme, axis, or pixel size change must invalidate chartPlotCanvasKey")
	}
	for _, pal := range []kit.Palette{light, dark} {
		for i, ch := range channels {
			if c := secondaryCurveColor(ch, pal, 0.32); c.A != 255 {
				t.Fatalf("channel %d secondaryCurveColor alpha = %d, want 255", i, c.A)
			}
		}
	}
}

func TestDashboardStatus(t *testing.T) {
	msgs := locales.In("en")
	for _, tc := range []struct {
		name string
		snap backend.Snapshot
		err  error
		want kit.BadgeTone
	}{
		{"error", backend.Snapshot{State: backend.Streaming}, context.Canceled, kit.ToneDanger},
		{"missing", backend.Snapshot{ErrorKind: "missing"}, nil, kit.TonePlain},
		{"permission", backend.Snapshot{ErrorKind: "permission"}, nil, kit.ToneWarning},
		{"offline", backend.Snapshot{}, nil, kit.TonePlain},
		{"connecting", backend.Snapshot{State: backend.Connecting}, nil, kit.ToneWarning},
		{"waiting", backend.Snapshot{State: backend.Waiting}, nil, kit.ToneWarning},
		{"streaming", backend.Snapshot{State: backend.Streaming}, nil, kit.ToneSuccess},
	} {
		if _, _, got := dashboardStatus(msgs, tc.snap, tc.err); got != tc.want {
			t.Fatalf("%s: dashboardStatus tone = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPDPairing(t *testing.T) {
	fields := pdPairFields([]pdview.Field{
		{Label: "响应", Value: "18.0 ms"},
		{Label: "就绪", Value: "20.8 ms"},
		{Label: "LongFieldLabel", Value: "This is a much longer value exceeding twenty six runes"},
		{Label: "Empty", Value: ""},
		{Label: "角色", Value: "Source / DFP"},
		{Label: "协议", Value: "PD 3.x · ID 6 · 0x0DA6"},
		{Label: "Tail", Value: "odd"},
	})
	if len(fields) != 4 ||
		!fields[0].paired || fields[0].left.Label != "响应" || fields[0].right.Label != "就绪" ||
		fields[1].paired || fields[1].left.Label != "LongFieldLabel" ||
		!fields[2].paired || fields[2].left.Label != "角色" || fields[2].right.Label != "协议" ||
		fields[3].paired || fields[3].left.Label != "Tail" {
		t.Fatalf("pdPairFields = %+v", fields)
	}

	pdos := pdPairPDOs([]pd.PDO{
		{Position: 1, Kind: "Fixed", MinV: 5, MaxV: 5, MaxA: 3},
		{Position: 2, Kind: "Fixed", MinV: 9, MaxV: 9, MaxA: 3},
		{Position: 3, Kind: "SPR AVS", MinV: 9, MaxV: 20, MaxA15V: 3, MaxA20V: 3},
		{Position: 4, Kind: "PPS", MinV: 3.3, MaxV: 21, MaxA: 5},
	})
	if len(pdos) != 3 ||
		!pdos[0].paired || pdos[0].left.Position != 1 || pdos[0].right.Position != 2 ||
		pdos[1].paired || pdos[1].left.Kind != "SPR AVS" ||
		pdos[2].paired || pdos[2].left.Position != 4 {
		t.Fatalf("pdPairPDOs = %+v", pdos)
	}
}

func TestResolveChartTicksDualSlot(t *testing.T) {
	const chPower, chVoltage, chCurrent = 0, 1, 2
	d := &dashboard{}
	var ran bool
	tt := ui.NewTester(func(ctx *ui.Context) {
		ran = true
		ax0, ax1 := chart.Linear{Lo: 0, Hi: 50}, chart.Linear{Lo: 0, Hi: 20}
		res0 := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true)
		res1 := d.resolveChartTicks(ctx, 1, chVoltage, ax1, "V", false, true)
		if len(res0.ticks) == 0 || len(res0.labels) != len(res0.ticks) || res0.rightMargin <= 0 ||
			len(res1.ticks) == 0 || res1.key.channel != chVoltage || res1.key.unit != "V" ||
			d.chartTickMetrics[0].key.channel != chPower || d.chartTickMetrics[1].key.channel != chVoltage {
			t.Fatalf("dual slot init failed: res0=%+v res1=%+v", res0, res1)
		}

		// Cache hit, scale invalidation, channel switch isolation, and snapshotOnly immutability.
		d.chartTickMetrics[0].rightMargin = 123.5
		if hit := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true); hit.rightMargin != 123.5 {
			t.Fatalf("expected cache hit (123.5), got %v", hit.rightMargin)
		}
		zoom := max(ctx.Zoom(), 1)
		d.chartTickMetrics[0].key.scale = zoom + 1
		if scaled := d.resolveChartTicks(ctx, 0, chPower, ax0, "W", false, true); scaled.rightMargin == 123.5 || scaled.key.scale != zoom {
			t.Fatalf("scale mismatch did not invalidate: width=%v scale=%v", scaled.rightMargin, scaled.key.scale)
		}
		if cur := d.resolveChartTicks(ctx, 0, chCurrent, chart.Linear{Lo: 0, Hi: 5}, "A", false, true); cur.key.channel != chCurrent || d.chartTickMetrics[1].key.channel != chVoltage {
			t.Fatalf("channel switch affected slot 1: [0]=%+v [1]=%+v", d.chartTickMetrics[0].key, d.chartTickMetrics[1].key)
		}
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

func TestReconnectHoldTransitions(t *testing.T) {
	svc := &backend.Service{}
	prev := backend.Snapshot{
		State:  backend.Streaming,
		Device: device.Info{ID: "dev-a", Name: "Meter A", Driver: "witrn"},
		Latest: &device.Sample{Voltage: 5.0},
	}
	d := &dashboard{service: svc}
	var live, got backend.Snapshot
	var reconn, disabled bool
	tt := ui.NewTester(func(ctx *ui.Context) {
		got, reconn, disabled = d.resolveDisplaySnapshot(ctx, live)
	}, 200, 100)

	// Same-device hold window: <250ms enabled, >=250ms disabled, >=900ms released -> Connecting.
	d.reconnectHold = reconnectHold{service: svc, snap: prev, pending: true}
	live = backend.Snapshot{State: backend.Stopped}
	tt.Frame()
	if !reconn || disabled || got.State != backend.Streaming || got.Device.ID != "dev-a" || d.reconnectHold.pending {
		t.Fatalf("t=0ms: got=(%v,%q) reconn=%v disabled=%v pending=%v", got.State, got.Device.ID, reconn, disabled, d.reconnectHold.pending)
	}
	tt.Advance(reconnectDisableDelay - time.Millisecond)
	if !reconn || disabled {
		t.Fatalf("t=249ms: reconn=%v disabled=%v, want true/false", reconn, disabled)
	}
	tt.Advance(2 * time.Millisecond)
	if !reconn || !disabled || got.State != backend.Streaming {
		t.Fatalf("t=251ms: state=%v reconn=%v disabled=%v", got.State, reconn, disabled)
	}
	tt.Advance(reconnectGrace - reconnectDisableDelay)
	if reconn || !disabled || got.State != backend.Connecting || d.reconnectHold.service != nil {
		t.Fatalf("t>=900ms: state=%v reconn=%v disabled=%v hold=%v", got.State, reconn, disabled, d.reconnectHold.service != nil)
	}

	// Recoverable report error keeps hold; missing error or new device releases immediately.
	d.reconnectHold = reconnectHold{service: svc, snap: prev, pending: true}
	live = backend.Snapshot{State: backend.Waiting, ErrorKind: "report", Error: "invalid sample"}
	tt.Frame()
	if !reconn || disabled || got.Device.ID != "dev-a" || d.reconnectHold.service == nil {
		t.Fatalf("report error should keep hold: id=%q reconn=%v disabled=%v", got.Device.ID, reconn, disabled)
	}
	d.reconnectHold = reconnectHold{service: svc, snap: prev, pending: true}
	live = backend.Snapshot{State: backend.Waiting, ErrorKind: "missing", Error: "no device"}
	tt.Frame()
	if reconn || disabled || got.ErrorKind != "missing" || d.reconnectHold.service != nil {
		t.Fatalf("missing error should release hold: kind=%q reconn=%v disabled=%v", got.ErrorKind, reconn, disabled)
	}
	d.reconnectHold = reconnectHold{service: svc, snap: prev, pending: true}
	live = backend.Snapshot{State: backend.Connecting, Device: device.Info{ID: "dev-b", Name: "Meter B", Driver: "witrn"}}
	tt.Frame()
	if reconn || !disabled || got.Device.ID != "dev-b" || d.reconnectHold.service != nil {
		t.Fatalf("device switch failed: id=%q reconn=%v disabled=%v", got.Device.ID, reconn, disabled)
	}

	// Offline connect normalizes Stopped -> Connecting until grace expires; snapshotOnly stays read-only.
	d.reconnectHold = reconnectHold{service: svc, pending: true}
	live = backend.Snapshot{State: backend.Stopped}
	tt.Frame()
	if reconn || !disabled || got.State != backend.Connecting || d.reconnectHold.service == nil {
		t.Fatalf("offline connect failed: state=%v reconn=%v disabled=%v", got.State, reconn, disabled)
	}
	tt.Advance(reconnectGrace)
	if d.reconnectHold.service != nil {
		t.Fatal("offline connect grace wake should clear reconnectHold")
	}
	d.snapshotOnly = true
	d.reconnectHold = reconnectHold{service: svc, snap: prev, pending: true}
	tt.Frame()
	if !reconn || disabled || got.Device.ID != "dev-a" || !d.reconnectHold.pending || !d.reconnectHold.startedAt.IsZero() {
		t.Fatalf("snapshotOnly failed: id=%q reconn=%v disabled=%v pending=%v", got.Device.ID, reconn, disabled, d.reconnectHold.pending)
	}
}

func TestSnapshotForRevealIsolatesListState(t *testing.T) {
	const (
		canvasW = 220
		canvasH = 180
		listX   = float32(72)
	)
	cases := []struct {
		name  string
		tag   string
		count int
		pick  func(*dashboard) *ui.ListState
		raw   func(*dashboard) *ui.ListState
		atEnd bool
	}{
		{
			name:  "packets_end",
			tag:   "pkt",
			count: 48,
			pick:  func(d *dashboard) *ui.ListState { return d.packetListState() },
			raw:   func(d *dashboard) *ui.ListState { return &d.packets.list },
			atEnd: true,
		},
		{
			name:  "packets_mid",
			tag:   "pkt",
			count: 48,
			pick:  func(d *dashboard) *ui.ListState { return d.packetListState() },
			raw:   func(d *dashboard) *ui.ListState { return &d.packets.list },
			atEnd: false,
		},
		{
			name:  "log_end",
			tag:   "log",
			count: 20,
			pick:  func(d *dashboard) *ui.ListState { return d.logListState() },
			raw:   func(d *dashboard) *ui.ListState { return &d.logView.list },
			atEnd: true,
		},
		{
			name:  "log_mid",
			tag:   "log",
			count: 20,
			pick:  func(d *dashboard) *ui.ListState { return d.logListState() },
			raw:   func(d *dashboard) *ui.ListState { return &d.logView.list },
			atEnd: false,
		},
	}

	msgs := locales.In("en")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &dashboard{}
			live := tc.pick(d)
			selected := tc.count - 1
			if !tc.atEnd {
				selected = 10
			}
			live.Selected = &selected
			if tc.atEnd {
				live.ScrollToEnd()
			} else {
				live.ScrollTo(10, ui.Start)
			}

			renderList := func(ctx *ui.Context, key string, state *ui.ListState, prefix string) {
				ui.Row(ctx).Fill().Children(func() {
					ui.Box(ctx).Width(listX).FillHeight()
					ui.Box(ctx).Grow(1).FillHeight().MinHeight(0).Clip().Children(func() {
						ui.List(ctx.Key(key), state, tc.count).Fill().Clip().
							ItemKey(func(i int) any { return i }).
							Rows(func(row ui.ListRow) {
								ui.Text(row.Context, fmt.Sprintf("%s-%s-%d", prefix, tc.tag, row.Index)).
									SingleLine().Height(28)
							})
					})
				})
			}

			showReveal := false
			var lastSnap *dashboard
			tt := ui.NewTester(func(ctx *ui.Context) {
				stage := ui.Box(ctx.Key("theme-stage")).Absolute().Top(0).Right(0).Bottom(0).Left(0)
				stage.Children(func() {
					if showReveal {
						d.warmRevealListData(msgs)
						snap := d.snapshotForReveal(false)
						lastSnap = snap
						snapList := tc.pick(snap)
						snapSel := selected
						snapList.Selected = &snapSel
						if !tc.atEnd {
							snapList.FollowEnd = true
						}
						ui.Box(ctx.Key("theme-reveal-alternate")).Absolute().Fill().ZIndex(0).
							PassThrough().IgnoreInput(true).AccessHidden(true).Children(func() {
							renderList(ctx, "snap-"+tc.tag, snapList, "snap")
						})
					} else {
						d.revealLists = nil
					}
					ui.Box(ctx.Key("workspace-host")).Fill().ZIndex(1).Children(func() {
						renderList(ctx, "live-"+tc.tag, tc.pick(d), "live")
					})
				})
			}, canvasW, canvasH)

			// First pass: lay out the single live list so Visible()/AtEnd() are populated.
			tt.Frame()
			if live.AtEnd() != tc.atEnd {
				t.Fatalf("single list AtEnd=%v, want %v", live.AtEnd(), tc.atEnd)
			}
			firstBefore, lastBefore := live.Visible()
			if tc.atEnd && lastBefore != tc.count-1 {
				t.Fatalf("single list at end visible=(%d,%d), want last=%d", firstBefore, lastBefore, tc.count-1)
			}
			if !tc.atEnd && (firstBefore != 10 || lastBefore >= tc.count-1) {
				t.Fatalf("single list mid-scroll visible=(%d,%d), want first=10 and last<%d", firstBefore, lastBefore, tc.count-1)
			}

			// Render multiple consecutive reveal frames and verify revealLists reuse + isolation.
			showReveal = true
			var wantRevealLists *revealListStates
			var wantSnapList *ui.ListState
			for frame := range 3 {
				tt.Frame()
				if lastSnap == nil || d.revealLists == nil {
					t.Fatalf("frame %d: reveal state not populated (lastSnap=%v revealLists=%v)", frame, lastSnap != nil, d.revealLists != nil)
				}
				snapList := tc.pick(lastSnap)
				if frame == 0 {
					wantRevealLists = d.revealLists
					wantSnapList = snapList
				} else if d.revealLists != wantRevealLists || snapList != wantSnapList {
					t.Fatalf("frame %d: revealLists was not reused (got %p/%p, want %p/%p)", frame, d.revealLists, snapList, wantRevealLists, wantSnapList)
				}
				if snapList == live || tc.raw(lastSnap) == snapList {
					t.Fatalf("frame %d: snapshot list state was not isolated from live/raw fields", frame)
				}
				if snapList.AtEnd() != tc.atEnd || live.AtEnd() != tc.atEnd {
					t.Fatalf("frame %d: AtEnd mismatch: snap=%v live=%v want=%v", frame, snapList.AtEnd(), live.AtEnd(), tc.atEnd)
				}
				snapFirst, snapLast := snapList.Visible()
				liveFirst, liveLast := live.Visible()
				if snapFirst != liveFirst || snapLast != liveLast {
					t.Fatalf("frame %d: visible range mismatch: snap=(%d,%d) live=(%d,%d)", frame, snapFirst, snapLast, liveFirst, liveLast)
				}
				if !tc.atEnd && (snapFirst != 10 || snapLast >= tc.count-1) {
					t.Fatalf("frame %d: snapshot jumped away from mid-scroll row 10: snap=(%d,%d)", frame, snapFirst, snapLast)
				}
				for _, row := range []int{liveFirst, selected, liveLast} {
					for _, prefix := range []string{"snap", "live"} {
						label := fmt.Sprintf("%s-%s-%d", prefix, tc.tag, row)
						r, ok := tt.Find(label)
						if !ok || r.W <= 0 || r.H <= 0 {
							t.Fatalf("frame %d: row %q missing or zero-sized: ok=%v rect=%+v", frame, label, ok, r)
						}
						if r.X < listX || r.X >= listX*1.5 {
							t.Fatalf("frame %d: row %q shifted horizontally out of list bounds: rect=%+v (listX=%v)", frame, label, r, listX)
						}
					}
				}
			}

			// End reveal: revealLists must be released cleanly while live list keeps rendering.
			showReveal = false
			tt.Frame()
			if d.revealLists != nil {
				t.Fatalf("expected d.revealLists released after reveal ends, got %p", d.revealLists)
			}
			rLive, okLive := tt.Find(fmt.Sprintf("live-%s-%d", tc.tag, selected))
			if !okLive || rLive.W <= 0 || rLive.H <= 0 || rLive.X < listX || rLive.X >= listX*1.5 {
				t.Fatalf("live row invalid after reveal ended: ok=%v rect=%+v", okLive, rLive)
			}
		})
	}

	t.Run("live_pd_canReturnLive_isolation", func(t *testing.T) {
		packets := make([]pd.Packet, 48)
		for i := range packets {
			packets[i] = pd.Packet{Sequence: uint64(i + 1), Elapsed: float64(i) * 0.1, SOP: "SOP", Type: "GoodCRC"}
		}
		d := &dashboard{page: pagePD, service: &backend.Service{}}
		d.packets.live = backend.PacketSnapshot{Packets: packets}

		// 1. Before first layout: unlaid live list must not trigger canReturnLive on live or snapshot.
		d.warmRevealListData(msgs)
		snap := d.snapshotForReveal(false)
		if !d.packetListAtEnd() || !snap.packetListAtEnd() {
			t.Fatalf("unlaid list should report atEnd=true: live=%v snap=%v", d.packetListAtEnd(), snap.packetListAtEnd())
		}
		liveFrame := d.preparePDFrame(msgs, false, 1200)
		snapFrame := snap.preparePDFrame(msgs, false, 1200)
		if len(liveFrame.visible) != len(packets) || len(snapFrame.visible) != len(packets) {
			t.Fatalf("unexpected visible packet counts: live=%d snap=%d want=%d", len(liveFrame.visible), len(snapFrame.visible), len(packets))
		}
		if liveFrame.canReturnLive || snapFrame.canReturnLive {
			t.Fatalf("unlaid live PD should have canReturnLive=false: live=%v snap=%v", liveFrame.canReturnLive, snapFrame.canReturnLive)
		}

		// 2. Laid out at end: canReturnLive stays false.
		tt := ui.NewTester(func(ctx *ui.Context) {
			ui.List(ctx.Key("live-pd"), d.packetListState(), len(packets)).Fill().Rows(func(row ui.ListRow) {
				ui.Text(row.Context, fmt.Sprintf("#%d", row.Index+1)).SingleLine().Height(28)
			})
		}, canvasW, canvasH)
		d.packetListState().ScrollToEnd()
		tt.Frame()
		if !d.packets.list.AtEnd() {
			t.Fatal("expected live packets list at end after ScrollToEnd")
		}
		d.warmRevealListData(msgs)
		snap = d.snapshotForReveal(false)
		liveFrame = d.preparePDFrame(msgs, false, 1200)
		snapFrame = snap.preparePDFrame(msgs, false, 1200)
		if !snap.packetListAtEnd() || liveFrame.canReturnLive || snapFrame.canReturnLive {
			t.Fatalf("at-end live PD should have canReturnLive=false: snapAtEnd=%v live=%v snap=%v", snap.packetListAtEnd(), liveFrame.canReturnLive, snapFrame.canReturnLive)
		}

		// 3. Laid out and scrolled away from end: canReturnLive is true on both live and snapshot.
		d.packetListState().ScrollTo(10, ui.Start)
		tt.Frame()
		if d.packets.list.AtEnd() {
			t.Fatal("expected live packets list not at end after ScrollTo(10)")
		}
		d.warmRevealListData(msgs)
		snap = d.snapshotForReveal(false)
		liveFrame = d.preparePDFrame(msgs, false, 1200)
		snapFrame = snap.preparePDFrame(msgs, false, 1200)
		if snap.packetListAtEnd() || !liveFrame.canReturnLive || !snapFrame.canReturnLive {
			t.Fatalf("scrolled-up live PD should have canReturnLive=true: snapAtEnd=%v live=%v snap=%v", snap.packetListAtEnd(), liveFrame.canReturnLive, snapFrame.canReturnLive)
		}

		// 4. Release revealLists when reveal ends.
		d.revealLists = nil
		if d.revealLists != nil || d.packetListState() != &d.packets.list || d.logListState() != &d.logView.list {
			t.Fatal("revealLists cleanup did not restore direct live list state")
		}
	})
}

func TestComputePageInsetsLandscapeSubnav(t *testing.T) {
	layout := workspaceLayout{landscape: true, position: navigationSide}
	subnavPage := computePageInsets(layout, workspacePage{}, 0)
	if subnavPage.top != 6 || subnavPage.start != 12 || subnavPage.end != 12 || subnavPage.bottom != 8 {
		t.Fatalf("landscape subnav non-list insets = %+v, want top=6 start=12 end=12 bottom=8", subnavPage)
	}
	floatingPage := computePageInsets(layout, workspacePage{}, 50)
	if floatingPage.top != 60 {
		t.Fatalf("landscape floating non-list top = %v, want 60", floatingPage.top)
	}
	listPage := computePageInsets(layout, workspacePage{list: func(float32) {}}, 0)
	if listPage.top != 4 {
		t.Fatalf("landscape list top = %v, want 4", listPage.top)
	}
