package kit

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

type liquidLensSpec struct {
	Tint       ui.Color
	FallbackBg ui.Color
	Clarity    float32
	Glow       float32 // >= 0 optical press illumination intensity
	Hover      float32 // 0..1 hover illumination
	Touch      ui.Point
	ClearLens  bool    // true for clear optical magnifying droplets
	RimBoost   float32 // extra specular rim highlight
}

func expandGlass(box ui.Rect, dx, dy float32) ui.Rect {
	return ui.Rect{X: box.X - dx, Y: box.Y - dy, W: max(1, box.W+2*dx), H: max(1, box.H+2*dy)}
}

// attachLiquidLens fades the material and its highlights together.
func attachLiquidLens(ctx *ui.Context, key string, radius, opacity float32, expand func(ui.Rect) (dx, dy float32), spec liquidLensSpec) {
	a := clamp32(opacity, 0, 1)
	if a <= 0.005 || StaticSnapshot {
		return
	}
	lens := ui.Box(ctx.Key(key)).Absolute().Fill().PassThrough()
	if a < 0.995 {
		lens.Opacity(a)
	}
	lens.Draw(func(painter *ui.Painter, r ui.Rect) {
		if r.W <= 0 || r.H <= 0 {
			return
		}
		var dx, dy float32
		if expand != nil {
			dx, dy = expand(r)
		}
		outBox := expandGlass(r, dx, dy)
		rad := radius
		if rad <= 0 {
			rad = outBox.H * 0.5
		} else {
			rad = max(1, rad+max(dy, 0))
		}
		paintLiquidGlass(painter, outBox, rad, spec)
	})
}

// applyLiquidRim paints a crisp, unblurred perimeter specular bezel onto el
// whose top and bottom catchlights tilt gently with normalized pointer X.
func applyLiquidRim(el ui.Element, p Palette, width, alpha, glow, tx float32, tint ui.Color) ui.Element {
	a := clamp32(alpha, 0, 1)
	if el == (ui.Element{}) || StaticSnapshot || a <= 0.005 {
		return el
	}
	g := clamp32(glow, 0, 1.35)
	if tx == 0 {
		tx = 0.5
	}
	tx = clamp32(tx, 0.06, 0.94)

	hi := ui.RGBA(255, 255, 255, 255)
	topA := clamp32(a*(0.62+0.26*g), 0, 0.92)
	sideA := clamp32(a*(0.14+0.08*g), 0, 0.34)
	botA := clamp32(a*(0.40+0.20*g), 0, 0.72)
	if p.Dark {
		hi = ui.RGBA(232, 242, 255, 255)
		topA = clamp32(a*(0.48+0.26*g), 0, 0.85)
		sideA = clamp32(a*(0.10+0.06*g), 0, 0.24)
		botA = clamp32(a*(0.28+0.18*g), 0, 0.58)
	}
	edgeAccent := hi
	if tint.A > 0 {
		edgeAccent = hi.Mix(tint, 0.30)
	}

	angle := float32(180) + (tx-0.5)*26
	return el.Border(clamp32(width, 0.75, 2.5), ui.Color{}).BorderLinearGradient(ui.LinearGradient{
		Angle: angle,
		Stops: []ui.GradientStop{
			{Color: hi.Alpha(topA), At: 0},
			{Color: edgeAccent.Alpha(sideA), At: 0.26},
			{Color: edgeAccent.Alpha(sideA * 0.82), At: 0.74},
			{Color: hi.Alpha(botA), At: 1},
		},
	})
}

// paintLiquidGlass renders a curved Liquid Glass lens with touch-reactive
// specular highlights using MyGo's built-in glass material and vector painters.
func paintLiquidGlass(p *ui.Painter, box ui.Rect, radius float32, spec liquidLensSpec) {
	if box.W <= 0 || box.H <= 0 {
		return
	}
	r := min(radius, min(box.W, box.H)*0.5)
	prefs := p.Preferences()
	if prefs.ReduceTransparency {
		paintReducedGlass(p, box, r, spec)
		return
	}

	dark := p.Theme().Dark
	glow := clamp32(spec.Glow, 0, 1.35)
	hover := clamp32(spec.Hover, 0, 1)
	tx, ty := clamp32(spec.Touch.X, 0.06, 0.94), clamp32(spec.Touch.Y, 0.06, 0.94)

	style := glass.Regular
	if spec.ClearLens {
		style = glass.Clear
	}
	glassTint := computePressGlassTint(spec.Tint, glow, hover, dark, spec.ClearLens)
	clarity := clamp32(spec.Clarity+0.26*glow, 0, 1)

	glass.Paint(p, box, r, glass.Glass{
		Style:   style,
		Tint:    glassTint,
		Clarity: clarity,
	})
	paintGlassDomeSheen(p, box, r, glow, hover, tx, spec.ClearLens, dark)
	paintGlassTouchPool(p, box, r, glow, hover, tx, ty, spec.Tint, spec.ClearLens, dark)
	paintGlassRimStrokes(p, box, r, glow, hover, spec.RimBoost, spec.Tint, dark, prefs.HighContrast)
}

