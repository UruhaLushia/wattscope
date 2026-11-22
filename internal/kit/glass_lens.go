package kit

import (
	_ "embed"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

type liquidLensSpec struct {
	Tint       ui.Color
	FallbackBg ui.Color
	Clarity    float32
	Alpha      float32 // 0..1 optical visibility; 0 treated as 1 when unset
	Glow       float32 // >= 0 optical press illumination intensity
	Hover      float32 // 0..1 hover illumination
	Zoom       float32 // >= 0 optical center magnification (0 treated as 1)
	ZoomY      float32 // optional vertical-axis override (0 matches Zoom)
	Touch      ui.Point
	ClearLens  bool // true for clear optical magnifying droplets
	WarpOnly   bool // true to remap coordinates without bevel, dispersion, or rim shading
}

func expandGlass(box ui.Rect, dx, dy float32) ui.Rect {
	return ui.Rect{X: box.X - dx, Y: box.Y - dy, W: max(1, box.W+2*dx), H: max(1, box.H+2*dy)}
}

// attachLiquidLens renders an edge-refracting, backdrop-transparent lens box.
func attachLiquidLens(ctx *ui.Context, key string, radius, opacity float32, expand func(ui.Rect) (dx, dy float32), spec liquidLensSpec) {
	a := clamp32(opacity, 0, 1)
	if a <= 0.005 || isStaticSnapshot(ctx) {
		return
	}
	spec.Alpha = a
	ui.Box(ctx.Key(key)).Absolute().Fill().PassThrough().Draw(func(painter *ui.Painter, r ui.Rect) {
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

func drawRestPillSheen(el ui.Element, radius float32, dark bool) {
	el.Draw(func(p *ui.Painter, box ui.Rect) {
		if box.W <= 2 || box.H <= 2 || p.Preferences().ReduceTransparency {
			return
		}
		r := min(radius, min(box.W, box.H)*0.5)
		gradient := restPillLight
		if dark {
			gradient = restPillDark
		}
		p.FillGradient(box, gradient, r)
	})
}

var (
	restPillLight = ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.RGBA(255, 255, 255, 255).Alpha(0.14), At: 0},
		{Color: ui.Transparent, At: 0.45},
		{Color: ui.RGBA(255, 255, 255, 255).Alpha(0.04), At: 1},
	}}
	restPillDark = ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.RGBA(232, 242, 255, 255).Alpha(0.06), At: 0},
		{Color: ui.Transparent, At: 0.45},
		{Color: ui.RGBA(232, 242, 255, 255).Alpha(0.02), At: 1},
	}}
)

//go:embed optical_glass.kage
var opticalGlassSource string

var opticalGlassShader = shader.MustCompile(opticalGlassSource)

var (
	lensRadius        = opticalGlassShader.Uniform("Radius")
	lensHalfSize      = opticalGlassShader.Uniform("HalfSize")
	lensShapeInset    = opticalGlassShader.Uniform("ShapeInset")
	lensNormalInset   = opticalGlassShader.Uniform("NormalInset")
	lensBezel         = opticalGlassShader.Uniform("Bezel")
	lensRefraction    = opticalGlassShader.Uniform("Refraction")
	lensMagnification = opticalGlassShader.Uniform("Magnification")
	lensDispersion    = opticalGlassShader.Uniform("Dispersion")
	lensToneLow       = opticalGlassShader.Uniform("ToneLow")
	lensToneRange     = opticalGlassShader.Uniform("ToneRange")
	lensToneChroma    = opticalGlassShader.Uniform("ToneChroma")
	lensToneCurve     = opticalGlassShader.Uniform("ToneCurve")
	lensFrost         = opticalGlassShader.Uniform("Frost")
	lensBevelShade    = opticalGlassShader.Uniform("BevelShade")
	lensHairline      = opticalGlassShader.Uniform("Hairline")
	lensAlpha         = opticalGlassShader.Uniform("Alpha")
	lensGlow          = opticalGlassShader.Uniform("Glow")
	lensDark          = opticalGlassShader.Uniform("Dark")
	lensTint          = opticalGlassShader.Uniform("Tint")
	lensTouch         = opticalGlassShader.Uniform("Touch")
	lensValues        = sync.Pool{New: func() any { return opticalGlassShader.NewValues() }}
)

