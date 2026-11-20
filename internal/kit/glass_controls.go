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

func trackGlassInteraction(owner ui.Element) glassInteraction {
	center := ui.Point{X: 0.5, Y: 0.5}
	if StaticSnapshot {
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
	press := owner.Spring(glassPressKey{}, target, GlassPressAnimation(pressed).Spring)
	return glassInteraction{
		elastic: clamp32(press, -0.22, 1.30),
		glow:    clamp32(press, 0, 1.35),
		hover:   owner.Animate(glassHoverKey{}, hoverTarget, 120*time.Millisecond),
		touch:   *touch,
	}
}

func (g glassInteraction) active() bool {
	return g.glow > 0.005 || g.hover > 0.005 || g.elastic != 0
}

func (g glassInteraction) material(base glass.Glass) ui.Material {
	if g.active() {
		return interactiveGlass{base: base, glassInteraction: g}
	}
	return base
}

// GlassIconPop gives SVG icons inside Liquid Glass controls a subtle spring pop
// on press while keeping neighboring text labels unscaled for sharp glyphs.
func GlassIconPop(iconEl ui.Element, p Palette, amount float32) ui.Element {
	if !p.LiquidGlass || iconEl == (ui.Element{}) || StaticSnapshot {
		return iconEl
	}
	target := float32(1)
	anim := MotionGlassRelease
	if amount > 0 {
		target = 1 + 0.07*clamp32(amount, 0, 1.2)
		anim = MotionGlassPress
	}
	return iconEl.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: target}, anim)
}

type dockGlass struct {
	glassInteraction
}

func (g dockGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	if p.Preferences().ReduceTransparency || !g.active() {
		(glass.Glass{}).PaintMaterial(p, box, radii)
		return
	}
	dx, dy := 1.8*g.elastic, 1.2*g.elastic
	paintLiquidGlass(p, expandGlass(box, dx, dy), max(1, radii[0]+max(dy, 0)), liquidLensSpec{
		Glow:     0.28 * g.glow,
		Touch:    g.touch,
		RimBoost: 0.08 * g.glow,
	})
}

type interactiveGlass struct {
	base glass.Glass
	glassInteraction
}

func (g interactiveGlass) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	if p.Preferences().ReduceTransparency {
		g.base.PaintMaterial(p, box, radii)
		return
	}
	maxExX := clamp32(box.W*0.045, 2.4, 5.5)
	maxExY := clamp32(box.H*0.085, 1.9, 4.5)
	dx := maxExX*g.elastic + 0.35*g.hover
	dy := maxExY*g.elastic + 0.25*g.hover
	paintLiquidGlass(p, expandGlass(box, dx, dy), max(1, radii[0]+max(dy, 0)), liquidLensSpec{
		Tint:    g.base.Tint,
		Clarity: g.base.Clarity,
		Glow:    g.glow,
		Hover:   g.hover,
		Touch:   g.touch,
	})
}

