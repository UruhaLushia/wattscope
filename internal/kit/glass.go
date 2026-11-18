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
	el.Clip().Radius(radius)
	if p.LiquidGlass {
		return el.Material(glass.Glass{
			Tint:        tint,
			Clarity:     clarity,
			Interactive: interactive,
		})
	}
	el.Background(fallbackBg)
	if fallbackBorder.A > 0 {
		el.Border(1, fallbackBorder)
	}
	return el
}

func GlassFloatingButton(el ui.Element, p Palette, radius float32, tint ui.Color, fallbackBg, fallbackBorder ui.Color) ui.Element {
	return GlassFloating(el, p, radius, 0, tint, true, fallbackBg, fallbackBorder)
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

func GlassPillControl(ctx *ui.Context, owner ui.Element, _ Palette, radius float32, _ ui.Color, _ bool, fallbackBg ui.Color, borderW float32, fallbackBorder ui.Color, pressScale float32) {
	PillSurface(ctx, owner, radius, fallbackBg, borderW, fallbackBorder, pressScale)
}

func GlassAccentTint(p Palette, lightAlpha, darkAlpha float32) ui.Color {
	if p.Dark {
		return p.Accent.Alpha(darkAlpha)
	}
	return p.Accent.Alpha(lightAlpha)
}
