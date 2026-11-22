package kit

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	RadiusS    float32 = 8
	RadiusM    float32 = 11
	RadiusL    float32 = 14
	RadiusXL   float32 = 18
	RadiusFull float32 = 999

	DefaultAccentHex = "#3e63dd"
)

var AccentPresets = []string{
	"#3e63dd", // Indigo
	"#0d9488", // Teal
	"#8e4ec6", // Violet
	"#e54666", // Ruby
	"#f76b15", // Orange
	"#30a46c", // Green
}

type Palette struct {
	Dark        bool
	LiquidGlass bool
	Canvas      ui.Color // page background
	Track       ui.Color // segmented and navigation track
	TrackOn     ui.Color // selected segment/tab accent pill on a track
	TrackOnInk  ui.Color // foreground ink over TrackOn
	CardShadow  ui.Color
	Inset       ui.Color
	Surface     ui.Color
	SurfaceHi   ui.Color
	Line        ui.Color
	LineSoft    ui.Color
	Text        ui.Color
	Muted       ui.Color
	Faint       ui.Color
	Hover       ui.Color
	Press       ui.Color
	Accent      ui.Color
	AccentHi    ui.Color
	AccentSoft  ui.Color
	AccentLine  ui.Color
	AccentInk   ui.Color
	AccentText  ui.Color
	Green       ui.Color
	GreenSoft   ui.Color
	Red         ui.Color
	RedSoft     ui.Color
	Warning     ui.Color
	WarningSoft ui.Color
	Shadow      ui.Color
	Backdrop    ui.Color
	Scrollbar   ui.Color
}

type themeCacheKey struct {
	dark   bool
	accent ui.Color
}

var (
	themeMu    sync.Mutex
	themeCache = map[themeCacheKey]struct {
		theme   *ui.Theme
		palette Palette
	}{}
)

func parseAccentHex(value string) ui.Color {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "#") || (len(value) != 7 && len(value) != 9) {
		return ui.Hex(DefaultAccentHex)
	}
	return ui.Hex(value)
}