// GlassPillControl attaches a press-expanding Liquid Glass pill backing onto owner.
func GlassPillControl(ctx *ui.Context, owner ui.Element, p Palette, radius float32, tint ui.Color, visibleAtRest bool, fallbackBg ui.Color, borderW float32, fallbackBorder ui.Color, pressScale float32) {
	if !p.LiquidGlass {
		PillSurface(ctx, owner, radius, fallbackBg, borderW, fallbackBorder, pressScale)
		return
	}

	interaction := trackGlassInteraction(owner)
	elastic, glow, hover := interaction.elastic, interaction.glow, interaction.hover
	surface := ui.Box(ctx).Absolute().Fill().ZIndex(-1).Radius(radius).PassThrough()

	bg := fallbackBg
	if glow > 0.01 && tint.A == 0 && bg.A == 0 {
		if p.Dark {
			bg = ui.RGBA(255, 255, 255, 0.05*clamp32(glow, 0, 1))
		} else {
			bg = ui.RGBA(255, 255, 255, 0.16*clamp32(glow, 0, 1))
		}
	}
	surface.Background(surface.AnimateColor("pill-bg", bg, MotionFast))
	if borderW > 0 {
		surface.Border(borderW, surface.AnimateColor("pill-bd", fallbackBorder, MotionFast))
	}
	if !interaction.active() {
		if visibleAtRest && fallbackBg.A > 0 && tint.A == 0 {
			rimAlpha := float32(0.28)
			if p.Dark {
				rimAlpha = 0.12
			}
			surface.Draw(func(painter *ui.Painter, r ui.Rect) {
				if r.W <= 0 || r.H <= 0 || painter.Preferences().ReduceTransparency {
					return
				}
				painter.Stroke(ui.Rect{X: r.X + 0.5, Y: r.Y + 0.5, W: max(0, r.W-1), H: max(0, r.H-1)}, ui.RGBA(255, 255, 255, rimAlpha), max(1, radius-0.5), 1)
			})
		}
		return
	}

	subtle := pressScale >= 0.98
	active := max(glow, hover*0.55)
	glassTint := tint
	if glassTint.A == 0 && fallbackBg.A > 0 {
		glassTint = scaleAlpha(fallbackBg, 0.48)
	}
	surface.Children(func() {
		attachLiquidLens(ctx, "glass-lens", radius, active*1.15, func(r ui.Rect) (float32, float32) {
			if subtle {
				return 1.4 * elastic, 1.4 * elastic
			}
			maxExX := clamp32(r.W*0.042, 2.2, 4.4)
			maxExY := clamp32(r.H*0.075, 1.8, 3.6)
			return maxExX*elastic + 0.25*hover, maxExY*elastic + 0.2*hover
		}, liquidLensSpec{
			Tint:       glassTint,
			FallbackBg: fallbackBg,
			Glow:       glow,
			Hover:      hover,
			Touch:      interaction.touch,
		})
	})
}

// GlassSelectionPill renders the active tab or segment indicator pill with
// Liquid Glass bloom, bezel refraction, and touch-point highlight on press.
func GlassSelectionPill(ctx *ui.Context, owner ui.Element, p Palette, radius float32, restBg ui.Color, matchKey any, showShadow bool) {
	if !p.LiquidGlass {
		pillBg := restBg
		pillPose, pillAnim := ui.Pose{}, MotionRail
		if owner.Pressed() {
			pillBg = restBg.Mix(p.Text, 0.10)
			pillPose.Scale = 0.96
			pillAnim = MotionFast
		}
		pill := ui.Box(ctx).Absolute().Fill().ZIndex(-1).Radius(radius).
			TransformOrigin(0.5, 0.5).AnimatePose(pillPose, pillAnim).
			Background(owner.AnimateColor("sp-bg", pillBg, MotionFast)).PassThrough().
			Transition(PillMatchTransition(matchKey))
		if showShadow && p.CardShadow.A > 0 && !owner.Pressed() {
			pill.Shadow(0, 1, 2, 0, p.CardShadow)
		}
		return
	}

	interaction := trackGlassInteraction(owner)
	elastic, glow, hover := interaction.elastic, interaction.glow, interaction.hover
	lensMix := clamp32(glow, 0, 1)
	bg := scaleAlpha(restBg, 1-0.36*lensMix)

	pill := ui.Box(ctx).Absolute().Fill().ZIndex(-1).Radius(radius).
		Background(owner.AnimateColor("sp-bg", bg, MotionFast)).PassThrough().
		Transition(pillMatchTransition(matchKey, &MotionGlassScrub))

	if showShadow && p.CardShadow.A > 0 {
		pill.Shadow(0, 1+2*lensMix, 3+6*lensMix, 0, p.CardShadow)
	} else if lensMix > 0.01 {
		alpha := float32(0.08) * lensMix
		if p.Dark {
			alpha = 0.22 * lensMix
		}
		pill.Shadow(0, 2*lensMix, 5*lensMix, 0, ui.RGBA(12, 18, 30, alpha))
	}

	if !interaction.active() || ctx.Preferences().ReduceTransparency {
		return
	}
	tint := scaleAlpha(restBg, 0.52)
	pill.Children(func() {
		attachLiquidLens(ctx, "glass-lens", radius, max(glow, hover*0.55)*1.15, func(r ui.Rect) (float32, float32) {
			maxExX := clamp32(r.W*0.045, 2.2, 4.5)
			maxExY := clamp32(r.H*0.10, 2.0, 3.8)
			return maxExX*elastic + 0.3*hover, maxExY*elastic + 0.2*hover
		}, liquidLensSpec{
			Tint:     tint,
			Glow:     max(glow, hover*0.4),
			Hover:    hover,
			Touch:    interaction.touch,
			RimBoost: 0.16 * lensMix,
		})
	})
}
