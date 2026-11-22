package kit

import (
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

type glassPressKey struct{}
type glassHoverKey struct{}

type glassInteraction struct {
	elastic, glow, hover float32
	touch                ui.Point
}

func trackGlassInteraction(owner ui.Element, staticSnapshot bool) glassInteraction {
	center := ui.Point{X: 0.5, Y: 0.5}
	if staticSnapshot {
		return glassInteraction{touch: center}
	}
	touch := ui.Local(owner, glassPressKey{}, func() ui.Point { return center })
	enabled := !owner.IsDisabled()
	pressed := enabled && owner.Pressed()
	var target, hoverTarget float32
	if pressed {
		target = 1
		box := owner.LayoutBounds()
		if box.W > 0 && box.H > 0 {
			x, y, _ := owner.PointerPosition()
			*touch = ui.Point{X: clamp32(x/box.W, 0.06, 0.94), Y: clamp32(y/box.H, 0.06, 0.94)}
		}
	}
	if enabled && owner.Hovered() {
		hoverTarget = 1
	}
	press := owner.Spring(glassPressKey{}, target, glassPressAnimation(pressed).Spring)
	return glassInteraction{
		elastic: clamp32(press, -0.24, 1.38),
		glow:    clamp32(press, 0, 1.38),
		hover:   owner.Animate(glassHoverKey{}, hoverTarget, 120*time.Millisecond),
		touch:   *touch,
	}
}

func (g glassInteraction) active() bool {
	return g.glow > 0.005 || g.hover > 0.005 || g.elastic != 0
}

func resolveGlassAlpha(alpha float32) float32 {
	if alpha <= 0 {
		return 1
	}
	return clamp32(alpha, 0, 1)
}

func (g glassInteraction) material(base glass.Glass, alpha float32, staticSnapshot bool) ui.Material {
	if g.active() {
		return interactiveGlass{base: base, glassInteraction: g, alpha: alpha, staticSnapshot: staticSnapshot}
	}
	return staticGlass{base: base, alpha: alpha, staticSnapshot: staticSnapshot}
}

type staticGlass struct {
	base           glass.Glass
	alpha          float32
	staticSnapshot bool
}

func (g staticGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	a := resolveGlassAlpha(g.alpha)
	if g.staticSnapshot || p.Preferences().ReduceTransparency {
		if a > 0.5 {
			g.base.PaintMaterial(p, box, radii)
		}
		return
	}
	radius := radii[0]
	if radius <= 0 {
		radius = min(box.W, box.H) * 0.5
	}
	paintLiquidGlass(p, box, radius, liquidLensSpec{
		Tint:      g.base.Tint,
		Clarity:   g.base.Clarity,
		Alpha:     a,
		ClearLens: g.base.Style == glass.Clear,
	})
}

func glassIconPop(ctx *ui.Context, iconEl ui.Element, p Palette, amount float32) {
	if !p.LiquidGlass || iconEl == (ui.Element{}) || isStaticSnapshot(ctx) {
		return
	}
	target := float32(1)
	anim := motionGlassRelease
	if amount > 0 {
		target = 1 + 0.14*clamp32(amount, 0, 1.25)
		anim = motionGlassPress
	}
	iconEl.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: target}, anim)
}

type dockGlass struct {
	glassInteraction
	alpha          float32
	staticSnapshot bool
}

func (g dockGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	a := resolveGlassAlpha(g.alpha)
	if g.staticSnapshot || p.Preferences().ReduceTransparency {
		if a > 0.5 {
			(glass.Glass{}).PaintMaterial(p, box, radii)
		}
		return
	}
	dx, dy := float32(0), float32(0)
	if g.active() {
		dx, dy = 4.5*g.elastic, 3.0*g.elastic
	}
	paintLiquidGlass(p, expandGlass(box, dx, dy), max(1, radii[0]+max(dy, 0)), liquidLensSpec{
		Clarity: 0.45,
		Alpha:   a,
		Glow:    0.15 * g.glow,
		Touch:   g.touch,
	})
}

type interactiveGlass struct {
	base glass.Glass
	glassInteraction
	alpha          float32
	staticSnapshot bool
}