func HexColor(c ui.Color) string {
	if c.A != 255 {
		return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func ResolveTheme(dark bool, accentHex string) (*ui.Theme, Palette) {
	accent := parseAccentHex(accentHex)
	key := themeCacheKey{dark: dark, accent: accent}
	themeMu.Lock()
	defer themeMu.Unlock()
	if cached, ok := themeCache[key]; ok {
		return cached.theme, cached.palette
	}
	p := buildPalette(dark, accent)
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
	} else {
		t = ui.LightTheme()
	}
	t.Background = p.Canvas
	t.Surface = p.Surface
	t.SurfaceHover = p.SurfaceHi
	t.SurfacePressed = p.Press
	t.Border = p.Line
	t.Text = p.Text
	t.TextMuted = p.Muted
	t.Accent = p.Accent
	t.AccentHover = p.AccentHi
	t.AccentPressed = p.Accent
	t.AccentText = p.AccentInk
	t.Success = p.Green
	t.Warning = p.Warning
	t.Danger = p.Red
	t.Selection = p.AccentSoft
	t.Focus = p.Accent
	t.Scrollbar = p.Scrollbar
	t.Radius = RadiusM
	t.Spacing = 4
	t.FontSize = 13

	themeCache[key] = struct {
		theme   *ui.Theme
		palette Palette
	}{theme: t, palette: p}
	return t, p
}

func buildPalette(dark bool, accent ui.Color) Palette {
	p := Palette{
		Dark:       dark,
		Accent:     accent,
		AccentHi:   accent.Mix(ui.Hex("#ffffff"), .14),
		AccentLine: accent.Alpha(.38),
		Backdrop:   ui.RGBA(8, 10, 14, .38),
	}
	p.AccentHi.A = accent.A
	if dark {
		p.Canvas = ui.Hex("#0b0c10").Mix(accent, .015)
		p.Inset = ui.Hex("#11141b").Mix(accent, .045)
		p.Surface = ui.Hex("#151820").Mix(accent, .025)
		p.SurfaceHi = ui.Hex("#1b1f29").Mix(accent, .055)
		p.Line = ui.Hex("#2a2e37")
		p.LineSoft = ui.RGBA(255, 255, 255, .07)
		p.Text = ui.Hex("#eceef2")
		p.Muted = ui.Hex("#959cab")
		p.Faint = p.Muted.Alpha(.5)
		p.Hover = accent.Alpha(.11)
		p.Press = accent.Alpha(.18)
		p.Track = ui.Hex("#181c25").Mix(accent, .07)
		p.TrackOn = accent.Mix(p.SurfaceHi, .24)
		p.Green = ui.Hex("#4cc38a")
		p.Red = ui.Hex("#f2555a")
		p.Warning = ui.Hex("#f5b544")
		p.Shadow = ui.RGBA(0, 0, 0, .42)
		p.CardShadow = ui.Transparent
		p.Scrollbar = ui.RGBA(255, 255, 255, .14)
		p.AccentSoft = accent.Alpha(.20)
	} else {
		p.Canvas = ui.Hex("#f6f8fb").Mix(accent, .02)
		p.Inset = ui.Hex("#f4f7fb").Mix(accent, .06)
		p.Surface = ui.Hex("#ffffff")
		p.SurfaceHi = ui.Hex("#f8fafc").Mix(accent, .04)
		p.Line = ui.Hex("#e1e5ed")
		p.LineSoft = ui.RGBA(17, 24, 39, .07)
		p.Text = ui.Hex("#15181e")
		p.Muted = ui.Hex("#5e6572")
		p.Faint = ui.RGBA(17, 24, 39, .08)
		p.Hover = accent.Alpha(.075)
		p.Press = accent.Alpha(.13)
		p.Track = ui.Hex("#f4f7fb").Mix(accent, .07)
		p.TrackOn = accent.Mix(ui.Hex("#ffffff"), .08)
		p.Green = ui.Hex("#18794e")
		p.Red = ui.Hex("#d13438")
		p.Warning = ui.Hex("#a35200")
		p.Shadow = ui.RGBA(16, 24, 40, .12)
		p.CardShadow = ui.RGBA(16, 24, 40, .05)
		p.Scrollbar = ui.RGBA(0, 0, 0, .14)
		p.AccentSoft = accent.Alpha(.11)
	}
	surfaceAccent := accent.Over(p.Surface)
	p.AccentInk = AccentInk(surfaceAccent)
	p.TrackOnInk = AccentInk(p.TrackOn)
	p.AccentText = accentText(surfaceAccent, p.Surface, p.Text)
	p.GreenSoft = p.Green.Alpha(.14)
	p.RedSoft = p.Red.Alpha(.13)
	p.WarningSoft = p.Warning.Alpha(.14)
	return p
}

// APCA-W3 contrast helpers adapted from Tomori's kit.
func apcaY(c ui.Color) float64 {
	linear := func(v uint8) float64 { return math.Pow(float64(v)/255, 2.4) }
	y := 0.2126729*linear(c.R) + 0.7151522*linear(c.G) + 0.0721750*linear(c.B)
	if y < 0.022 {
		y += math.Pow(0.022-y, 1.414)
	}
	return y
}

// ApcaLc returns the signed APCA contrast value between text and background.
func ApcaLc(text, background ui.Color) float64 {
	yt, yb := apcaY(text), apcaY(background)
	if math.Abs(yb-yt) < 0.0005 {
		return 0
	}
	if yb > yt {
		s := (math.Pow(yb, 0.56) - math.Pow(yt, 0.57)) * 1.14
		if s < 0.1 {
			return 0
		}
		return (s - 0.027) * 100
	}
	s := (math.Pow(yb, 0.65) - math.Pow(yt, 0.62)) * 1.14
	if s > -0.1 {
		return 0
	}
	return (s + 0.027) * 100
}

var inkWhite, inkDark = ui.Hex("#ffffff"), ui.Hex("#15181e")

// AccentInk selects light or dark foreground ink for maximum legibility over background.
func AccentInk(background ui.Color) ui.Color {
	if math.Abs(ApcaLc(inkWhite, background)) >= math.Abs(ApcaLc(inkDark, background)) {
		return inkWhite
	}
	return inkDark
}

func accentText(accent, surface, text ui.Color) ui.Color {
	for i := 0; i <= 10; i++ {
		color := accent.Mix(text, float32(i)/10)
		if math.Abs(ApcaLc(color, surface)) >= 50 {
			return color
		}
	}
	return text
}

func NarrowWidth(ctx *ui.Context) bool {
	w, _ := ctx.Size()
	return w < 520
}

func ControlHeight(touch bool) float32 {
	if touch {
		return 38
	}
	return 32
}
