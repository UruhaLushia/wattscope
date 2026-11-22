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

func glassFloatingEl(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, interactive bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(ctx, el, p, radius, clarity, interactive, true, fallbackBg, fallbackBorder)
}

func glassFloatingUnclipped(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return glassFloating(ctx, el, p, radius, clarity, false, false, fallbackBg, fallbackBorder)
}

func glassFloating(ctx *ui.Context, el ui.Element, p Palette, radius, clarity float32, interactive, clip bool, fallbackBg, fallbackBorder ui.Color) ui.Element {
	el.Radius(radius)
	if clip && (!p.LiquidGlass || !interactive) {
		el.Clip()
	}
	if p.LiquidGlass {
		snap := isStaticSnapshot(ctx)
		base := glass.Glass{Clarity: clarity}
		if interactive {
			return el.Material(trackGlassInteraction(el, snap).material(base, snap))
		}
		return el.Material(staticGlass{base: base, staticSnapshot: snap})
	}
	return glassSurface(el, fallbackBg, fallbackBorder)
}

// GlassFloatingButton attaches interactive Liquid Glass optics and press spring scaling onto floating button el.
func GlassFloatingButton(ctx *ui.Context, el ui.Element, p Palette, radius float32, fallbackBg, fallbackBorder ui.Color) ui.Element {
	if !p.LiquidGlass {
		return glassFloatingEl(ctx, el, p, radius, 0, false, fallbackBg, fallbackBorder)
	}
	snap := isStaticSnapshot(ctx)
	state := trackGlassInteraction(el, snap)
	if !snap {
		anim := motionGlassRelease
		if el.Pressed() && !el.IsDisabled() {
			anim = motionGlassPress
		}
		scale := 1 + 0.18*clamp32(state.elastic, -0.15, 1.25)
		el.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: scale}, anim)
	}
	return el.Radius(radius).Material(interactiveGlass{
		base:             glass.Glass{},
		glassInteraction: state,
		staticSnapshot:   snap,
	})
}

// GlassBarChrome applies the non-glass header background and bottom border to el when LiquidGlass is disabled.
func GlassBarChrome(el ui.Element, p Palette) ui.Element {
	if p.LiquidGlass {
		return el
	}
	el.Background(p.Surface)
	if p.LineSoft.A > 0 {
		el.BorderWidth(0, 0, 1, 0).BorderColor(p.LineSoft)
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