func paintLiquidGlass(p *ui.Painter, box ui.Rect, radius float32, spec liquidLensSpec) {
	if box.W <= 0 || box.H <= 0 {
		return
	}
	minSide := min(box.W, box.H)
	r := min(radius, minSide*0.5)
	if p.Preferences().ReduceTransparency {
		paintReducedGlass(p, box, r, spec)
		return
	}

	alpha := spec.Alpha
	if alpha <= 0 {
		alpha = 1
	} else {
		alpha = clamp32(alpha, 0, 1)
	}
	dark := p.Theme().Dark
	glow := clamp32(spec.Glow, 0, 1.35)
	hover := clamp32(spec.Hover, 0, 1)
	tx, ty := clamp32(spec.Touch.X, 0.06, 0.94), clamp32(spec.Touch.Y, 0.06, 0.94)
	if spec.Touch.X == 0 && spec.Touch.Y == 0 {
		tx, ty = 0.5, 0.5
	}

	pressAmt := clamp32(max(glow, hover*0.5), 0, 1)
	bezel := clamp32(minSide*0.10, 3.8, 5.2) + 1.8*pressAmt
	refraction := clamp32(bezel*0.26, 0.9, 1.4) + 1.2*pressAmt
	dispersion := float32(0.10) + 0.35*pressAmt
	blur := float32(2.0)
	toneLow, toneHigh, toneSat := float32(0.24), float32(1.0), float32(1.12)
	toneCurve := float32(1.15)
	frost := float32(0.06)
	bevelShade := float32(0.045) * pressAmt
	hairline := float32(0.075)
	if dark {
		toneLow, toneHigh, toneSat = 0.08, 0.58, 1.32
		toneCurve = 1.7
		frost = 0.12
		bevelShade = 0.14 * pressAmt
		hairline = 0.15
	}
	if spec.Clarity > 0 {
		k := clamp32(spec.Clarity, 0, 1)
		frost *= clamp32(1-0.65*k, 0.15, 1)
		blur *= clamp32(1-0.30*k, 0.50, 1)
		if !dark {
			toneLow *= clamp32(1-0.35*k, 0.45, 1)
		}
	}
	if spec.WarpOnly {
		bezel = clamp32(minSide*0.115, 5.8, 8.0)
		refraction = 0
		dispersion = 0
		blur = 0
		toneCurve = 0
		frost = 0
		bevelShade = 0
		hairline = 0
		glow = 0
		hover = 0
	} else if spec.ClearLens {
		bezel = clamp32(minSide*0.115, 6.0, 8.2)
		refraction = clamp32(bezel*0.30, 1.8, 2.5)
		dispersion = 2.95
		blur = 0
		toneCurve = 0
		frost = 0
		bevelShade = 0.005
		hairline = 0.048
		if dark {
			bevelShade = 0.14
			hairline = 0.085
		}
	}
	darkFlag := float32(0)
	if dark {
		darkFlag = 1
	}
	magX := float32(0)
	if spec.Zoom > 0 {
		magX = (clamp32(spec.Zoom, 0.5, 2.0) - 1) * alpha
	}
	magY := magX
	if spec.ZoomY > 0 {
		magY = (clamp32(spec.ZoomY, 0.5, 2.0) - 1) * alpha
	}
	tint := spec.Tint
	toneRange := toneHigh - toneLow
	halfWidth, halfHeight := box.W*0.5, box.H*0.5
	normalRadius := min(r*1.35, minSide*0.5)
	values := lensValues.Get().(*shader.Values)
	values.Float(lensRadius, r).
		Vec2(lensHalfSize, halfWidth, halfHeight).
		Vec2(lensShapeInset, halfWidth-r, halfHeight-r).
		Vec2(lensNormalInset, halfWidth-normalRadius, halfHeight-normalRadius).
		Float(lensBezel, bezel).
		Float(lensRefraction, refraction*alpha).
		Vec2(lensMagnification, magX, magY).
		Float(lensDispersion, dispersion*alpha).
		Float(lensToneLow, toneLow).
		Float(lensToneRange, toneRange).
		Float(lensToneChroma, toneRange*toneSat).
		Float(lensToneCurve, toneCurve).
		Float(lensFrost, clamp32(frost*alpha, 0, 0.85)).
		Float(lensBevelShade, bevelShade).
		Float(lensHairline, hairline).
		Float(lensAlpha, alpha).
		Float(lensGlow, max(glow, hover*0.45)).
		Float(lensDark, darkFlag).
		Vec4(lensTint, float32(tint.R)/255, float32(tint.G)/255, float32(tint.B)/255, clamp32(float32(tint.A)/255*alpha, 0, 0.92)).
		Vec3(lensTouch, tx*box.W, ty*box.H, max(minSide*0.75, 22))
	opticalGlassShader.Draw(p, box, shader.Options{Radius: r, BackdropBlur: blur * alpha, Values: values})
	lensValues.Put(values)
}

func paintReducedGlass(p *ui.Painter, box ui.Rect, r float32, spec liquidLensSpec) {
	c := p.Theme().Surface
	if spec.Tint.A > 0 {
		c = c.Mix(spec.Tint, float32(spec.Tint.A)/255*0.85)
	} else if spec.FallbackBg.A > 0 {
		c = spec.FallbackBg
	}
	alpha := spec.Alpha
	if alpha <= 0 {
		alpha = 1
	} else {
		alpha = clamp32(alpha, 0, 1)
	}
	c = scaleAlpha(c, alpha)
	p.Fill(box, c, r)
}
