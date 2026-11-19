package app

import (
	"context"
	"crypto/sha256"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/internal/session"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
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

func TestOverviewAnimationFrames(t *testing.T) {
	d := chartFrameFixture()
	tester := ui.NewTester(d.view, 1120, 720)
	tester.SetScale(2)
	tester.Advance(time.Second)
	d.goPage(pageSettings)
	tester.Frame()
	tester.Advance(time.Second)
	d.goPage(pageOverview)
	tester.Frame()
	var previous [32]byte
	for _, delta := range []time.Duration{0, 33 * time.Millisecond, 50 * time.Millisecond, 83 * time.Millisecond, time.Second} {
		tester.Advance(delta)
		tester.Frame()
		frame := sha256.Sum256(tester.Image().Pix)
		if frame == previous {
			t.Fatal("page did not change during transition")
		}
		t.Logf("transition frame %x", frame)
		previous = frame
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
	d := &dashboard{
		ctx:      ctx,
		cancel:   cancel,
		done:     done,
		service:  service,
		language: "en",
		mobile:   mobile,
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

func TestDesktopLayoutAndNavigation(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1120, 720)
	tt.Frame()

	if !tt.HasText("WattScope") {
		t.Fatal("expected WattScope brand title")
	}
	if !tt.HasText(msgs.Power()) || !tt.HasText(msgs.Voltage()) || !tt.HasText(msgs.Current()) {
		t.Fatal("expected hero metric cards on Overview")
	}

	// Switch to top navigation and navigate across all pages.
	d.settings.Navigation = navigationHorizontal
	tt.Frame()
	for _, page := range []struct {
		index int
		label string
	}{
		{pagePD, msgs.PagePd()},
		{pageRecords, msgs.GuiRecords()},
		{pageDevice, msgs.GuiDevice()},
		{pageSettings, msgs.GuiSettings()},
		{pageOverview, msgs.GuiOverview()},
	} {
		if err := tt.Click(page.label); err != nil {
			t.Fatalf("click nav %q: %v", page.label, err)
		}
		tt.Advance(350 * time.Millisecond)
		if d.page != page.index {
			t.Fatalf("expected page %d after clicking %q, got %d", page.index, page.label, d.page)
		}
	}

	// Top navigation keeps identical tab dimensions and status text at 849 and 767 without mid-width shrinking.
	zh := locales.In("zh-Hans")
	d.language, d.settings.Language = "zh-Hans", "zh-Hans"
	d.settings.Navigation = navigationHorizontal
	for _, w := range []int{849, 767} {
		tt.SetSize(w, 782)
		tt.Frame()
		for _, label := range []string{zh.GuiOverview(), zh.PagePd(), zh.GuiRecords(), zh.GuiDevice(), zh.GuiSettings()} {
			if !tt.HasText(label) {
				t.Fatalf("expected top nav label %q to stay visible at %dpx width", label, w)
			}
		}
		if !tt.HasText(zh.SimulatedMeter()) {
			t.Fatalf("expected status pill text %q to stay visible at %dpx width", zh.SimulatedMeter(), w)
		}
	}
	rWide, ok1 := tt.Find(zh.GuiOverview())
	tt.SetSize(849, 782)
	tt.Frame()
	r849, ok2 := tt.Find(zh.GuiOverview())
	if !ok1 || !ok2 || math.Abs(float64(rWide.W-r849.W)) > 0.5 {
		t.Fatalf("expected top nav tab width unchanged between 767px (%+v) and 849px (%+v)", rWide, r849)
	}

	// Enforce desktop minimum size renders key overview/status content cleanly without clipping.
	tt.SetSize(minWindowWidth, minWindowHeight)
	tt.Frame()
	if !tt.HasText(zh.SimulatedMeter()) || !tt.HasText(zh.ActionHold()) {
		t.Fatalf("expected status pill and Hold action visible at minimum window %dx%d", minWindowWidth, minWindowHeight)
	}

	// Test narrow desktop icon-rail sidebar and top bar collapse without panics.
	d.language, d.settings.Language = "en", "en"
	d.settings.Navigation = navigationSidebar
	tt.SetSize(560, 520)
	tt.Frame()
	d.settings.Navigation = navigationHorizontal
	tt.SetSize(440, 520)
	tt.Frame()
}

func TestOverviewHoldAndSettingsControls(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1080, 700)
	tt.Frame()

	if err := tt.Click(msgs.ActionHold()); err != nil {
		t.Fatalf("click Hold: %v", err)
	}
	tt.Frame()
	if d.held == nil {
		t.Fatal("expected chart to enter Hold state")
	}
	if err := tt.Click(msgs.ActionResume()); err != nil {
		t.Fatalf("click Resume: %v", err)
	}
	tt.Frame()
	if d.held != nil {
		t.Fatal("expected chart to resume live")
	}

	// Go to Settings and test accent swatches, dark mode, and language switch via kit anchored popup menus.
	d.goPage(pageSettings)
	tt.Frame()
	if labelRect, ok := tt.Find(msgs.GuiTheme()); !ok {
		t.Fatal("expected Theme row label")
	} else {
		tt.ClickAt(labelRect.X+labelRect.W/2, labelRect.Y+labelRect.H/2)
		tt.Frame()
		if *d.menuOpen("theme") {
			t.Fatal("expected clicking Theme label side of row not to open popup menu")
		}
	}
	if err := tt.Click(msgs.GuiTheme() + " · " + msgs.GuiLight()); err != nil {
		t.Fatalf("open Theme menu: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if err := tt.Click(msgs.GuiDark()); err != nil {
		t.Fatalf("click Dark in Theme menu: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.settings.Theme != mygo.ThemeDark {
		t.Fatalf("expected dark theme, got %v", d.settings.Theme)
	}
	if err := tt.Click(kit.AccentPresets[1]); err != nil {
		t.Fatalf("click accent swatch: %v", err)
	}
	tt.Frame()
	if d.settings.Accent != kit.AccentPresets[1] {
		t.Fatalf("expected accent %s, got %s", kit.AccentPresets[1], d.settings.Accent)
	}
	glassSwitch, ok := tt.AccessByRole(ui.RoleSwitch, msgs.GuiLiquidGlass())
	if !ok || glassSwitch.Checked || glassSwitch.Bounds.H == 0 {
		t.Fatal("expected visible, unchecked Liquid Glass switch")
	}
	glassBounds := glassSwitch.Bounds
	tt.ClickAt(glassBounds.X+glassBounds.W/2, glassBounds.Y+glassBounds.H/2)
	tt.Frame()
	var restored dashboard
	restored.loadSettings("en")
	if !d.settings.LiquidGlass || !restored.settings.LiquidGlass {
		t.Fatal("expected Liquid Glass to turn on immediately and stay saved")
	}
	switchInfo, ok := tt.AccessByRole(ui.RoleSwitch, msgs.GuiSmoothChart())
	if !ok || !switchInfo.Checked || switchInfo.Bounds.H == 0 {
		t.Fatal("expected visible, enabled chart smoothing switch")
	}
	bounds := switchInfo.Bounds
	tt.ClickAt(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	tt.Frame()
	restored.loadSettings("en")
	if d.settings.SmoothChart || restored.settings.SmoothChart {
		t.Fatal("expected chart smoothing to be turned off and saved")
	}
	if err := tt.Click(msgs.GuiLanguage() + " · English"); err != nil {
		t.Fatalf("open Language menu: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if err := tt.Click("简体中文"); err != nil {
		t.Fatalf("click Chinese language in Language menu: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.language != "zh-Hans" {
		t.Fatalf("expected language zh-Hans, got %s", d.language)
	}
}

func TestChartSmoothing(t *testing.T) {
	history := []device.Sample{
		{Elapsed: 0, Power: 10},
		{Elapsed: 0.05, Power: 20},
		{Elapsed: 0.10, Power: 10},
		{Elapsed: 0.15, Segment: 1, Power: 15},
		{Elapsed: 0.90, Segment: 1, Power: 5},
	}
	rect := ui.Rect{W: 320, H: 160}
	var d dashboard
	raw := d.prepareChartGeometry(history, [7]backend.ChartRange{}, rect, 0, 1, 0, 1)
	rawPeakY, rawNewSegmentY := raw.lines[0][1][1], raw.lines[0][4][1]
	d.settings.SmoothChart = true
	smoothed := d.prepareChartGeometry(history, [7]backend.ChartRange{}, rect, 0, 1, 0, 1)
	points := smoothed.lines[0]
	rawSpike := points[0][1] - rawPeakY
	smoothedSpike := points[0][1] - points[1][1]
	if smoothedSpike <= 0 || smoothedSpike >= rawSpike*0.1 {
		t.Fatalf("expected a single-sample spike to shrink below 10%%, got %g px from %g px", smoothedSpike, rawSpike)
	}
	if !math.IsNaN(float64(points[3][0])) || points[4][1] != rawNewSegmentY || smoothed.lastValue != 5 {
		t.Fatal("expected smoothing to restart on a new segment or after a gap")
	}
}

func TestMobileSafeAreaBottomBarAndPDModal(t *testing.T) {
	d, cleanup := newTestDashboard(t, true)
	defer cleanup()

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 390, 844)
	tt.SetInsets(44, 0, 34, 0)
	tt.Frame()

	// Mobile top bar renders current page title below the 44px top safe area without desktop chrome.
	if tt.HasText("WattScope") {
		t.Fatal("did not expect desktop WattScope brand chrome inside mobile top bar")
	}
	titleRect, ok := tt.Find(msgs.GuiOverview())
	if !ok || titleRect.Y < 44 || titleRect.Y+titleRect.H > 44+mobileHeaderHeight {
		t.Fatalf("expected page title inside mobile top bar below 44px top safe area, got %+v (ok=%v)", titleRect, ok)
	}

	// Verify mobile Settings uses anchored popup menus and hides desktop navigation setting.
	if err := tt.Click(msgs.GuiMore()); err != nil {
		t.Fatalf("tap More bottom tab: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	tt.Frame()
	if err := tt.Click(msgs.GuiSettings()); err != nil {
		t.Fatalf("open Settings from More: %v", err)
	}
	for range 30 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if tt.HasText(msgs.GuiNavigation()) {
		t.Fatal("did not expect desktop navigation placement setting on mobile")
	}
	if err := tt.Click(msgs.GuiTheme() + " · " + msgs.GuiLight()); err != nil {
		t.Fatalf("tap Theme row on mobile: %v", err)
	}
	for range 20 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if !*d.menuOpen("theme") {
		t.Fatal("expected theme popup menu to open on mobile")
	}
	if err := tt.Click(msgs.GuiDark()); err != nil {
		t.Fatalf("choose Dark theme in mobile popup menu: %v", err)
	}
	for range 25 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.settings.Theme != mygo.ThemeDark {
		t.Fatalf("expected dark theme after choosing in menu, got %v", d.settings.Theme)
	}
	d.setTheme(string(mygo.ThemeLight))
	tt.Frame()

	// Return to Overview and verify mobile hero metrics card, chart range menu, and bottom bar safe area.
	if err := tt.Click(msgs.GuiOverview()); err != nil {
		t.Fatalf("return to Overview: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	tt.Frame()

	if vr, ok := tt.Find(msgs.Voltage()); !ok {
		t.Fatal("expected Voltage cell in mobile metrics card")
	} else {
		tt.ClickAt(vr.X+vr.W/2, vr.Y+vr.H/2)
		tt.Frame()
		if d.channel != 1 {
			t.Fatalf("expected channel=1 after tapping Voltage cell, got %d", d.channel)
		}
	}
	if !tt.HasText("1m") {
		t.Fatal("expected default 1m range badge on mobile")
	}
	if _, ok := tt.Find(msgs.VisibleRange()); ok {
		t.Fatal("expected chart range dropdown to be hidden on mobile")
	}

	// Bottom bar tab must stay above the 34px bottom safe area.
	rect, ok := tt.Find(msgs.GuiMore())
	if !ok {
		t.Fatal("expected More tab in mobile bottom bar")
	}
	if rect.Y+rect.H > 844-34+1 {
		t.Fatalf("bottom tab overlaps safe area: bottom=%v limit=%v", rect.Y+rect.H, 844-34)
	}

	// Simulate virtual keyboard open: bottom bar hides cleanly.
	tt.SetInsets(44, 0, 280, 0)
	tt.Frame()
	if _, visible := tt.Find(msgs.GuiMore()); visible {
		t.Fatal("expected mobile bottom bar to hide when keyboard is open")
	}

	// Restore normal insets and test PD offline packet list + detail modal on mobile.
	tt.SetInsets(44, 0, 34, 0)
	d.packets.offline = &backend.PacketSnapshot{
		Total: 1,
		Packets: []pd.Packet{{
			Sequence:   1,
			Elapsed:    0.125,
			SOP:        "SOP",
			Type:       "Source_Capabilities",
			PowerRole:  "Source",
			DataRole:   "DFP",
			Revision:   "3.0",
			CRC:        "valid",
			MessageHex: "a1112c910108",
			PDOs: []pd.PDO{{
				Position: 1,
				Kind:     "Fixed",
				MinV:     5,
				MaxV:     5,
				MaxA:     3,
				MaxW:     15,
			}},
		}},
	}
	pdTab, ok := tt.AccessByRole(ui.RoleTab, msgs.PagePd())
	if !ok {
		t.Fatal("expected USB PD tab in mobile bottom bar")
	}
	tt.ClickAt(pdTab.Bounds.X+pdTab.Bounds.W/2, pdTab.Bounds.Y+pdTab.Bounds.H/2)
	tt.Advance(350 * time.Millisecond)
	tt.Frame()

	// No bottom preview pane on mobile; tapping a packet row directly opens the bottom sheet drawer.
	if err := tt.Click("#1 Source_Capabilities"); err != nil {
		t.Fatalf("tap packet row on mobile: %v", err)
	}
	for range 20 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if !d.packets.expanded {
		r, okFind := tt.Find("#1 Source_Capabilities")
		t.Fatalf("expected tapping packet row on mobile to open detail sheet (page=%d visible=%d rowRect=%+v okFind=%v revealing=%+v)", d.page, len(d.packets.visible), r, okFind, d.themeReveal)
	}
	if err := tt.Click(msgs.GuiCopy()); err != nil {
		t.Fatalf("tap Copy raw bytes: %v", err)
	}
	tt.Frame()
	if !tt.HasText(msgs.GuiCopied()) {
		t.Fatal("expected toast confirmation after copying")
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if d.packets.expanded || d.page != pagePD {
		t.Fatal("expected system Back to close only the packet sheet")
	}
	openSheet := func() (float32, float32) {
		t.Helper()
		tt.Advance(350 * time.Millisecond)
		tt.Frame()
		if err := tt.Click("#1 Source_Capabilities"); err != nil {
			t.Fatalf("reopen packet sheet: %v", err)
		}
		tt.Advance(350 * time.Millisecond)
		tt.Frame()
		grabber, ok := tt.AccessByRole(ui.RoleButton, msgs.GuiClose())
		if !ok || grabber.Bounds.W < 100 {
			t.Fatalf("missing packet sheet grabber: %+v", grabber)
		}
		return grabber.Bounds.X + grabber.Bounds.W/2, grabber.Bounds.Y + grabber.Bounds.H/2
	}
	touchX, touchY := openSheet()
	tt.FingerDown(touchX, touchY)
	tt.FingerUp(touchX, touchY)
	tt.Frame()
	if d.packets.expanded {
		t.Fatal("expected tapping the grabber to close the packet sheet")
	}
	touchX, touchY = openSheet()
	tt.FingerDown(touchX, touchY)
	tt.FingerMove(touchX, touchY+12)
	tt.Frame()
	tt.FingerUp(touchX, touchY+12)
	tt.Advance(350 * time.Millisecond)
	tt.Frame()
	if !d.packets.expanded {
		t.Fatal("expected a short drag to keep the packet sheet open")
	}
	tt.FingerDown(touchX, touchY)
	tt.FingerMove(touchX, touchY+100)
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	tt.FingerUp(touchX, touchY+100)
	tt.Frame()
	if d.packets.expanded {
		t.Fatal("expected dragging down from the grabber to close the packet sheet")
	}

	// Desktop split Master-Detail renders PacketDetails inline without modal.
	d.mobile = false
	d.packets.expanded = false
	tt.SetInsets(0, 0, 0, 0)
	tt.SetSize(1024, 700)
	tt.Frame()
	if !tt.HasText(msgs.PacketDetails()) {
		t.Fatal("expected inline Packet details pane on wide desktop")
	}
	_ = device.Sample{}
}

func TestMobileCapsuleHoldAndDragNavigation(t *testing.T) {
	d, cleanup := newTestDashboard(t, true)
	defer cleanup()

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 390, 844)
	tt.SetInsets(44, 0, 34, 0)
	tt.Frame()

	overviewTab, ok := tt.AccessByRole(ui.RoleTab, msgs.GuiOverview())
	if !ok {
		t.Fatal("missing Overview tab")
	}
	moreTab, ok := tt.AccessByRole(ui.RoleTab, msgs.GuiMore())
	if !ok {
		t.Fatal("missing More tab")
	}

	startX := overviewTab.Bounds.X + overviewTab.Bounds.W/2
	startY := overviewTab.Bounds.Y + overviewTab.Bounds.H/2
	endX := moreTab.Bounds.X + moreTab.Bounds.W/2

	// Touch hold and scrub from Overview across the capsule to More.
	tt.FingerDown(startX, startY)
	for step := 1; step <= 6; step++ {
		frac := float32(step) / 6
		tt.FingerMove(startX+(endX-startX)*frac, startY)
		tt.Advance(25 * time.Millisecond)
		tt.Frame()
	}
	tt.FingerUp(endX, startY)
	for range 12 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.page != pageMore {
		t.Fatalf("expected touch drag to More (page %d), got %d", pageMore, d.page)
	}

	// Touch cancel mid-drag must not change the active page.
	curOverview, ok := tt.AccessByRole(ui.RoleTab, msgs.GuiOverview())
	if !ok {
		t.Fatal("missing Overview tab while on More")
	}
	curMore, ok := tt.AccessByRole(ui.RoleTab, msgs.GuiMore())
	if !ok {
		t.Fatal("missing More tab while on More")
	}
	sx := curMore.Bounds.X + curMore.Bounds.W/2
	ox := curOverview.Bounds.X + curOverview.Bounds.W/2
	tt.FingerDown(sx, startY)
	tt.FingerMove((sx+ox)/2, startY)
	tt.Frame()
	tt.FingerCancel()
	for range 10 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.page != pageMore {
		t.Fatalf("expected canceled drag to keep More page, got %d", d.page)
	}

	// Pointer (mouse) hold and drag from More back to USB PD.
	curMore, _ = tt.AccessByRole(ui.RoleTab, msgs.GuiMore())
	pdTab, ok := tt.AccessByRole(ui.RoleTab, msgs.PagePd())
	if !ok {
		t.Fatal("missing USB PD tab")
	}
	sx = curMore.Bounds.X + curMore.Bounds.W/2
	rx := pdTab.Bounds.X + pdTab.Bounds.W/2
	tt.Move(sx, startY)
	tt.Press(sx, startY)
	tt.Frame()
	for step := 1; step <= 5; step++ {
		frac := float32(step) / 5
		tt.Move(sx+(rx-sx)*frac, startY)
		tt.Advance(25 * time.Millisecond)
		tt.Frame()
	}
	tt.Release(rx, startY)
	for range 12 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.page != pagePD {
		t.Fatalf("expected pointer drag to USB PD (page %d), got %d", pagePD, d.page)
	}
}

func TestMobileSecondaryBackAndFourTabBar(t *testing.T) {
	d, cleanup := newTestDashboard(t, true)
	defer cleanup()

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 390, 844)
	tt.SetInsets(44, 0, 34, 0)
	tt.Frame()

	for _, tabLabel := range []string{msgs.GuiOverview(), msgs.PagePd(), msgs.GuiRecords(), msgs.GuiMore()} {
		if _, ok := tt.AccessByRole(ui.RoleTab, tabLabel); !ok {
			t.Fatalf("expected bottom bar tab %q", tabLabel)
		}
	}

	if err := tt.Click(msgs.GuiRecords()); err != nil {
		t.Fatalf("tap Records bottom tab: %v", err)
	}
	tt.Frame()
	if d.page != pageRecords {
		t.Fatalf("expected pageRecords (%d), got %d", pageRecords, d.page)
	}

	if err := tt.Click(msgs.GuiMore()); err != nil {
		t.Fatalf("tap More bottom tab: %v", err)
	}
	tt.Frame()
	if err := tt.Click(msgs.GuiDeviceInfo()); err != nil {
		t.Fatalf("open Device info sub-page: %v", err)
	}
	tt.Frame()
	if d.page != pageDevice || d.pageDirection != 1 {
		t.Fatalf("expected pageDevice with direction 1, got page=%d dir=%d", d.page, d.pageDirection)
	}

	backAcc, ok := tt.AccessByRole(ui.RoleButton, msgs.GuiBack())
	if !ok || backAcc.Bounds.W < 38 || backAcc.Bounds.H < 38 {
		t.Fatalf("expected >=38dp back button target, ok=%v bounds=%+v", ok, backAcc.Bounds)
	}
	tt.FingerDown(backAcc.Bounds.X+backAcc.Bounds.W/2, backAcc.Bounds.Y+backAcc.Bounds.H/2)
	tt.Frame()
	tt.FingerUp(backAcc.Bounds.X+backAcc.Bounds.W/2, backAcc.Bounds.Y+backAcc.Bounds.H/2)
	tt.Frame()
	if d.page != pageMore || d.pageDirection != -1 {
		t.Fatalf("expected top-bar back to pop to More with dir=-1, got page=%d dir=%d", d.page, d.pageDirection)
	}

	if err := tt.Click(msgs.GuiSettings()); err != nil {
		t.Fatalf("open Settings sub-page: %v", err)
	}
	tt.Frame()
	*d.menuOpen("theme") = true
	tt.Frame()
	tt.KeyDown(ui.KeyBack)
	tt.KeyUp(ui.KeyBack)
	tt.Frame()
	if *d.menuOpen("theme") || d.page != pageSettings {
		t.Fatalf("expected KeyBack to close open menu first on Settings, open=%v page=%d", *d.menuOpen("theme"), d.page)
	}
	tt.KeyDown(ui.KeyBack)
	tt.KeyUp(ui.KeyBack)
	tt.Frame()
	if d.page != pageMore || d.pageDirection != -1 {
		t.Fatalf("expected KeyBack to return from Settings to More, got page=%d dir=%d", d.page, d.pageDirection)
	}

	if err := tt.Click(msgs.GuiConnectionLog()); err != nil {
		t.Fatalf("open Connection log sub-page: %v", err)
	}
	tt.Frame()
	tt.FingerDown(10, 320)
	tt.FingerMove(48, 320)
	tt.Frame()
	tt.FingerMove(96, 320)
	tt.Frame()
	tt.FingerUp(96, 320)
	tt.Frame()
	if d.page != pageMore || d.pageDirection != -1 {
		t.Fatalf("expected edge swipe back to pop to More, got page=%d dir=%d", d.page, d.pageDirection)
	}
}

func TestReadoutThrottleCadence(t *testing.T) {
	d := &dashboard{}
	latest := device.Sample{Elapsed: 0.05, Power: 10.0}
	snap := backend.Snapshot{
		State:  backend.Streaming,
		Latest: &latest,
	}

	var shown []float64
	tt := ui.NewTester(func(ctx *ui.Context) {
		sample, _, _ := d.throttledReadout(ctx, snap, snap)
		if sample != nil {
			shown = append(shown, sample.Power)
		}
	}, 200, 100)

	// Rapid 50ms updates within 250ms keep the initial reading steady.
	latest = device.Sample{Elapsed: 0.10, Power: 11.0}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	latest = device.Sample{Elapsed: 0.15, Power: 12.0}
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	if got := shown[len(shown)-1]; got != 10.0 {
		t.Fatalf("expected throttled readout to hold 10.0 within 250ms window, got %v", got)
	}

	// Once >= 250ms elapses, the scheduled timer wakes up and refreshes to 12.0 even without another Frame().
	tt.Advance(200 * time.Millisecond)
	if got := shown[len(shown)-1]; got != 12.0 {
		t.Fatalf("expected readout to refresh to 12.0 after 250ms, got %v", got)
	}
}

func TestShortChartDrawsFromLeftAndThenScrolls(t *testing.T) {
	samples := []device.Sample{
		{Elapsed: 120, Power: 5},
		{Elapsed: 121, Power: 6},
		{Elapsed: 124, Power: 7},
		{Elapsed: 127, Power: 8},
	}
	d, cleanup := newTestDashboardWithSamples(t, samples)
	defer cleanup()
	tt := ui.NewTester(d.view, 1120, 720)
	tt.Frame()

	assertPlacement := func(start, end, first, last float64) {
		t.Helper()
		geometry := d.chartGeometry
		points := geometry.lines[0]
		rect := geometry.key.rect
		if geometry.key.start != start || geometry.key.end != end || len(points) < 2 || rect.W <= 0 {
			t.Fatalf("expected plotted time window [%v, %v], got [%v, %v] with %d points", start, end, geometry.key.start, geometry.key.end, len(points))
		}
		firstFrac := float64((points[0][0] - rect.X) / rect.W)
		lastFrac := float64((points[len(points)-1][0] - rect.X) / rect.W)
		if math.Abs(firstFrac-first) > 1e-4 || math.Abs(lastFrac-last) > 1e-4 {
			t.Fatalf("expected plot to span %.3f–%.3f of chart width, got %.3f–%.3f", first, last, firstFrac, lastFrac)
		}
	}
	assertPlacement(120, 180, 0, 7.0/60)

	rect := d.chartGeometry.key.rect
	tt.ClickAt(rect.X+rect.W*(4.0/60), rect.Y+rect.H/2)
	tt.Frame()
	if !d.hasCursor || d.cursorAt != 124 {
		t.Fatalf("expected the cursor to select 4s from the left edge, got %v", d.cursorAt)
	}
	assertPlacement(120, 180, 0, 7.0/60)

	d.hold(d.service.Snapshot(0))
	tt.Frame()
	assertPlacement(120, 180, 0, 7.0/60)

	d.resumeLive()
	d.windowSec = 5
	tt.Frame()
	assertPlacement(122, 127, 2.0/5, 1)
}

func TestChartPointerAnchoredScrollZoom(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.settings.Navigation = navigationHorizontal

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1120, 720)
	tt.Frame()

	// 1. Live mode wheel zoom steps along clean ladder while staying Live (not unexpectedly pausing).
	legendRect, ok := tt.Find(msgs.Power() + " (W)")
	if !ok {
		t.Fatal("expected chart legend")
	}
	px := legendRect.X + 240
	py := legendRect.Y + legendRect.H + 70
	d.windowSec = 30
	tt.Frame()
	tt.Wheel(px, py, 0, -50)
	tt.Frame()
	if d.held != nil {
		t.Fatal("expected wheel zoom in live mode to keep streaming live")
	}
	if got := d.effectiveWindow(); !(got < 30 && got >= 20) {
		t.Fatalf("expected live wheel zoom up from 30s to narrow smoothly, got %v", got)
	}

	// Left-drag on a short live capture (< window) does not unexpectedly pause or drop a stray T1 cursor.
	d.resumeLive()
	d.windowSec = 60
	tt.Frame()
	tt.Press(px, py)
	tt.Move(px+80, py)
	tt.Release(px+80, py)
	tt.Frame()
	if d.held != nil || d.hasViewEnd || d.hasCursor {
		t.Fatalf("expected left-drag on short live capture to stay live without cursor (held=%v hasViewEnd=%v hasCursor=%v)", d.held != nil, d.hasViewEnd, d.hasCursor)
	}

	// 2. Populate 60s held history: hover previews, click pins, wheel anchors on pointer, drag pans, double/right-click resumes.
	samples := make([]device.Sample, 0, 1201)
	for i := 0; i <= 1200; i++ {
		elapsed := float64(i) * 0.05
		samples = append(samples, device.Sample{
			Elapsed: elapsed,
			Voltage: 5.0,
			Current: 2.0,
			Power:   10.0 + math.Sin(elapsed),
		})
	}
	latest := samples[len(samples)-1]
	d.windowSec = 60
	d.held = &backend.Snapshot{
		State:   backend.Streaming,
		Device:  device.Info{Name: "Test Meter", Driver: "demo", Simulated: true},
		Latest:  &latest,
		History: samples,
	}
	tt.Frame()

	if tt.HasText(msgs.Inspect()) {
		t.Fatal("did not expect redundant bottom Inspect slider while holding")
	}

	// Single click pins cursor cleanly; moving mouse afterward does not drag or relocate the selected timeline cursor.
	tt.ClickAt(px, py)
	tt.Frame()
	if !d.hasCursor {
		t.Fatal("expected left click on chart to pin inspection cursor")
	}
	selectedAt := d.cursorAt
	tt.Move(px+200, py)
	tt.Frame()
	if d.cursorAt != selectedAt {
		t.Fatalf("expected passive mouse movement not to disturb selected cursor %v, got %v", selectedAt, d.cursorAt)
	}

	// Clicking another position relocates the pinned T1 cursor cleanly.
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(px+160, py)
	tt.Frame()
	if !(d.cursorAt > selectedAt) {
		t.Fatalf("expected clicking rightward position to relocate cursor forward: before=%v after=%v", selectedAt, d.cursorAt)
	}

	beforeWin := d.effectiveWindow()
	beforeCursorAt := d.cursorAt

	tt.Wheel(px, py, 0, -50)
	tt.Frame()
	afterWin := d.effectiveWindow()
	if !(afterWin < beforeWin) {
		t.Fatalf("expected scroll up to narrow window: before=%v after=%v", beforeWin, afterWin)
	}
	if d.cursorAt != beforeCursorAt {
		t.Fatalf("expected pinned cursor %v to remain locked across zoom, got %v", beforeCursorAt, d.cursorAt)
	}
	if !d.hasViewEnd || !(d.viewEnd < latest.Elapsed-1.0) {
		t.Fatalf("expected left-anchored wheel zoom to shift viewEnd earlier than %v, got hasViewEnd=%v viewEnd=%v", latest.Elapsed, d.hasViewEnd, d.viewEnd)
	}

	// Precise scroll events (smooth wheel / touchpad) also adjust the window.
	prevWin := d.effectiveWindow()
	tt.Scroll(px, py, 0, 40)
	tt.Frame()
	if !(d.effectiveWindow() > prevWin) {
		t.Fatalf("expected precise scroll down to widen window: prev=%v got=%v", prevWin, d.effectiveWindow())
	}

	// Zoom in and middle-drag rightwards to pan earlier along history.
	tt.Wheel(px, py, 0, -50)
	tt.Wheel(px, py, 0, -50)
	tt.Frame()
	winEndBeforePan := d.viewEnd
	tt.PressButton(2, px+120, py)
	tt.Move(px+260, py)
	tt.ReleaseButton(2, px+260, py)
	tt.Frame()
	if !(d.viewEnd < winEndBeforePan) {
		t.Fatalf("expected rightward middle-drag to pan to earlier timestamps: before=%v after=%v", winEndBeforePan, d.viewEnd)
	}

	// Right-click resumes live.
	tt.RightClickAt(px, py)
	tt.Frame()
	if d.held != nil {
		t.Fatal("expected right-click on chart to resume live")
	}
}

func TestChartMultiTraceLegendAndHover(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.settings.Navigation = navigationHorizontal

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1120, 720)

	samples := make([]device.Sample, 0, 1201)
	for i := 0; i <= 1200; i++ {
		elapsed := float64(i) * 0.05
		temp := 31.0 + elapsed*0.05
		samples = append(samples, device.Sample{
			Elapsed:     elapsed,
			Voltage:     17.4 + 0.1*math.Sin(elapsed),
			Current:     1.0 + 0.2*math.Cos(elapsed),
			Power:       17.4 + 3.5*math.Cos(elapsed),
			Temperature: &temp,
		})
	}
	latest := samples[len(samples)-1]
	d.held = &backend.Snapshot{
		State:   backend.Streaming,
		Device:  device.Info{Name: "POWER-Z KM003C", Driver: "powerz-km003c"},
		Latest:  &latest,
		History: samples,
	}
	tt.Frame()

	// Default chart range is 1m (60s) rendered as a read-only label with no dropdown button.
	if got := d.effectiveWindow(); got != 60 {
		t.Fatalf("expected default windowSec=60 (1m), got %v", got)
	}
	if !tt.HasText("1m") {
		t.Fatal("expected 1m range badge in chart header")
	}
	if _, ok := tt.Find(msgs.VisibleRange()); ok {
		t.Fatal("expected chart range dropdown button to be removed on desktop")
	}

	// Narrow window for multi-cursor spacing in test.
	d.windowSec = 20
	tt.Frame()
	if !tt.HasText("20s") {
		t.Fatal("expected range badge to reflect zoomed 20s window")
	}

	// Toggle Voltage off and optional Temperature on; primary Power remains pinned.
	beforeMask := d.effectiveSeriesMask()
	if beforeMask&(1<<1) == 0 {
		t.Fatalf("expected Voltage enabled by default in multi-trace mask %08b", beforeMask)
	}
	if err := tt.Click(msgs.Voltage() + " (" + channels[1].unit + ")"); err != nil {
		t.Fatalf("toggle Voltage legend chip: %v", err)
	}
	tt.Frame()
	if d.effectiveSeriesMask()&(1<<1) != 0 {
		t.Fatalf("expected Voltage toggled off in mask %08b", d.effectiveSeriesMask())
	}
	if err := tt.Click(msgs.Temperature() + " (" + channels[3].unit + ")"); err != nil {
		t.Fatalf("toggle Temperature legend chip: %v", err)
	}
	tt.Frame()
	if d.effectiveSeriesMask()&(1<<3) == 0 {
		t.Fatalf("expected Temperature toggled on in mask %08b", d.effectiveSeriesMask())
	}
	if d.effectiveSeriesMask()&(1<<0) == 0 {
		t.Fatalf("expected primary Power series to stay enabled in mask %08b", d.effectiveSeriesMask())
	}

	legendRect, ok := tt.Find(msgs.Power() + " (W)")
	if !ok {
		t.Fatal("expected chart legend")
	}
	px := legendRect.X + 200
	py := legendRect.Y + legendRect.H + 70

	// Pin T1 with click, then hover elsewhere to activate simultaneous T2 preview without shifting T1.
	tt.ClickAt(px, py)
	tt.Frame()
	if !d.hasCursor {
		t.Fatal("expected click to pin T1 cursor")
	}
	pinnedAt := d.cursorAt
	tt.Move(px+220, py)
	tt.Frame()
	if !d.hasHover || d.hoverAt <= pinnedAt {
		t.Fatalf("expected passive move to activate T2 hover ahead of T1 (hasHover=%v hoverAt=%v pinnedAt=%v)", d.hasHover, d.hoverAt, pinnedAt)
	}
	if d.cursorAt != pinnedAt {
		t.Fatalf("expected hover preview to keep T1 pinned at %v, got %v", pinnedAt, d.cursorAt)
	}
}

func TestChartFriendlyDragPanAndFullSessionZoom(t *testing.T) {
	if got := formatWindowLabel(5400); got != "1h30m" {
		t.Fatalf("formatWindowLabel(5400) = %q, want 1h30m", got)
	}
	if got := formatRelTime(-150); got != "−2m30s" {
		t.Fatalf("formatRelTime(-150) = %q, want −2m30s", got)
	}
	if got := formatRelTime(-5400); got != "−1h30m" {
		t.Fatalf("formatRelTime(-5400) = %q, want −1h30m", got)
	}

	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.settings.Navigation = navigationHorizontal

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1120, 720)

	// Build a 12-minute (720s) session — well beyond the old 5-minute (300s) cap.
	samples := make([]device.Sample, 0, 721)
	for i := 0; i <= 720; i++ {
		elapsed := float64(i)
		samples = append(samples, device.Sample{
			Elapsed: elapsed,
			Voltage: 12.0,
			Current: 1.5 + 0.3*math.Sin(elapsed*0.05),
			Power:   18.0 + 3.6*math.Sin(elapsed*0.05),
		})
	}
	latest := samples[len(samples)-1]
	d.held = &backend.Snapshot{
		State:        backend.Streaming,
		Device:       device.Info{Name: "POWER-Z KM003C", Driver: "powerz-km003c"},
		Latest:       &latest,
		History:      samples,
		HistoryStart: 0,
		HistoryEnd:   720,
		HistoryCount: len(samples),
	}
	tt.Frame()

	legendRect, ok := tt.Find(msgs.Power() + " (W)")
	if !ok {
		t.Fatal("expected chart legend")
	}
	px := legendRect.X + 260
	py := legendRect.Y + legendRect.H + 70

	// 1. Zooming out extends past 5m (300s) all the way to the full 12m (720s) session.
	for range 20 {
		tt.Wheel(px, py, 0, 80)
		tt.Frame()
	}
	if got := d.effectiveWindow(); math.Abs(got-720) > 1e-3 {
		t.Fatalf("expected zoom out to reach full 720s session, got %v", got)
	}
	if !d.fullSession {
		t.Fatal("expected fullSession=true when zoomed out to full session span")
	}
	if !tt.HasText("12m") {
		t.Fatal("expected range badge to display 12m")
	}

	// 2. Zoom back into 60s window; left-drag away from T1 pans timeline 1:1 (no middle-click needed).
	d.fullSession = false
	d.windowSec = 60
	d.hasViewEnd = true
	d.viewEnd = 720
	tt.Frame()

	tt.Press(px, py)
	tt.Move(px+160, py)
	tt.Release(px+160, py)
	tt.Frame()
	if !(d.viewEnd < 715) {
		t.Fatalf("expected rightward left-drag to pan earlier into history, got viewEnd=%v", d.viewEnd)
	}
	if d.hasCursor {
		t.Fatal("expected pan drag not to drop a stray T1 cursor")
	}

	pannedEnd := d.viewEnd
	tt.Advance(500 * time.Millisecond)
	tt.Press(px+200, py)
	tt.Move(px+80, py)
	tt.Release(px+80, py)
	tt.Frame()
	if !(d.viewEnd > pannedEnd) {
		t.Fatalf("expected leftward left-drag to pan forward: before=%v after=%v", pannedEnd, d.viewEnd)
	}

	// 3. Single click pins T1; normal left-drag even starting right on T1 still pans the chart directly,
	// while clicking T1 again toggles it off.
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(px, py)
	tt.Frame()
	if !d.hasCursor {
		t.Fatal("expected single click to pin T1 cursor")
	}
	t1Before := d.cursorAt
	viewEndBeforePan := d.viewEnd

	tt.Advance(500 * time.Millisecond)
	tt.Press(px, py)
	tt.Move(px+120, py)
	tt.Release(px+120, py)
	tt.Frame()
	if !(d.viewEnd < viewEndBeforePan) {
		t.Fatalf("expected normal left-drag even near T1 to pan viewport earlier: before=%v after=%v", viewEndBeforePan, d.viewEnd)
	}
	if d.cursorAt != t1Before {
		t.Fatalf("expected pan drag to keep T1 anchored at %v, got %v", t1Before, d.cursorAt)
	}

	// Because 1:1 pan moved the waveform +120px right, T1 is now located at px+120;
	// clicking px+120 toggles T1 off, and clicking px+40 pins T1 at a new timestamp.
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(px+120, py)
	tt.Frame()
	if d.hasCursor {
		t.Fatal("expected clicking pinned T1 at its panned screen position to clear cursor")
	}
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(px+40, py)
	tt.Frame()
	if !d.hasCursor {
		t.Fatal("expected clicking new position to pin T1 cursor")
	}

	// 4. Double-click while paused resumes Live.
	tt.Advance(500 * time.Millisecond)
	tt.ClickAt(px+80, py)
	tt.Advance(80 * time.Millisecond)
	tt.ClickAt(px+80, py)
	tt.Frame()
	if d.held != nil {
		t.Fatal("expected double-click on paused chart to resume live")
	}

	// 5. Auto-resume in live browsing (without manual Pause):
	//    left-drag right detaches into history; dragging left back to Now in a separate drag auto-resumes live;
	//    horizontal scroll back to Now and zooming out to full session also auto-resume live.
	dLive, cleanupLive := newTestDashboardWithSamples(t, samples)
	defer cleanupLive()
	dLive.settings.Navigation = navigationHorizontal
	dLive.windowSec = 60
	tl := ui.NewTester(dLive.view, 1120, 720)
	tl.Frame()
	lLegend, ok := tl.Find(msgs.Power() + " (W)")
	if !ok {
		t.Fatal("expected chart legend")
	}
	lx := lLegend.X + 260
	ly := lLegend.Y + lLegend.H + 70

	// Drag right from live -> detaches smoothly without hard-freezing dLive.held.
	tl.Press(lx, ly)
	tl.Move(lx+180, ly)
	tl.Release(lx+180, ly)
	tl.Frame()
	if dLive.held != nil {
		t.Fatal("expected live drag-pan not to hard-freeze held snapshot")
	}
	if !dLive.hasViewEnd || !(dLive.viewEnd < 715) {
		t.Fatalf("expected live rightward drag to pan into history, got hasViewEnd=%v viewEnd=%v", dLive.hasViewEnd, dLive.viewEnd)
	}

	// In a separate drag, drag left all the way back to the right edge -> auto-resumes live!
	tl.Advance(500 * time.Millisecond)
	tl.Press(lx+240, ly)
	tl.Move(lx, ly)
	tl.Release(lx, ly)
	tl.Frame()
	if dLive.hasViewEnd || dLive.held != nil {
		t.Fatalf("expected dragging back to live edge to auto-resume live (hasViewEnd=%v viewEnd=%v)", dLive.hasViewEnd, dLive.viewEnd)
	}

	// Horizontal scroll right detaches, horizontal scroll left back to right edge auto-resumes live.
	tl.Scroll(lx, ly, -120, 2)
	tl.Frame()
	if !dLive.hasViewEnd || !(dLive.viewEnd < 715) {
		t.Fatalf("expected horizontal scroll right from live to pan earlier, got hasViewEnd=%v viewEnd=%v", dLive.hasViewEnd, dLive.viewEnd)
	}
	tl.Scroll(lx, ly, 240, 0)
	tl.Frame()
	if dLive.hasViewEnd {
		t.Fatalf("expected horizontal scroll back to right edge to auto-resume live, got viewEnd=%v", dLive.viewEnd)
	}

	// Detached history zoomed all the way out to the full session auto-resumes live and locks fullSession.
	tl.Scroll(lx, ly, -180, 0)
	tl.Frame()
	if !dLive.hasViewEnd {
		t.Fatal("expected detached history before zoom-out")
	}
	for range 20 {
		tl.Wheel(lx, ly, 0, 80)
		tl.Frame()
	}
	if dLive.hasViewEnd {
		t.Fatalf("expected zooming out to full session to auto-resume live, got hasViewEnd=%v viewEnd=%v", dLive.hasViewEnd, dLive.viewEnd)
	}
	if !dLive.fullSession {
		t.Fatal("expected fullSession=true after zooming out to entire 12m live session")
	}

	// 6. Mobile horizontal swipe pans zoomed chart and auto-resumes live when swiped back to Now,
	//    while vertical swipe passes through cleanly without zooming.
	dm, cleanupM := newTestDashboardWithSamples(t, samples)
	defer cleanupM()
	dm.mobile = true
	dm.windowSec = 60
	tm := ui.NewTester(dm.view, 390, 844)
	tm.Frame()
	mLegend, ok := tm.Find(msgs.Power() + " (W)")
	if !ok {
		t.Fatal("expected chart legend")
	}
	mx := mLegend.X + 120
	my := mLegend.Y + mLegend.H + 70
	tm.Scroll(mx, my, -80, 2)
	tm.Frame()
	if !dm.hasViewEnd || !(dm.viewEnd < 715) {
		t.Fatalf("expected mobile horizontal swipe to pan earlier, got hasViewEnd=%v viewEnd=%v", dm.hasViewEnd, dm.viewEnd)
	}
	tm.Scroll(mx, my, 160, 0)
	tm.Frame()
	if dm.hasViewEnd {
		t.Fatalf("expected mobile horizontal swipe back to right edge to auto-resume live, got viewEnd=%v", dm.viewEnd)
	}
	tm.Scroll(mx, my, 0, 80)
	tm.Frame()
	if math.Abs(dm.windowSec-60) > 1e-6 {
		t.Fatalf("expected mobile vertical scroll not to zoom chart, got windowSec=%v", dm.windowSec)
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
		StaleAfter:        5 * time.Second,
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

func TestWide2KOverviewLayout(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.language = "zh-Hans"
	d.settings.Language = "zh-Hans"
	d.settings.Navigation = navigationHorizontal

	msgs := locales.In("zh-Hans")
	tt := ui.NewTester(d.view, 1920, 1080)
	tt.Frame()

	for _, label := range []string{msgs.GuiSession(), msgs.GuiSignals(), msgs.Energy(), msgs.ChargeShort(), msgs.GuiDuration()} {
		if !tt.HasText(label) {
			t.Fatalf("expected 2K overview to show %q", label)
		}
	}
}

func TestDevicePageAndTopSwitcherPopover(t *testing.T) {
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.settings.Navigation = navigationHorizontal
	d.source = session.Source{Demo: true}

	msgs := locales.In("en")
	tt := ui.NewTester(d.view, 1120, 720)
	tt.Frame()

	// Clicking top status pill opens device switcher popover instead of navigating to Device page.
	if tt.HasText(msgs.Live()) {
		t.Fatal("did not expect redundant Live badge in overview")
	}
	if err := tt.Click(msgs.SimulatedMeter()); err != nil {
		t.Fatalf("click status pill: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	d.work.Wait()
	tt.Frame()
	if !d.deviceMenuOpen {
		t.Fatal("expected clicking top status pill to open device switcher popover")
	}
	if d.page != pageOverview {
		t.Fatalf("expected page to remain Overview when opening top switcher, got %d", d.page)
	}

	// Discovered meters appear in popover and selecting one updates device and closes popover.
	d.meters = []device.Info{
		{ID: "k2-1", Name: "WITRN K2", Driver: "witrn-k2", Transport: "hid", Serial: "K2-001", Path: "/dev/hidraw0"},
		{ID: "km-2", Name: "KM003C", Driver: "km003c", Transport: "usb", Serial: "KM-002"},
	}
	tt.Frame()
	if !tt.HasText("K2-001") || !tt.HasText("KM-002") {
		t.Fatal("expected discovered devices inside top switcher popover")
	}

	if err := tt.Click("KM003C · KM-002"); err != nil {
		t.Fatalf("click KM003C in switcher: %v", err)
	}
	tt.Advance(250 * time.Millisecond)
	if d.deviceMenuOpen {
		t.Fatal("expected selecting a device to close switcher popover")
	}
	if d.source.Device != "km-2" {
		t.Fatalf("expected source.Device to switch to km-2, got %q", d.source.Device)
	}

	// Device page renders cleanly without Live badge or duplicate driver/transport row.
	d.goPage(pageDevice)
	tt.Frame()
	if tt.HasText(msgs.Live()) {
		t.Fatal("did not expect Live badge on Device page")
	}
	if tt.HasText(msgs.DriverTransport()) {
		t.Fatal("did not expect duplicate Driver / Transport row in Device info card")
	}
	if !tt.HasText(msgs.GuiDiscoveredDevices()) {
		t.Fatalf("expected Discovered devices heading %q on Device page", msgs.GuiDiscoveredDevices())
	}
	if devInfo, ok := tt.Find(msgs.GuiDeviceInfo()); !ok {
		t.Fatalf("expected inline Device info tab in top-bar Device mode")
	} else if d.settings.LiquidGlass && devInfo.Y < 44 {
		t.Fatalf("expected inline Device info tab below floating header (>=44), got Y=%v", devInfo.Y)
	}
	if _, ok := tt.Find(msgs.GuiConnectionLog()); !ok {
		t.Fatalf("expected inline Connection log tab in top-bar Device mode")
	}

	// Switch to full sidebar: inline SectionTabs remain in the page header instead of a nested sidebar submenu.
	d.settings.Navigation = navigationSidebar
	tt.Advance(350 * time.Millisecond)
	tt.Frame()
	if devInfo, ok := tt.Find(msgs.GuiDeviceInfo()); !ok {
		t.Fatal("expected inline Device info tab in sidebar Device mode")
	} else if devInfo.X <= kit.DesktopSidebarWidth {
		t.Fatalf("expected Device info in page content (>%v), not sidebar submenu, got X=%v", kit.DesktopSidebarWidth, devInfo.X)
	}
	if err := tt.Click(msgs.GuiConnectionLog()); err != nil {
		t.Fatalf("click inline Connection log tab: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if d.page != pageConnectionLog {
		t.Fatalf("expected Connection log page (%d), got %d", pageConnectionLog, d.page)
	}
	if err := tt.Click(msgs.GuiDeviceInfo()); err != nil {
		t.Fatalf("click inline Device info tab: %v", err)
	}
	tt.Advance(350 * time.Millisecond)
	if d.page != pageDevice {
		t.Fatalf("expected Device page (%d), got %d", pageDevice, d.page)
	}
}

func TestDurationFormatting(t *testing.T) {
	cases := []struct {
		seconds float64
		want    string
	}{
		{seconds: -2, want: "0s"},
		{seconds: 0, want: "0s"},
		{seconds: 45, want: "45s"},
		{seconds: 65, want: "1m5s"},
		{seconds: 8178, want: "2h16m18s"},
		{seconds: 30708, want: "8h31m48s"},
	}
	for _, tc := range cases {
		if got := formatDuration(tc.seconds); got != tc.want {
			t.Fatalf("formatDuration(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}

	sessionCases := []struct {
		seconds  float64
		wantVal  string
		wantUnit string
	}{
		{seconds: 12.4, wantVal: "12.4", wantUnit: "s"},
		{seconds: 59.9, wantVal: "59.9", wantUnit: "s"},
		{seconds: 60, wantVal: "1m0s", wantUnit: ""},
		{seconds: 8178, wantVal: "2h16m18s", wantUnit: ""},
	}
	for _, tc := range sessionCases {
		gotVal, gotUnit := formatSessionDuration(tc.seconds)
		if gotVal != tc.wantVal || gotUnit != tc.wantUnit {
			t.Fatalf("formatSessionDuration(%v) = (%q, %q), want (%q, %q)", tc.seconds, gotVal, gotUnit, tc.wantVal, tc.wantUnit)
		}
	}
}

func TestDesktopSelfBuiltFilePicker(t *testing.T) {
	recDir := t.TempDir()
	capturePath := filepath.Join(recDir, "sample.pd.jsonl")
	pkt := pd.Packet{
		Sequence:   1,
		Timestamp:  time.Now(),
		Elapsed:    0.1,
		SOP:        "SOP",
		MessageHex: "6110",
	}
	if err := recording.ExportCapture(capturePath, []pd.Packet{pkt}); err != nil {
		t.Fatalf("export fixture capture: %v", err)
	}
	csvPath := filepath.Join(recDir, "report.csv")
	if err := os.WriteFile(csvPath, []byte("elapsed_s,power_w\n0.1,5.0\n"), 0o600); err != nil {
		t.Fatalf("write fixture csv: %v", err)
	}
	hiddenCapture := filepath.Join(recDir, ".secret.pd.jsonl")
	if err := os.WriteFile(hiddenCapture, nil, 0o600); err != nil {
		t.Fatalf("write hidden file: %v", err)
	}
	subDir := filepath.Join(recDir, "exports")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir exports: %v", err)
	}

	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.dir = recDir
	d.goPage(pageRecords)

	msgs := locales.In("en")
	mainTester := ui.NewTester(d.view, 1120, 720)
	mainTester.Frame()

	// Clicking Open Capture on desktop opens our self-built picker rather than system dialog.
	if err := mainTester.Click(msgs.GuiOpenCapture()); err != nil {
		t.Fatalf("click open capture: %v", err)
	}
	d.work.Wait()

	pickerTester := ui.NewTester(d.pickerView, 760, 560)
	pickerTester.Frame()
	if !d.picker.data.Open || d.picker.data.Kind != pickerOpenCapture {
		t.Fatalf("expected open capture picker to be active, got open=%v kind=%d", d.picker.data.Open, d.picker.data.Kind)
	}
	if !pickerTester.HasText("sample.pd.jsonl") || !pickerTester.HasText("exports") {
		t.Fatal("expected capture file and subfolder to appear in picker list")
	}
	if pickerTester.HasText("report.csv") {
		t.Fatal("did not expect .csv files in PD capture picker")
	}
	if pickerTester.HasText(".secret.pd.jsonl") {
		t.Fatal("did not expect dotfile when ShowHidden is false")
	}

	// Selecting capture file and confirming loads offline capture into PD page and closes picker.
	if err := pickerTester.Click("sample.pd.jsonl"); err != nil {
		t.Fatalf("select capture file: %v", err)
	}
	pickerTester.Frame()
	if d.picker.data.Path != "sample.pd.jsonl" {
		t.Fatalf("expected filename input to update, got %q", d.picker.data.Path)
	}
	if err := pickerTester.Click(msgs.GuiOpen()); err != nil {
		t.Fatalf("click Open button: %v", err)
	}
	d.work.Wait()
	mainTester.Frame()
	if d.picker.data.Open {
		t.Fatal("expected picker to close after opening capture")
	}
	if d.page != pagePD || d.packets.offline == nil || d.packets.path != capturePath {
		t.Fatalf("expected PD page with loaded capture %q, got page=%d offline=%v path=%q", capturePath, d.page, d.packets.offline != nil, d.packets.path)
	}

	// Save Copy mode preserves filename across directory navigation and guards overwrite.
	d.openSaveCopyPicker(msgs, csvPath)
	d.work.Wait()
	pickerTester.Frame()
	d.browsePickerDirectory(recDir)
	d.work.Wait()
	pickerTester.Frame()
	d.browsePickerDirectory(subDir)
	d.work.Wait()
	pickerTester.Frame()

	if d.picker.data.Path != "report.csv" {
		t.Fatalf("expected save filename preserved across navigation, got %q", d.picker.data.Path)
	}
	if err := pickerTester.Click(msgs.GuiSave()); err != nil {
		t.Fatalf("click Save: %v", err)
	}
	d.work.Wait()
	pickerTester.Frame()

	destPath := filepath.Join(subDir, "report.csv")
	copied, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("expected saved copy at %q: %v", destPath, err)
	}
	if string(copied) != "elapsed_s,power_w\n0.1,5.0\n" {
		t.Fatalf("unexpected copied content %q", string(copied))
	}

	// Re-open Save Copy on existing target: first Save asks for Overwrite, second replaces cleanly.
	if err := os.WriteFile(csvPath, []byte("elapsed_s,power_w\n0.2,9.5\n"), 0o600); err != nil {
		t.Fatalf("update fixture csv: %v", err)
	}
	d.openSaveCopyPicker(msgs, csvPath)
	d.work.Wait()
	pickerTester.Frame()
	d.browsePickerDirectory(subDir)
	d.work.Wait()
	pickerTester.Frame()

	if err := pickerTester.Click(msgs.GuiSave()); err != nil {
		t.Fatalf("first Save on existing file: %v", err)
	}
	pickerTester.Frame()
	if d.picker.data.OverwriteConfirm != destPath || !pickerTester.HasText(msgs.GuiFileExists()) {
		t.Fatalf("expected overwrite confirmation prompt for %q, got %q", destPath, d.picker.data.OverwriteConfirm)
	}
	if err := pickerTester.Click(msgs.GuiOverwrite()); err != nil {
		t.Fatalf("click Overwrite: %v", err)
	}
	d.work.Wait()
	pickerTester.Frame()

	updated, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if string(updated) != "elapsed_s,power_w\n0.2,9.5\n" {
		t.Fatalf("expected overwritten content, got %q", string(updated))
	}
}

func TestTrendAxisStartsFromZero(t *testing.T) {
	pos := trendAxis(16.2, 17.8)
	if pos.Lo != 0 || pos.Hi < 17.8 {
		t.Fatalf("expected positive chart axis to start at 0 and cover high, got [%v, %v]", pos.Lo, pos.Hi)
	}
	ticks := pos.Ticks(3)
	if len(ticks) < 2 || ticks[0].Value != 0 {
		t.Fatalf("expected first tick at 0, got %+v", ticks)
	}

	idle := trendAxis(0, 0)
	if idle.Lo != 0 || idle.Hi <= 0 {
		t.Fatalf("expected zero idle axis [0, >0], got [%v, %v]", idle.Lo, idle.Hi)
	}

	neg := trendAxis(-2.4, -1.8)
	if neg.Hi != 0 || neg.Lo >= -2.4 {
		t.Fatalf("expected negative axis to anchor 0 at top, got [%v, %v]", neg.Lo, neg.Hi)
	}

	mixed := trendAxis(-1.2, 2.5)
	if mixed.Lo >= -1.2 || mixed.Hi <= 2.5 {
		t.Fatalf("expected bidirectional axis to cover [-1.2, 2.5], got [%v, %v]", mixed.Lo, mixed.Hi)
	}
}

func TestConnectionLogTextAndFormatting(t *testing.T) {
	en := locales.In("en")
	if got := en.DiagnosticField("os", "darwin"); got != "OS: macOS" {
		t.Fatalf("expected darwin formatted as macOS, got %q", got)
	}
	if got := en.DiagnosticField("transport", "usb"); got != "Transport: USB" {
		t.Fatalf("expected usb formatted as USB, got %q", got)
	}
	zh := locales.In("zh-Hans")
	if got := zh.DiagnosticField("transport", "hid"); got != "传输方式\uff1aHID" {
		t.Fatalf("expected hid formatted as HID, got %q", got)
	}

	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	buf := &diagnostics.Buffer{}
	d.logs = buf

	tt := ui.NewTester(d.view, 900, 680)
	d.goPage(pageConnectionLog)
	tt.Frame()
	if !tt.HasText(en.GuiNoConnectionLog()) {
		t.Fatalf("expected empty connection log state %q", en.GuiNoConnectionLog())
	}

	logger := slog.New(buf.Handler())
	logger.Info(diagnostics.AppStarted, "os", "darwin", "arch", "arm64")
	logger.Debug(diagnostics.DriverScanFinished, "driver", "witrn-k2", "found", 1)
	logger.Warn(diagnostics.ConnectionFailed, "stage", "open", "transport", "usb", "kind", "timeout", "error", "deadline exceeded")
	tt.Frame()
	if !tt.HasText(en.LogAppStarted()) || !tt.HasText("OS: macOS") || !tt.HasText(en.LogErrorTimeout()) || tt.HasText(en.LogDriverScanFinished()) {
		t.Fatal("expected formatted diagnostic entries and normalized OS/reason text")
	}
	if err := tt.Click(en.LogCopyEntry()); err != nil {
		t.Fatalf("click copy log entry: %v", err)
	}
	tt.Frame()
	copied := tt.Clipboard()
	if !strings.Contains(copied, en.LogConnectionFailed()) || !strings.Contains(copied, en.LogErrorTimeout()) ||
		!strings.Contains(copied, "deadline exceeded") || strings.Contains(copied, en.LogAppStarted()) {
		t.Fatalf("expected only the warning entry in the clipboard, got %q", copied)
	}
	if !tt.HasText(en.GuiCopied()) {
		t.Fatal("expected Copied toast after copying the entry")
	}
	for _, selection := range []struct {
		level slog.Level
		label string
		text  string
	}{
		{slog.LevelDebug, en.LogFilterDebug(), en.LogDriverScanFinished()},
		{slog.LevelError, en.LogFilterError(), en.LogEmptyLevel()},
		{slog.LevelWarn, en.LogFilterWarning(), en.LogConnectionFailed()},
	} {
		if err := tt.Click(en.LogLevelFilter(en.DiagnosticLevel(d.logLevel))); err != nil {
			t.Fatalf("open log level menu: %v", err)
		}
		tt.Advance(350 * time.Millisecond)
		if err := tt.Click(selection.label); err != nil {
			t.Fatalf("choose %s log level: %v", selection.level, err)
		}
		tt.Frame()
		if d.logLevel != selection.level || !tt.HasText(selection.text) {
			t.Fatalf("expected %s log filter to show %q", selection.level, selection.text)
		}
	}
}

func TestConnectionLogRetention(t *testing.T) {
	buffer := &diagnostics.Buffer{}
	logger := slog.New(buffer.Handler())
	levels := [...]slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn}
	for index := range 900 {
		logger.Log(t.Context(), levels[index%len(levels)], diagnostics.AppStarted, "driver", "test")
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

func TestSettingsRowAlignmentAndSmoothPeak(t *testing.T) {
	// 1. Verify SettingRow keeps single-line labels centered inside MinHeight(52) instead of pinned to top.
	d, cleanup := newTestDashboard(t, true)
	defer cleanup()
	d.language = "zh-Hans"
	d.settings.Language = "zh-Hans"
	msgs := locales.In("zh-Hans")

	tt := ui.NewTester(d.view, 390, 844)
	d.goPage(pageSettings)
	for range 20 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	langRect, okLang := tt.Find(msgs.GuiLanguage())
	smoothRect, okSmooth := tt.Find(msgs.GuiSmoothChart())
	if !okLang || !okSmooth {
		t.Fatalf("expected Language and Smooth curves rows in settings (lang=%v smooth=%v)", okLang, okSmooth)
	}
	// Language OptionRow has MinHeight(52); Smooth curves SettingRow sits 1px divider below it (also MinHeight(52)).
	// Verify vertical centers stay spaced cleanly by ~53dp (one row height + 1px divider), not shifted upward.
	langCenter := langRect.Y + langRect.H/2
	smoothCenter := smoothRect.Y + smoothRect.H/2
	centerDelta := smoothCenter - langCenter
	if centerDelta < 50 || centerDelta > 56 {
		t.Fatalf("expected SettingRow label center ~53dp below OptionRow center, got %.1fdp (lang=%+v smooth=%+v)", centerDelta, langRect, smoothRect)
	}
	if math.Abs(float64(langRect.X-smoothRect.X)) > 1 {
		t.Fatalf("expected IconSettingRow label X to align with OptionRow label X, got lang=%.1f smooth=%.1f", langRect.X, smoothRect.X)
	}

	// 2. Verify live >60s chart renders relative time ticks ("现在", "−1m", "−30s") and consistent smoothed peak/endpoint.
	samples := make([]device.Sample, 0, 1801)
	for i := 0; i <= 1800; i++ {
		sec := float64(i) * 0.05
		cur := 1.8 + 0.1*math.Sin(sec*4)
		if sec >= 74 && sec <= 76 {
			cur = 2.6
		}
		samples = append(samples, device.Sample{
			Elapsed: sec,
			Voltage: 9.0,
			Current: cur,
			Power:   9.0 * cur,
		})
	}
	ds, cleanupS := newTestDashboardWithSamples(t, samples)
	defer cleanupS()
	ds.language = "zh-Hans"
	ds.settings.Language = "zh-Hans"
	ds.settings.SmoothChart = true
	ts := ui.NewTester(ds.view, 390, 844)
	ts.Frame()
	if gotL, gotM := formatRelTime(ds.chartGeometry.key.start-90), formatRelTime((ds.chartGeometry.key.start+ds.chartGeometry.key.end)/2-90); gotL != "−1m" || gotM != "−30s" {
		t.Fatalf("expected relative live window ticks (−1m, −30s), got (%q, %q)", gotL, gotM)
	}
	if ds.chartGeometry.key.end > 90.05 {
		t.Fatalf("expected live >60s chart to stay anchored at latest timestamp 90, got end=%v", ds.chartGeometry.key.end)
	}
}

func TestLiquidGlassAndMobileStandardBottomBar(t *testing.T) {
	d, cleanup := newTestDashboard(t, true)
	defer cleanup()
	d.language = "zh-Hans"
	d.settings.Language = "zh-Hans"
	msgs := locales.In("zh-Hans")

	tt := ui.NewTester(d.view, 390, 844)
	tt.SetScale(2)
	tt.SetInsets(44, 0, 34, 0)
	d.goPage(pageSettings)
	for range 20 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}

	// Enable Liquid Glass and switch bottom bar to Standard style via Settings controls.
	glassSwitch, ok := tt.AccessByRole(ui.RoleSwitch, msgs.GuiLiquidGlass())
	if !ok {
		t.Fatal("missing Liquid Glass switch in mobile settings")
	}
	tt.ClickAt(glassSwitch.Bounds.X+glassSwitch.Bounds.W/2, glassSwitch.Bounds.Y+glassSwitch.Bounds.H/2)
	tt.Frame()
	if err := tt.Click(msgs.GuiBottomBarStandard()); err != nil {
		t.Fatalf("switch mobile bottom bar to Standard: %v", err)
	}
	for range 20 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}

	var restored dashboard
	restored.mobile = true
	restored.loadSettings("zh-Hans")
	if !d.settings.LiquidGlass || d.settings.BottomBarStyle != bottomBarStandard || restored.settings.BottomBarStyle != bottomBarStandard {
		t.Fatalf("expected LiquidGlass=true and bottomBarStandard persisted, got glass=%v style=%q restored=%q",
			d.settings.LiquidGlass, d.settings.BottomBarStyle, restored.settings.BottomBarStyle)
	}

	// Standard style shows all 4 equal-width tabs with labels above the 34dp safe area.
	var tabBounds []ui.Rect
	for _, label := range []string{msgs.GuiOverview(), msgs.PagePd(), msgs.GuiRecords(), msgs.GuiMore()} {
		acc, ok := tt.AccessByRole(ui.RoleTab, label)
		if !ok {
			t.Fatalf("missing Standard bottom bar tab %q", label)
		}
		if acc.Bounds.Y+acc.Bounds.H > 844-34+1 {
			t.Fatalf("Standard bottom bar tab %q overlaps safe area: bottom=%.1f", label, acc.Bounds.Y+acc.Bounds.H)
		}
		if _, hasText := tt.Find(label); !hasText {
			t.Fatalf("expected Standard bottom bar to show text for tab %q", label)
		}
		tabBounds = append(tabBounds, acc.Bounds)
	}
	for i := 1; i < len(tabBounds); i++ {
		if math.Abs(float64(tabBounds[i].W-tabBounds[0].W)) > 1.5 {
			t.Fatalf("expected equal-width Standard bottom bar tabs, got tab[0]=%.1f tab[%d]=%.1f", tabBounds[0].W, i, tabBounds[i].W)
		}
	}

	// Scrubbing horizontally across the Standard bottom bar switches destination cleanly.
	startX := tabBounds[0].X + tabBounds[0].W/2
	startY := tabBounds[0].Y + tabBounds[0].H/2
	endX := tabBounds[3].X + tabBounds[3].W/2
	tt.FingerDown(startX, startY)
	for step := 1; step <= 6; step++ {
		frac := float32(step) / 6
		tt.FingerMove(startX+(endX-startX)*frac, startY)
		tt.Advance(25 * time.Millisecond)
		tt.Frame()
	}
	tt.FingerUp(endX, startY)
	for range 12 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if d.page != pageMore {
		t.Fatalf("expected drag across Standard bottom bar to reach More (page %d), got %d", pageMore, d.page)
	}

	// Return to Overview, scroll partway so content passes under floating top/bottom glass bars, and verify rendering.
	if err := tt.Click(msgs.GuiOverview()); err != nil {
		t.Fatalf("tap Overview on Standard bottom bar: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if previewDir := os.Getenv("WATTSCOPE_GLASS_PREVIEW"); previewDir != "" {
		_ = os.MkdirAll(previewDir, 0o755)
		save := func(name string, tester *ui.Tester) {
			f, err := os.Create(filepath.Join(previewDir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			_ = png.Encode(f, tester.Image())
			_ = f.Close()
		}
		// 1. Mobile Light Standard Bottom Bar + Liquid Glass (Overview, scrolled slightly, and Settings)
		tt.Scroll(195, 400, 0, -140)
		for range 10 {
			tt.Frame()
			tt.Advance(25 * time.Millisecond)
		}
		save("mobile-glass-overview-light", tt)
		d.settings.Theme = mygo.ThemeDark
		for range 10 {
			tt.Frame()
			tt.Advance(25 * time.Millisecond)
		}
		save("mobile-glass-overview-dark", tt)
		d.settings.Theme = mygo.ThemeLight
		d.goPage(pageSettings)
		for range 15 {
			tt.Frame()
			tt.Advance(25 * time.Millisecond)
		}
		save("mobile-glass-settings-light", tt)

		// 2. Desktop Light & Dark Liquid Glass (Overview + open switcher popover)
		dd, cleanupD := newTestDashboard(t, false)
		defer cleanupD()
		dd.language = "zh-Hans"
		dd.settings.Language = "zh-Hans"
		dd.settings.LiquidGlass = true
		dd.settings.Navigation = navigationHorizontal
		td := ui.NewTester(dd.view, 1120, 720)
		td.SetScale(2)
		for range 15 {
			td.Frame()
			td.Advance(25 * time.Millisecond)
		}
		td.Scroll(560, 400, 0, -110)
		for range 10 {
			td.Frame()
			td.Advance(25 * time.Millisecond)
		}
		save("desktop-glass-overview-light", td)
		dd.settings.Theme = mygo.ThemeDark
		for range 10 {
			td.Frame()
			td.Advance(25 * time.Millisecond)
		}
		save("desktop-glass-overview-dark", td)
	}
}

func TestThemeCircularRevealAndLocaleAnimations(t *testing.T) {
	// 1. Verify circular theme reveal radiates outward from the clicked theme button.
	d, cleanup := newTestDashboard(t, false)
	defer cleanup()
	d.language = "en"
	d.settings.Language = "en"
	d.settings.Theme = mygo.ThemeLight
	d.settings.LiquidGlass = true
	msgs := locales.In("en")

	tt := ui.NewTester(d.view, 1120, 720)
	tt.SetScale(2)
	for range 10 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}

	themeBtn, ok := tt.AccessByRole(ui.RoleButton, msgs.GuiDark())
	if !ok {
		t.Fatalf("missing desktop theme toggle button %q", msgs.GuiDark())
	}
	tt.ClickAt(themeBtn.Bounds.X+themeBtn.Bounds.W/2, themeBtn.Bounds.Y+themeBtn.Bounds.H/2)
	tt.Frame()

	tt.Advance(140 * time.Millisecond)
	tt.Frame()
	if previewDir := os.Getenv("WATTSCOPE_GLASS_PREVIEW"); previewDir != "" {
		_ = os.MkdirAll(previewDir, 0o755)
		f, err := os.Create(filepath.Join(previewDir, "theme-reveal-mid.png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, tt.Image())
		_ = f.Close()
	}

	img := tt.Image()
	// Image is scaled 2x: check near top-right (inside expanding dark circle) vs bottom-left (still light).
	inCircle := img.RGBAAt(2000, 140)
	outCircle := img.RGBAAt(80, 1360)
	if inCircle.R >= 60 || outCircle.R <= 180 {
		t.Fatalf("expected circular reveal mid-flight (dark inside circle near button, light far away), got in=%+v out=%+v", inCircle, outCircle)
	}

	// Complete the animation and verify the far corner transitions cleanly to dark.
	tt.Advance(760 * time.Millisecond)
	tt.Frame()
	doneCorner := tt.Image().RGBAAt(80, 1360)
	if doneCorner.R >= 60 {
		t.Fatalf("expected far corner to finish revealing dark theme, got %+v", doneCorner)
	}

	// 1b. Verify top-navigation bar switches theme colors simultaneously inside the reveal boundary.
	d.settings.Navigation = navigationHorizontal
	for range 12 {
		tt.Advance(30 * time.Millisecond)
		tt.Frame()
	}
	themeBtn, ok = tt.AccessByRole(ui.RoleButton, msgs.GuiLight())
	if !ok {
		t.Fatalf("missing desktop theme toggle button %q in top nav", msgs.GuiLight())
	}
	tt.ClickAt(themeBtn.Bounds.X+themeBtn.Bounds.W/2, themeBtn.Bounds.Y+themeBtn.Bounds.H/2)
	tt.Frame()
	tt.Advance(280 * time.Millisecond)
	tt.Frame()
	if previewDir := os.Getenv("WATTSCOPE_GLASS_PREVIEW"); previewDir != "" {
		f, err := os.Create(filepath.Join(previewDir, "theme-reveal-top-mid.png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, tt.Image())
		_ = f.Close()
	}
	topImg := tt.Image()
	navTabBtn, ok := tt.Find(msgs.GuiSettings())
	if !ok {
		t.Fatalf("missing Settings tab in top nav")
	}
	topNavSample := topImg.RGBAAt(int((navTabBtn.X+navTabBtn.W/2)*2), int((navTabBtn.Y+navTabBtn.H/2)*2))
	if topNavSample.R < 150 && topNavSample.G < 150 && topNavSample.B < 150 {
		t.Fatalf("expected top navigation bar inside reveal boundary to switch toward light theme immediately, got %+v", topNavSample)
	}
	tt.Advance(700 * time.Millisecond)
	tt.Frame()

	// 1c. Verify sidebar switches theme colors simultaneously once the wave crosses it.
	d.settings.Navigation = navigationSidebar
	for range 12 {
		tt.Advance(30 * time.Millisecond)
		tt.Frame()
	}
	themeBtn, ok = tt.AccessByRole(ui.RoleButton, msgs.GuiDark())
	if !ok {
		t.Fatalf("missing desktop theme toggle button %q in sidebar mode", msgs.GuiDark())
	}
	tt.ClickAt(themeBtn.Bounds.X+themeBtn.Bounds.W/2, themeBtn.Bounds.Y+themeBtn.Bounds.H/2)
	tt.Frame()
	tt.Advance(460 * time.Millisecond)
	tt.Frame()
	if previewDir := os.Getenv("WATTSCOPE_GLASS_PREVIEW"); previewDir != "" {
		f, err := os.Create(filepath.Join(previewDir, "theme-reveal-side-mid.png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, tt.Image())
		_ = f.Close()
	}
	sideImg := tt.Image()
	sideSettingsBtn, ok := tt.Find(msgs.GuiSettings())
	if !ok {
		t.Fatalf("missing Settings tab in sidebar")
	}
	sideNavSample := sideImg.RGBAAt(int((sideSettingsBtn.X+100)*2), int((sideSettingsBtn.Y+sideSettingsBtn.H/2)*2))
	if sideNavSample.R >= 80 || sideNavSample.G >= 80 || sideNavSample.B >= 80 {
		t.Fatalf("expected sidebar inside reveal boundary to switch to dark theme immediately, got %+v", sideNavSample)
	}
	tt.Advance(500 * time.Millisecond)
	tt.Frame()

	themeBtn, ok = tt.AccessByRole(ui.RoleButton, msgs.GuiLight())
	if !ok {
		t.Fatal("missing theme toggle for consecutive clicks")
	}
	clickX := themeBtn.Bounds.X + themeBtn.Bounds.W/2
	clickY := themeBtn.Bounds.Y + themeBtn.Bounds.H/2
	tt.ClickAt(clickX, clickY)
	if d.settings.Theme != mygo.ThemeLight {
		t.Fatalf("expected first click to apply light immediately, got %v", d.settings.Theme)
	}
	tt.Advance(180 * time.Millisecond)
	if near := tt.Image().RGBAAt(1800, 260); near.R <= 180 {
		t.Fatalf("expected first light wave near button, got %+v", near)
	}
	tt.ClickAt(clickX, clickY)
	if d.settings.Theme != mygo.ThemeDark {
		t.Fatalf("expected second click to apply dark immediately, got %v", d.settings.Theme)
	}
	if near := tt.Image().RGBAAt(1800, 260); near.R <= 180 {
		t.Fatalf("expected second click to preserve the already revealed light area, got %+v", near)
	}
	tt.Advance(120 * time.Millisecond)
	overlap := tt.Image()
	if near, middle, far := overlap.RGBAAt(1800, 260), overlap.RGBAAt(1100, 1040), overlap.RGBAAt(80, 1360); near.R >= 60 || middle.R <= 180 || far.R >= 60 {
		t.Fatalf("expected both waves to keep advancing, got near=%+v middle=%+v far=%+v", near, middle, far)
	}
	tt.ClickAt(clickX, clickY)
	if d.settings.Theme != mygo.ThemeLight {
		t.Fatalf("expected third click to apply light immediately, got %v", d.settings.Theme)
	}
	tt.Advance(620 * time.Millisecond)
	if corner := tt.Image().RGBAAt(80, 1360); corner.R <= 180 {
		t.Fatalf("expected final light reveal to complete, got %+v", corner)
	}

	// Preview Device page in both top-nav and sidebar Liquid Glass modes when requested.
	if previewDir := os.Getenv("WATTSCOPE_GLASS_PREVIEW"); previewDir != "" {
		d.goPage(pageDevice)
		for range 12 {
			tt.Advance(30 * time.Millisecond)
			tt.Frame()
		}
		if f, err := os.Create(filepath.Join(previewDir, "device-top-glass.png")); err == nil {
			_ = png.Encode(f, tt.Image())
			_ = f.Close()
		}
		d.settings.Navigation = navigationSidebar
		for range 12 {
			tt.Advance(30 * time.Millisecond)
			tt.Frame()
		}
		if f, err := os.Create(filepath.Join(previewDir, "device-side-glass.png")); err == nil {
			_ = png.Encode(f, tt.Image())
			_ = f.Close()
		}
	}
	d.settings.Navigation = navigationSidebar

	// 2. Verify language switch animates text and component sizes smoothly instead of snapping.
	d.goPage(pageSettings)
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if err := tt.Click(msgs.GuiNavigation()); err != nil {
		// Ensure Settings page rendered; check segmented option label before language switch.
		t.Fatalf("missing Navigation setting before language switch: %v", err)
	}
	navChipBefore, ok := tt.Find(msgs.GuiSideNavigation())
	if !ok {
		t.Fatalf("missing %q segment before language switch", msgs.GuiSideNavigation())
	}
	if err := tt.Click(msgs.GuiLanguage() + " · English"); err != nil {
		t.Fatalf("open Language menu: %v", err)
	}
	for range 15 {
		tt.Frame()
		tt.Advance(25 * time.Millisecond)
	}
	if err := tt.Click("简体中文"); err != nil {
		t.Fatalf("choose 简体中文: %v", err)
	}
	tt.Advance(35 * time.Millisecond)
	tt.Frame()
	zhMsgs := locales.In("zh-Hans")
	navChipMid, okMid := tt.Find(zhMsgs.GuiSideNavigation())
	for range 20 {
		tt.Advance(25 * time.Millisecond)
		tt.Frame()
	}
	navChipAfter, okAfter := tt.Find(zhMsgs.GuiSideNavigation())
	if !okMid || !okAfter {
		t.Fatalf("expected Chinese navigation segment (mid=%v after=%v)", okMid, okAfter)
	}
	if math.Abs(float64(navChipAfter.W-navChipBefore.W)) < 2 {
		t.Fatalf("expected segment text width to change between English (%.1f) and Chinese (%.1f)", navChipBefore.W, navChipAfter.W)
	}
	if !(navChipMid.W < navChipBefore.W && navChipMid.W > navChipAfter.W) &&
		!(navChipMid.W > navChipBefore.W && navChipMid.W < navChipAfter.W) &&
		math.Abs(float64(navChipMid.W-navChipAfter.W)) < 0.5 && math.Abs(float64(navChipMid.X-navChipAfter.X)) < 0.5 {
		t.Fatalf("expected intermediate animated layout/size during language switch, got before=%+v mid=%+v after=%+v", navChipBefore, navChipMid, navChipAfter)
	}
}
