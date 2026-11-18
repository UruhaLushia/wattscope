package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
)

func sampleFixture() (backend.Snapshot, View) {
	now := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	var hist []device.Sample
	for i := 0; i < 40; i++ {
		t := float64(i) * 0.5
		hist = append(hist, device.Sample{
			Timestamp:        now.Add(time.Duration(t*1000) * time.Millisecond),
			Elapsed:          t,
			Voltage:          9.01 + 0.02*float64(i%5),
			Current:          1.85 + 0.1*float64(i%4),
			Power:            16.67 + 0.8*float64(i%4),
			CurrentDirection: device.CurrentPlugToReceptacle,
		})
	}
	temp := 31.4
	hist[len(hist)-1].Temperature = &temp
	latest := hist[len(hist)-1]
	cc1 := 5.1
	latest.CC1 = &cc1
	recMin := 0.01
	latest.RecordMinCurrent = &recMin

	snap := backend.Snapshot{
		State: backend.Streaming,
		Device: device.Info{
			ID:           "k2-demo",
			Driver:       "witrn-k2",
			Path:         "/dev/hidraw0",
			Name:         "WITRN K2",
			Transport:    "hid",
			Serial:       "K2-00123",
			Capabilities: []string{"pd_capture"},
		},
		Latest:  &latest,
		History: hist,
		Statistics: backend.Statistics{
			MeasuredSeconds:  20,
			WattHours:        0.1389,
			AmpHours:         0.0154,
			AbsoluteAmpHours: 0.0154,
			Samples:          40,
		},
		Connections:    1,
		CanCheckPDHost: true,
	}

	pkt1 := pd.Packet{
		Sequence: 1, Elapsed: 0.412, SOP: "SOP", PowerRole: "Source", DataRole: "DFP",
		Type: "Source_Capabilities", Header: 0x31a1, Revision: "3.0", MessageID: 0,
		CRC: "valid", MessageHex: "a1312c9101082cd102002cb10400",
		PDOs: []pd.PDO{
			{Position: 1, Kind: "Fixed", MaxV: 5, MaxA: 3, MaxW: 15, Raw: 0x0801912c},
			{Position: 2, Kind: "Fixed", MaxV: 9, MaxA: 3, MaxW: 27, Raw: 0x0002d12c},
		},
		Analysis: &pd.Analysis{
			CapabilitiesSequence: 1,
			SourceCapabilities:   &pd.SourceCapabilities{HeaderRevision: "3.0", PPS: true, FixedMaxW: 27},
		},
	}
	pkt2 := pd.Packet{
		Sequence: 2, Elapsed: 0.418, SOP: "SOP", PowerRole: "Sink", DataRole: "UFP",
		Type: "Request", Header: 0x1082, Revision: "3.0", MessageID: 0,
		CRC: "valid", MessageHex: "8210c8b00420",
		Request: &pd.Request{Position: 2, Kind: "Fixed", VoltageV: 9, OperatingA: 2, MaximumA: 2, Matched: true},
		Analysis: &pd.Analysis{
			CapabilitiesSequence: 1, RequestSequence: 2, Stage: "requested",
			SourceCapabilities: pkt1.Analysis.SourceCapabilities,
			Request:            &pd.Request{Position: 2, Kind: "Fixed", VoltageV: 9, OperatingA: 2, MaximumA: 2, Matched: true},
		},
	}

	view := View{
		Locale:         "en",
		Page:           PageOverview,
		Window:         time.Minute,
		RecordingDir:   "/tmp/wattscope/recordings",
		Now:            now,
		LastReceived:   now,
		Packets:        backend.PacketSnapshot{Packets: []pd.Packet{pkt1, pkt2}, Total: 2},
		SelectedPacket: 2,
	}
	return snap, view
}

func assertExactScreen(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("expected %d rows, got %d", height, len(lines))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("row %d: expected width %d, got %d (%q)", i, width, got, ansi.Strip(line))
		}
	}
}

func TestRenderDimensionsAndStates(t *testing.T) {
	snap, baseView := sampleFixture()
	sizes := [][2]int{
		{140, 36},
		{120, 32},
		{100, 28},
		{80, 24},
		{38, 11},
		{20, 6},
	}
	localesList := []string{"en", "zh-Hans"}
	pages := []Page{PageOverview, PagePD, PageRecordings, PageDevice}

	for _, locale := range localesList {
		for _, size := range sizes {
			w, h := size[0], size[1]
			for _, page := range pages {
				v := baseView
				v.Locale = locale
				v.Page = page
				out := Render(snap, w, h, v)
				assertExactScreen(t, out, w, h)
			}

			// Help, Details, PacketDetails, Filtering, Empty snapshot
			v := baseView
			v.Locale = locale
			v.Help = true
			assertExactScreen(t, Render(snap, w, h, v), w, h)

			v.Help = false
			v.Details = true
			assertExactScreen(t, Render(snap, w, h, v), w, h)

			v.Details = false
			v.Page = PagePD
			v.PacketDetails = true
			assertExactScreen(t, Render(snap, w, h, v), w, h)

			v.PacketDetails = false
			v.Filtering = true
			v.PacketFilter = "req"
			assertExactScreen(t, Render(snap, w, h, v), w, h)

			emptySnap := backend.Snapshot{State: backend.Waiting}
			emptyView := View{Locale: locale, Page: PageOverview, Now: baseView.Now}
			assertExactScreen(t, Render(emptySnap, w, h, emptyView), w, h)
		}
	}
}