func (g interactiveGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	a := resolveGlassAlpha(g.alpha)
	if g.staticSnapshot || p.Preferences().ReduceTransparency {
		if a > 0.5 {
			g.base.PaintMaterial(p, box, radii)
		}
		return
	}
	var dx, dy float32
	zoom := float32(1)
	if g.active() {
		maxExX := clamp32(box.W*0.14, 6.0, 10.5)
		maxExY := clamp32(box.H*0.18, 5.0, 8.5)
		dx = maxExX*g.elastic + 0.4*g.hover
		dy = maxExY*g.elastic + 0.3*g.hover
		zoom = 1 + 0.16*clamp32(g.glow, 0, 1.2)
	}
	paintLiquidGlass(p, expandGlass(box, dx, dy), max(1, radii[0]+max(dy, 0)), liquidLensSpec{
		Tint:    g.base.Tint,
		Clarity: g.base.Clarity,
		Alpha:   a,
		Glow:    g.glow,
		Hover:   g.hover,
		Zoom:    zoom,
		Touch:   g.touch,
	})
}

// GlassPillOptions configures the backing pill surface and Liquid Glass lens for GlassPillControl.
type GlassPillOptions struct {
	Radius             float32
	Tint               ui.Color
	VisibleAtRest      bool
	FallbackBackground ui.Color
	BorderWidth        float32
	FallbackBorder     ui.Color
	PressScale         float32
}

// GlassPillControl attaches a press-expanding Liquid Glass pill backing onto owner.
func GlassPillControl(ctx *ui.Context, owner ui.Element, p Palette, opts GlassPillOptions) {
	radius := opts.Radius
	tint := opts.Tint
	visibleAtRest := opts.VisibleAtRest
	fallbackBg := opts.FallbackBackground
	borderW := opts.BorderWidth
	fallbackBorder := opts.FallbackBorder
	pressScale := opts.PressScale

	if !p.LiquidGlass {
		pillSurface(ctx, owner, radius, fallbackBg, borderW, fallbackBorder, pressScale)
		return
	}

	interaction := trackGlassInteraction(owner, isStaticSnapshot(ctx))
	elastic, glow, hover := interaction.elastic, interaction.glow, interaction.hover
	surface := ui.Box(ctx).Absolute().Fill().ZIndex(-1).Radius(radius).PassThrough()

	bg := fallbackBg
	if glow > 0.01 && tint.A == 0 && bg.A == 0 {
		if p.Dark {
			bg = ui.RGBA(255, 255, 255, 0.04*clamp32(glow, 0, 1))
		} else {
			bg = ui.RGBA(255, 255, 255, 0.12*clamp32(glow, 0, 1))
		}
	}
	surface.Background(surface.AnimateColor("pill-bg", bg, MotionFast))
	if borderW > 0 {
		surface.Border(borderW, surface.AnimateColor("pill-bd", fallbackBorder, MotionFast))
	}
	if !interaction.active() {
		if visibleAtRest && fallbackBg.A > 0 && tint.A == 0 {
			drawRestPillSheen(surface, radius, p.Dark)
		}
		return
	}

	subtle := pressScale >= 0.98
	active := max(glow, hover*0.55)
	glassTint := tint
	if glassTint.A == 0 && fallbackBg.A > 0 {
		glassTint = scaleAlpha(fallbackBg, 0.42)
	}
	touch := interaction.touch
	surface.Children(func() {
		attachLiquidLens(ctx, "glass-lens", radius, active, func(r ui.Rect) (float32, float32) {
			if subtle {
				return 3.0 * elastic, 3.0 * elastic
			}
			maxExX := clamp32(r.W*0.15, 7.0, 13.0)
			maxExY := clamp32(r.H*0.22, 6.0, 10.5)
			return maxExX*elastic + 0.4*hover, maxExY*elastic + 0.3*hover
		}, liquidLensSpec{
			Tint:       glassTint,
			FallbackBg: fallbackBg,
			Glow:       glow,
			Hover:      hover,
			Zoom:       1 + 0.18*clamp32(glow, 0, 1.15),
			ClearLens:  !subtle && glassTint.A == 0,
			Touch:      touch,
		})
	})
}