func paintReducedGlass(p *ui.Painter, box ui.Rect, r float32, spec liquidLensSpec) {
	c := p.Theme().Surface
	if spec.Tint.A > 0 {
		c = c.Mix(spec.Tint, float32(spec.Tint.A)/255*0.85)
	} else if spec.FallbackBg.A > 0 {
		c = spec.FallbackBg
	}
	p.Fill(box, c, r)
}

func computePressGlassTint(base ui.Color, glow, hover float32, dark, clearLens bool) ui.Color {
	lift := clamp32(0.05*hover+0.12*glow, 0, 0.24)
	if dark {
		lift = clamp32(0.025*hover+0.07*glow, 0, 0.14)
	}
	if clearLens {
		lift *= 0.45
	}
	if lift <= 0.002 {
		return base
	}
	specWhite := ui.RGBA(255, 255, 255, 255)
	if dark {
		specWhite = ui.RGBA(212, 228, 255, 255)
	}
	if base.A == 0 {
		return specWhite.Alpha(lift)
	}
	return base.Mix(specWhite, lift*0.65)
}

func paintGlassDomeSheen(p *ui.Painter, box ui.Rect, r, glow, hover, tx float32, clearLens, dark bool) {
	strength := 0.24*glow + 0.12*hover
	if clearLens {
		strength = max(strength, 0.16)
	}
	if strength <= 0.004 {
		return
	}
	spec := ui.RGBA(255, 255, 255, 255)
	topK, botK := float32(0.24), float32(0.11)
	if dark {
		spec = ui.RGBA(232, 242, 255, 255)
		topK, botK = 0.13, 0.07
	}
	topA := clamp32(topK*strength, 0, 0.34)
	botA := clamp32(botK*strength, 0, 0.18)
	p.FillGradient(box, ui.LinearGradient{
		Angle: 180 + (tx-0.5)*18,
		Stops: []ui.GradientStop{
			{Color: spec.Alpha(topA), At: 0},
			{Color: spec.Alpha(topA * 0.22), At: 0.26},
			{Color: ui.Transparent, At: 0.56},
			{Color: spec.Alpha(botA * 0.24), At: 0.84},
			{Color: spec.Alpha(botA), At: 1},
		},
	}, r)
}

func paintGlassTouchPool(p *ui.Painter, box ui.Rect, r, glow, hover, tx, ty float32, tint ui.Color, clearLens, dark bool) {
	strength := glow*0.85 + hover*0.20
	if clearLens {
		strength = max(strength, 0.32)
	}
	if strength <= 0.005 {
		return
	}

	radSpan := clamp32(min(box.W, box.H)*1.08, 26, max(box.W, box.H)*0.9)
	core := ui.RGBA(255, 255, 255, 255)
	coreK, midK, tailK := float32(0.26), float32(0.14), float32(0.05)
	if dark {
		core = ui.RGBA(230, 242, 255, 255)
		coreK, midK, tailK = 0.15, 0.075, 0.025
	}
	tail := core
	if tint.A > 20 {
		tail = tint.Mix(core, 0.6)
	}
	p.FillRadialGradient(box, ui.RadialGradient{
		Center: ui.Point{X: tx, Y: ty},
		Radius: radSpan,
		Stops: []ui.GradientStop{
			{Color: core.Alpha(clamp32(coreK*strength, 0, 0.38)), At: 0},
			{Color: core.Alpha(clamp32(midK*strength, 0, 0.24)), At: 0.32},
			{Color: tail.Alpha(clamp32(tailK*strength, 0, 0.12)), At: 0.68},
			{Color: ui.Transparent, At: 1},
		},
	}, r)
}

func paintGlassRimStrokes(p *ui.Painter, box ui.Rect, r, glow, hover, rimBoost float32, tint ui.Color, dark, highContrast bool) {
	rimStrength := clamp32(0.28+0.14*hover+0.34*glow+rimBoost, 0, 0.95)
	if rimStrength <= 0.005 {
		return
	}
	outerW := float32(1.0) + 0.25*glow
	if highContrast {
		outerW = max(outerW, 1.5)
	}
	rimColor := ui.RGBA(255, 255, 255, 255)
	innerColor := rimColor
	outerAlpha := clamp32(rimStrength*0.62, 0, 0.85)
	innerAlpha := clamp32(rimStrength*0.18, 0, 0.26)
	if dark {
		rimColor = ui.RGBA(232, 242, 255, 255)
		outerAlpha = clamp32(rimStrength*0.46, 0, 0.72)
		innerAlpha = clamp32(rimStrength*0.12, 0, 0.20)
	}
	if tint.A > 0 {
		innerColor = rimColor.Mix(tint, 0.35)
	}

	if box.W > 3 && box.H > 3 && innerAlpha > 0.005 {
		inRect := ui.Rect{X: box.X + 1.25, Y: box.Y + 1.25, W: box.W - 2.5, H: box.H - 2.5}
		p.Stroke(inRect, innerColor.Alpha(innerAlpha), max(1, r-1.25), 2.0)
	}
	if box.W > 1.5 && box.H > 1.5 {
		outRect := ui.Rect{X: box.X + 0.5, Y: box.Y + 0.5, W: box.W - 1, H: box.H - 1}
		p.Stroke(outRect, rimColor.Alpha(outerAlpha), max(1, r-0.5), outerW)
	}
}
