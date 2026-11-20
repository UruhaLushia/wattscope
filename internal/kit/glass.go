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
	if clip && !(p.LiquidGlass && interactive) {
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
	glow, hover := state.glow, state.hover
	alpha := 0.08 + 0.07*glow + 0.02*hover
	if p.Dark {
		alpha = 0.24*glow + 0.06*hover
	}
	if alpha > 0.005 && (!p.Dark || glow > 0.01 || hover > 0.01) {
		el.Shadow(0, 2+3*glow, 6+9*glow, 0, ui.RGBA(12, 18, 30, clamp32(alpha, 0, 0.45)))
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

func GlassScrollEdge(ctx *ui.Context, p Palette, bottom bool, offset, height float32, zIndex int) {
	if !p.LiquidGlass || height <= 0 {
		return
	}
	key := "scroll-edge-top"
	if bottom {
		key = "scroll-edge-bottom"
	}
	edge := ui.Box(ctx.Key(key)).Absolute().Left(0).Right(0).Height(height).
		ZIndex(zIndex).PassThrough().
		Material(glass.ScrollEdge{Bottom: bottom, Background: p.Canvas})
	if bottom {
		edge.Bottom(offset)
	} else {
		edge.Top(offset)
	}
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
