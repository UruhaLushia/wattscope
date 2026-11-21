package kit

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

func CanvasBackdrop(el ui.Element, p Palette) ui.Element {
	return el.Background(p.Canvas)
}

func GlassSurface(el ui.Element, p Palette, _ float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	el.Background(fallbackBg)
	if fallbackBorder.A > 0 {
		el.Border(1, fallbackBorder)
	}
	return el
}

func GlassPanel(el ui.Element, p Palette, radius, clarity float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return ApplyCardShadow(GlassSurface(el.Radius(radius), p, clarity, fallbackBg, fallbackBorder), p)
}

func GlassFloating(el ui.Element, p Palette, radius, clarity float32, tint ui.Color, interactive bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(el, p, radius, clarity, tint, interactive, true, fallbackBg, fallbackBorder)
}

func glassFloatingUnclipped(el ui.Element, p Palette, radius, clarity float32, tint ui.Color, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(el, p, radius, clarity, tint, false, false, fallbackBg, fallbackBorder)
}

func glassFloating(el ui.Element, p Palette, radius, clarity float32, tint ui.Color, interactive, clip bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	el.Radius(radius)
	if clip && (!p.LiquidGlass || !interactive) {
		el.Clip()
	}
	if p.LiquidGlass {
		base := glass.Glass{Tint: tint, Clarity: clarity}
		if interactive {
			return el.Material(trackGlassInteraction(el).material(base))
		}
		return el.Material(staticGlass{base: base})
	}
	el.Background(fallbackBg)
	if fallbackBorder.A > 0 {
		el.Border(1, fallbackBorder)
	}
	return el
}

func GlassFloatingButton(el ui.Element, p Palette, radius float32, tint ui.Color, fallbackBg, fallbackBorder ui.Color) ui.Element {
	if !p.LiquidGlass {
		return GlassFloating(el, p, radius, 0, tint, false, fallbackBg, fallbackBorder)
	}
	state := trackGlassInteraction(el)
	if !StaticSnapshot {
		anim := MotionGlassRelease
		if el.Pressed() && !el.IsDisabled() {
			anim = MotionGlassPress
		}
		scale := 1 + 0.18*clamp32(state.elastic, -0.15, 1.25)
		el.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: scale}, anim)
	}
	return el.Radius(radius).Material(interactiveGlass{
		base:             glass.Glass{Tint: tint},
		glassInteraction: state,
	})
}

func GlassBarChrome(el ui.Element, p Palette, fallbackBg, fallbackBorder ui.Color) ui.Element {
	if p.LiquidGlass {
		return el
	}
	el.Background(fallbackBg)
	if fallbackBorder.A > 0 {
		el.BorderWidth(0, 0, 1, 0).BorderColor(fallbackBorder)
	}
	return el
}

var glassScrollBlur = glass.Blur{
	Radius: 6,
	Mask:   &ui.LinearGradient{From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180},
}

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
