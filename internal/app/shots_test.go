package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestShots writes screenshots of every page when WATTSCOPE_SHOTS names a directory.
func TestShots(t *testing.T) {
	dir := os.Getenv("WATTSCOPE_SHOTS")
	if dir == "" {
		t.Skip("set WATTSCOPE_SHOTS")
	}
	_ = os.MkdirAll(dir, 0o755)
	lang := os.Getenv("WATTSCOPE_SHOTS_LANG")
	if lang == "" {
		lang = "zh-Hans"
	}
	type shot struct {
		name   string
		mobile bool
		dark   bool
		nav    string
		w, h   int
		page   int
	}
	var shots []shot
	for _, dark := range []bool{false, true} {
		suffix := "light"
		if dark {
			suffix = "dark"
		}
		for page, name := range []string{"overview", "pd", "records", "device", "settings"} {
			shots = append(shots, shot{name + "-" + suffix, false, dark, navigationHorizontal, 1120, 720, page})
		}
		shots = append(shots, shot{"side-" + suffix, false, dark, navigationSidebar, 1120, 720, pageOverview})
		shots = append(shots, shot{"mobile-" + suffix, true, dark, navigationHorizontal, 390, 844, pageOverview})
		if !dark {
			shots = append(shots,
				shot{"mobile-records-light", true, false, navigationHorizontal, 390, 844, pageRecords},
				shot{"mobile-device-light", true, false, navigationHorizontal, 390, 844, pageDevice},
				shot{"mobile-settings-light", true, false, navigationHorizontal, 390, 844, pageSettings},
			)
		}
		shots = append(shots, shot{"narrow-" + suffix, false, dark, navigationHorizontal, 640, 600, pageOverview})
	}
	for _, s := range shots {
		d, cleanup := newTestDashboard(t, s.mobile)
		d.language = lang
		d.settings.Language = lang
		d.settings.Navigation = s.nav
		d.settings.Theme = mygo.ThemeLight
		if s.dark {
			d.settings.Theme = mygo.ThemeDark
		}
		d.page = s.page
		tt := ui.NewTester(d.view, s.w, s.h)
		tt.SetScale(2)
		if s.mobile {
			tt.SetInsets(44, 0, 34, 0)
		}
		tt.Frame()
		time.Sleep(1200 * time.Millisecond)
		tt.Advance(time.Second)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, s.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, tt.Image())
		f.Close()
		cleanup()
	}

	// Render mobile PD list and opened bottom Sheet drawer screenshots for visual check.
	{
		d, cleanup := newTestDashboard(t, true)
		d.language = lang
		d.settings.Language = lang
		d.settings.Theme = mygo.ThemeLight
		d.page = pagePD
		d.packets.offline = &backend.PacketSnapshot{
			Total: 3,
			Packets: []pd.Packet{
				{Sequence: 1, Elapsed: 0.012, SOP: "SOP", Type: "Source_Capabilities", PowerRole: "Source", DataRole: "DFP", Revision: "3.0", CRC: "valid", MessageHex: "a1112c910108", PDOs: []pd.PDO{{Position: 1, Kind: "Fixed", MinV: 5, MaxV: 5, MaxA: 3, MaxW: 15}, {Position: 2, Kind: "Fixed", MinV: 9, MaxV: 9, MaxA: 3, MaxW: 27}, {Position: 3, Kind: "Fixed", MinV: 20, MaxV: 20, MaxA: 5, MaxW: 100}}},
				{Sequence: 2, Elapsed: 0.018, SOP: "SOP", Type: "Request", PowerRole: "Sink", DataRole: "UFP", Revision: "3.0", CRC: "valid", MessageHex: "82102c910130"},
				{Sequence: 3, Elapsed: 0.025, SOP: "SOP", Type: "PS_RDY", PowerRole: "Source", DataRole: "DFP", Revision: "3.0", CRC: "valid", MessageHex: "6603"},
			},
		}
		tt := ui.NewTester(d.view, 390, 844)
		tt.SetScale(2)
		tt.SetInsets(44, 0, 34, 0)
		tt.Frame()
		tt.Advance(350 * time.Millisecond)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, "mobile-pd-light.png"))
		_ = png.Encode(f, tt.Image())
		f.Close()

		_ = tt.Click("#1 Source_Capabilities")
		for range 25 {
			tt.Frame()
			tt.Advance(25 * time.Millisecond)
		}
		f2, _ := os.Create(filepath.Join(dir, "mobile-pd-sheet-light.png"))
		_ = png.Encode(f2, tt.Image())
		f2.Close()
		cleanup()
	}

	shotDir := t.TempDir()
	_ = os.Mkdir(filepath.Join(shotDir, "exports"), 0o755)
	_ = os.WriteFile(filepath.Join(shotDir, "wattscope-20261118-150000.pd.jsonl"), nil, 0o600)
	_ = os.WriteFile(filepath.Join(shotDir, "pd-negotiation.pd.jsonl"), nil, 0o600)
	for _, dark := range []bool{false, true} {
		d, cleanup := newTestDashboard(t, false)
		d.language = lang
		d.settings.Language = lang
		d.settings.Theme = mygo.ThemeLight
		suffix := "light"
		if dark {
			d.settings.Theme = mygo.ThemeDark
			suffix = "dark"
		}
		msgs := locales.In(lang)
		if !dark {
			d.openFilePicker(filePickerData{Kind: pickerOpenCapture, Title: msgs.GuiOpenCapture(), Extensions: captureExtensions}, shotDir)
		} else {
			d.openFilePicker(filePickerData{Kind: pickerSaveCopy, Title: msgs.GuiSaveCopy(), Path: "wattscope-20261118.csv", SourcePath: filepath.Join(t.TempDir(), "src.csv")}, shotDir)
		}
		d.work.Wait()
		tt := ui.NewTester(d.pickerView, 760, 560)
		tt.SetScale(2)
		tt.Frame()
		_ = tt.Click("wattscope-20261118-150000.pd.jsonl")
		tt.Advance(250 * time.Millisecond)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, "picker-"+suffix+".png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, tt.Image())
		f.Close()
		cleanup()
	}
}