func TestCopywritingAndIndicators(t *testing.T) {
	snap, v := sampleFixture()
	snap.Recording = backend.Recording{Active: true, Path: "/tmp/wattscope/test.csv", Samples: 12}
	v.Paused = true
	v.Locale = "en"

	overview := ansi.Strip(Render(snap, 120, 32, v))
	if !strings.Contains(overview, "REC") || !strings.Contains(overview, "HOLD") {
		t.Fatalf("expected REC and HOLD badges in workspace header, got:\n%s", overview)
	}
	if !strings.Contains(overview, "▸ 1 Power") {
		t.Fatalf("expected active metric marker on Power card")
	}

	// Negative sample current should also render as positive magnitude without '-' sign
	snapNeg := snap
	negLatest := *snap.Latest
	negLatest.Current = -2.15
	snapNeg.Latest = &negLatest
	negOverview := ansi.Strip(Render(snapNeg, 120, 32, v))
	if strings.Contains(negOverview, "-2.1500") {
		t.Fatalf("expected non-negative current display in TUI, got:\n%s", negOverview)
	}
	if !strings.Contains(negOverview, "2.1500") {
		t.Fatalf("expected 2.1500 current display in TUI, got:\n%s", negOverview)
	}

	v.Page = PageRecordings
	recPage := ansi.Strip(Render(snap, 120, 32, v))
	if !strings.Contains(recPage, "12 rows") {
		t.Fatalf("expected CSV row count on recordings page, got:\n%s", recPage)
	}
	if !strings.Contains(recPage, "No file started yet") {
		t.Fatalf("expected 'No file started yet' for inactive PD capture, got:\n%s", recPage)
	}

	v.Page = PageDevice
	devPage := ansi.Strip(Render(snap, 120, 32, v))
	for _, want := range []string{"K2-00123", "STORAGE", "METER TELEMETRY"} {
		if !strings.Contains(devPage, want) {
			t.Fatalf("expected %q in device page, got:\n%s", want, devPage)
		}
	}

	// Offline mode should omit live recording controls on Recordings page
	v.Page = PageRecordings
	v.SourcePath = "/tmp/capture.pd.jsonl"
	offlineRec := ansi.Strip(Render(snap, 120, 32, v))
	if strings.Contains(offlineRec, "r CSV · p PD capture") {
		t.Fatalf("offline recordings page should not show live recording controls")
	}
}

func TestMouseHitTestingMatchesLayout(t *testing.T) {
	snap, v := sampleFixture()
	m := model{
		opts:          Options{Locale: "en", Window: time.Minute, RecordingDir: "/tmp/rec", Capture: &v.Packets},
		view:          v,
		frozen:        snap,
		frozenPackets: v.Packets,
		width:         120,
		height:        32,
	}

	// Click sidebar F2 (USB PD) at x=5, y=5 (4 + 1)
	updated, _ := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.view.Page != PagePD {
		t.Fatalf("expected clicking sidebar item 2 to switch to PagePD, got %v", m.view.Page)
	}

	// Click GoodCRC filter toggle on PagePD (row 1 inside content -> y = workspaceContentTop + 1 = 5)
	filters := packetFilters(layout(m.width, m.height).mainWidth, m.view)
	crcX := layout(m.width, m.height).mainX + ansi.StringWidth(filters[0].label) + ansi.StringWidth(packetFilterGap) + 1
	updated, _ = m.Update(tea.MouseMsg{X: crcX, Y: workspaceContentTop + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if !m.view.HideGoodCRC {
		t.Fatalf("expected clicking GoodCRC filter chip at x=%d to toggle HideGoodCRC", crcX)
	}

	// Switch to narrow 80x24 (tab bar instead of sidebar) and click F1 Overview tab
	m.width, m.height = 80, 24
	updated, _ = m.Update(tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.view.Page != PageOverview {
		t.Fatalf("expected clicking F1 tab on narrow screen to switch to PageOverview, got %v", m.view.Page)
	}

	// Click second metric card (Current) on 80x24 Overview (workspaceContentTop = 4)
	l := layout(m.width, m.height)
	widths := metricCardWidths(l.mainWidth)
	card2X := l.mainX + widths[0] + 2
	updated, _ = m.Update(tea.MouseMsg{X: card2X, Y: workspaceContentTop + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.view.Metric != currentMetric {
		t.Fatalf("expected clicking card 2 to select currentMetric (%d), got %d", currentMetric, m.view.Metric)
	}
}
