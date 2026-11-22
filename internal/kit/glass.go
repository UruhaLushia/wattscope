package kit

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// CanvasBackdrop sets the themed page canvas background on el.
func CanvasBackdrop(el ui.Element, p Palette) ui.Element {
	return el.Background(p.Canvas)
}

func glassSurface(el ui.Element, fallbackBg, fallbackBorder ui.Color) ui.Element {
	el.Background(fallbackBg)
	if fallbackBorder.A > 0 {
		el.Border(1, fallbackBorder)
	}
	return el
}

// GlassPanel styles el as a themed surface panel with rounded corners and card shadow.
func GlassPanel(el ui.Element, p Palette, radius float32) ui.Element {
	return ApplyCardShadow(glassSurface(el.Radius(radius), p.Surface, p.LineSoft), p)
}

type glassMixKey struct{}
type glassRadiusKey struct{}

func morphFloat(ctx *ui.Context, el ui.Element, key any, target float32, anim ui.Animation) float32 {
	if isStaticSnapshot(ctx) || isScaleReflow(ctx) {
		return target
	}
	return el.AnimateTo(key, target, anim)
}

func morphRadius(ctx *ui.Context, el ui.Element, key any, target float32) float32 {
	target = max(0, target)
	return max(0, morphFloat(ctx, el, key, target, motionGlassShape))
}

func glassMix(ctx *ui.Context, el ui.Element, p Palette) float32 {
	var target float32
	if p.LiquidGlass {
		target = 1
	}
	return clamp32(morphFloat(ctx, el, glassMixKey{}, target, motionGlassMorph), 0, 1)
}

func glassFloatingEl(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, interactive bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(ctx, el, p, radius, clarity, interactive, true, fallbackBg, fallbackBorder)
}

func glassFloatingUnclipped(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(ctx, el, p, radius, clarity, false, false, fallbackBg, fallbackBorder)
}

func glassFloating(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, interactive, clip bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	mix := glassMix(ctx, el, p)
	el.Radius(morphRadius(ctx, el, glassRadiusKey{}, radius))
	if clip && (!p.LiquidGlass || !interactive) {
		el.Clip()
	}
	bg := el.AnimateColor("g-bg", fallbackBg, motionGlassMorph)
	if mix < 0.995 {
		el.Background(scaleAlpha(bg, 1-mix*mix))
	} else {
		el.Background(ui.Transparent)
	}
	if fallbackBorder.A > 0 {
		el.Border(1, scaleAlpha(fallbackBorder, 1-mix))
	}
	if mix > 0.005 {
		snap := isStaticSnapshot(ctx)
		base := glass.Glass{Clarity: clarity}
		if interactive && p.LiquidGlass {
			return el.Material(trackGlassInteraction(el, snap).material(base, mix, snap))
		}
		return el.Material(staticGlass{base: base, alpha: mix, staticSnapshot: snap})
	}
	return el
}

// GlassFloatingButton attaches interactive Liquid Glass optics and press spring scaling onto floating button el.
func GlassFloatingButton(ctx *ui.Context, el ui.Element, p Palette, radius float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	mix := glassMix(ctx, el, p)
	el.Radius(morphRadius(ctx, el, glassRadiusKey{}, radius))
	bg := el.AnimateColor("g-bg", fallbackBg, MotionFast)
	if mix < 0.995 {
		el.Background(scaleAlpha(bg, 1-mix*mix))
	} else {
		el.Background(ui.Transparent)
	}
	if fallbackBorder.A > 0 {
		el.Border(1, scaleAlpha(fallbackBorder, 1-mix))
	}
	if mix > 0.005 {
		snap := isStaticSnapshot(ctx)
		state := trackGlassInteraction(el, snap)
		if !snap {
			anim := motionGlassRelease
			if el.Pressed() && !el.IsDisabled() {
				anim = motionGlassPress
			}
			scale := 1 + 0.18*mix*clamp32(state.elastic, -0.15, 1.25)
			el.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: scale}, anim)
		}
		el.Material(interactiveGlass{
			base:             glass.Glass{},
			glassInteraction: state,
			alpha:            mix,
			staticSnapshot:   snap,
		})
	}
	return el
}

// GlassBarChrome applies the animated header background and bottom border to el across LiquidGlass transitions.
func GlassBarChrome(el ui.Element, p Palette) ui.Element {
	bg, border := p.Surface, p.LineSoft
	if p.LiquidGlass {
		bg, border = ui.Transparent, ui.Transparent
	}
	el.Background(el.AnimateColor("bar-bg", bg, motionGlassMorph))
	if p.LineSoft.A > 0 {
		el.BorderWidth(0, 0, 1, 0).BorderColor(el.AnimateColor("bar-bd", border, motionGlassMorph))
	}
	return el
}

var glassScrollBlur = glass.Blur{
	Radius: 6,
	Mask:   &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180},
}

// GlassScrollEdge renders a soft top backdrop-blur fade overlay under floating chrome.
func GlassScrollEdge(ctx *ui.Context, p Palette, offset, height float32, zIndex int) {
	if !p.LiquidGlass || height <= 0 {
		return
	}
	ui.Box(ctx.Key("scroll-edge-top")).Absolute().Left(0).Right(0).Top(offset).Height(height).
		ZIndex(zIndex).PassThrough().
		Material(glassScrollBlur).
		Draw(func(painter *ui.Painter, box ui.Rect) {
			painter.FillGradient(box, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: p.Canvas.Alpha(0.38), At: 0},
				{Color: p.Canvas.Alpha(0.08), At: 0.45},
				{Color: ui.Transparent, At: 1},
			}}, 0)
		})
}

func glassNeutral(dark bool, lightAlpha, darkAlpha float32) ui.Color {
	if dark {
		return ui.RGBA(255, 255, 255, darkAlpha)
	}
	return ui.RGBA(15, 23, 42, lightAlpha)
}

// GlassAccentTint returns a theme-appropriate translucent accent tint color.
func GlassAccentTint(p Palette, lightAlpha, darkAlpha float32) ui.Color {
	if p.Dark {
		return p.Accent.Alpha(darkAlpha)
	}
	return p.Accent.Alpha(lightAlpha)
}

func scaleAlpha(c ui.Color, factor float32) ui.Color {
	c.A = uint8(clamp32(float32(c.A)*factor, 0, 255))
	return c
}
