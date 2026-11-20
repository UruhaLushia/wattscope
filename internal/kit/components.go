package kit

import "github.com/egoist/mygo/ui"

type BadgeTone uint8

const (
	TonePlain BadgeTone = iota
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
)

type ButtonVariant uint8

const (
	VariantSecondary ButtonVariant = iota
	VariantPrimary
	VariantDanger
	VariantGhost
)

func ApplyCardShadow(el ui.Element, p Palette) ui.Element {
	if p.CardShadow.A > 0 {
		return el.Shadow(0, 1, 3, 0, p.CardShadow)
	}
	return el
}

func Card(ctx *ui.Context, p Palette) ui.Element {
	return GlassPanel(ui.Column(ctx).FillWidth().MinWidth(0).Gap(12).Padding(16), p, RadiusL, 0.25, p.Surface, p.LineSoft).
		Transition(LocaleSizeTransition)
}

func Panel(ctx *ui.Context, p Palette, title string, content func()) {
	Card(ctx, p).Children(func() {
		SectionTitle(ctx, title)
		content()
	})
}

func SectionTitle(ctx *ui.Context, title string) {
	AnimatedLabel(ctx, title).FontSize(13).FontWeight(700).Role(ui.RoleHeading).Level(2)
}

func SectionHeader(ctx *ui.Context, p Palette, title string, trailing func()) {
	ui.Row(ctx).FillWidth().MinHeight(24).AlignItems(ui.Center).Gap(8).Children(func() {
		AnimatedLabel(ctx, title).FontSize(11).FontWeight(650).LetterSpacing(.4).TextColor(p.Muted).Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

const formGroupInset = 16

func FormGroup(ctx *ui.Context, p Palette, title string, trailing, rows func()) {
	ui.Column(ctx).FillWidth().Shrink(0).Gap(6).Children(func() {
		if title != "" {
			SectionHeader(ctx, p, title, trailing)
		}
		GlassPanel(ui.Column(ctx).FillWidth().Padding(0, formGroupInset), p, RadiusL, 0.25, p.Surface, p.LineSoft).
			Transition(LocaleSizeTransition).Children(rows)
	})
}

func FormColumn(ctx *ui.Context, build func()) ui.Element {
	return ui.Column(ctx).FillWidth().MaxWidth(1600).AlignSelf(ui.Center).MinWidth(0).MinHeight(0).Children(build)
}

func NarrowColumn(ctx *ui.Context, build func()) ui.Element {
	return ui.Column(ctx).FillWidth().MaxWidth(840).AlignSelf(ui.Center).MinWidth(0).Gap(16).Children(build)
}

func SettingRow(ctx *ui.Context, p Palette, label, hint string, control func()) {
	IconSettingRow(ctx, p, nil, label, hint, control)
}

func IconSettingRow(ctx *ui.Context, p Palette, icon *ui.SVG, label, hint string, control func()) {
	row := ui.Row(ctx).FillWidth().Wrap().MinHeight(52).Padding(10, 0).Gap(12).
		AlignItems(ui.Center).AlignContent(ui.Center).Justify(ui.SpaceBetween).
		Transition(LocaleSizeTransition)
	row.Children(func() {
		ui.Row(ctx).Grow(1).Basis(160).MinWidth(140).Gap(10).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(ctx, icon).FontSize(16).TextColor(p.Muted).Shrink(0)
			}
			ui.Column(ctx).Grow(1).MinWidth(0).Gap(2).Justify(ui.Center).Children(func() {
				AnimatedLabel(ctx, label).FontSize(13).FontWeight(550)
				if hint != "" {
					AnimatedLabel(ctx, hint).FontSize(11).TextColor(p.Muted).Wrap().MaxLines(2)
				}
			})
		})
		ui.Row(ctx).Shrink(0).MaxWidthPercent(100).Wrap().Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(control)
	})
}

func PropertyRow(ctx *ui.Context, p Palette, label, value string, trailing func()) {
	KVRow(ctx, p, label, value, false, trailing)
}

func KVRow(ctx *ui.Context, p Palette, label, value string, mobile bool, trailing func()) {
	if value == "" && trailing == nil {
		return
	}
	if mobile {
		ui.Row(ctx).FillWidth().MinHeight(46).Padding(10, 0).Gap(12).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Text(ctx, label).FontSize(13).FontWeight(500).TextColor(p.Muted).Shrink(0)
			if value != "" {
				ui.Text(ctx, value).FontSize(13).FontWeight(600).FontFeatures("tnum").
					Grow(1).MinWidth(0).Wrap().TextAlign(ui.End).Selectable()
			}
			if trailing != nil {
				trailing()
			}
		})
		return
	}
	ui.Row(ctx).FillWidth().Wrap().MinHeight(42).Padding(8, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(ctx, label).FontSize(12).FontWeight(600).TextColor(p.Muted).MinWidth(104).Shrink(0)
		ui.Text(ctx, value).FontSize(13).Grow(1).Basis(160).MinWidth(0).Selectable()
		if trailing != nil {
			trailing()
		}
	})
}

func ActionBar(ctx *ui.Context) ui.Element {
	return ui.Row(ctx).FillWidth().Wrap().Gap(8)
}

func Divider(ctx *ui.Context, p Palette) {
	ui.Box(ctx).FillWidth().Height(1).Shrink(0).Background(p.LineSoft)
}

func VerticalDivider(ctx *ui.Context, p Palette, height float32) {
	ui.Box(ctx).Width(1).Height(height).Shrink(0).Background(p.LineSoft)
}

func BadgeColors(p Palette, tone BadgeTone) (bg, fg ui.Color) {
	switch tone {
	case ToneAccent:
		return p.AccentSoft, p.AccentText
	case ToneSuccess:
		return p.GreenSoft, p.Green
	case ToneWarning:
		return p.WarningSoft, p.Warning
	case ToneDanger:
		return p.RedSoft, p.Red
	default:
		return p.Inset, p.Muted
	}
}

func Badge(ctx *ui.Context, p Palette, text string, tone BadgeTone) ui.Element {
	return badge(ctx, p, text, tone, true)
}

// ValueBadge shows a live value without animating it as it changes.
func ValueBadge(ctx *ui.Context, p Palette, text string, tone BadgeTone) ui.Element {
	return badge(ctx, p, text, tone, false)
}

func badge(ctx *ui.Context, p Palette, text string, tone BadgeTone, animate bool) ui.Element {
	bg, fg := BadgeColors(p, tone)
	radius := RadiusS
	if p.LiquidGlass {
		radius = RadiusFull
	}
	box := ui.Box(ctx).Height(22).Shrink(0).Padding(0, 8).AlignItems(ui.Center).Justify(ui.Center).
		Radius(radius).Background(bg)
	if animate {
		box.Transition(LocaleSizeTransition)
	}
	if p.LiquidGlass && tone == TonePlain {
		if p.Dark {
			box.Background(ui.RGBA(255, 255, 255, 0.08)).Border(1, ui.RGBA(255, 255, 255, 0.12))
		} else {
			box.Background(ui.RGBA(15, 23, 42, 0.05)).Border(1, ui.RGBA(15, 23, 42, 0.06))
		}
	}
	return box.Children(func() {
		label := ui.Text(ctx, text).FontSize(11).FontWeight(650).TextColor(fg).SingleLine().Shrink(0)
		if animate {
			label.ContentTransition(LocaleTextTransition)
		}
	})
}

func StatusDot(ctx *ui.Context, color ui.Color, size float32) ui.Element {
	el := ui.Box(ctx).Size(size, size).Shrink(0).Radius(RadiusFull)
	return el.Background(el.AnimateColor("dot", color, MotionFast))
}

func IconBadge(ctx *ui.Context, p Palette, icon *ui.SVG, size float32, bg, fg ui.Color) ui.Element {
	radius, iconSize := RadiusS, float32(16)
	if size > 32 {
		radius, iconSize = RadiusM, 20
	}
	return ui.Box(ctx).Size(size, size).Shrink(0).Radius(radius).Center().Background(bg).Children(func() {
		ui.Icon(ctx, icon).FontSize(iconSize).TextColor(fg)
	})
}

func MetricSummaryCell(ctx *ui.Context, p Palette, label, value, unit string) ui.Element {
	return ui.Column(ctx).Grow(1).Basis(80).MinWidth(70).Gap(2).Children(func() {
		ui.Text(ctx, label).FontSize(11).FontWeight(600).TextColor(p.Muted).SingleLine()
		ui.Row(ctx).AlignItems(ui.Baseline).Gap(4).Children(func() {
			ui.Text(ctx, value).FontFeatures("tnum").FontSize(15).FontWeight(700).SingleLine()
			if unit != "" {
				ui.Text(ctx, unit).FontSize(11).FontWeight(600).TextColor(p.Muted).SingleLine()
			}
		})
	})
}

func EmptyState(ctx *ui.Context, p Palette, icon *ui.SVG, title, hint string, actions func()) {
	ui.Column(ctx).Grow(1).FillWidth().Padding(24, 16).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
		if icon != nil {
			IconBadge(ctx, p, icon, 44, p.Inset, p.Muted).Margin(0, 0, 6, 0)
		}
		ui.Text(ctx, title).FontSize(14).FontWeight(700).TextAlign(ui.Center)
		if hint != "" {
			ui.Text(ctx, hint).FontSize(12).TextColor(p.Muted).MaxWidth(360).Wrap().TextAlign(ui.Center)
		}
		if actions != nil {
			ui.Row(ctx).Margin(10, 0, 0, 0).Wrap().Gap(8).Justify(ui.Center).Children(actions)
		}
	})
}

func buttonColors(p Palette, variant ButtonVariant) (bg, hover, press, ink, border ui.Color, weight int) {
	switch variant {
	case VariantPrimary:
		return p.Accent, p.AccentHi, p.Accent, p.AccentInk, ui.Transparent, 650
	case VariantDanger:
		return p.RedSoft, p.Red.Alpha(.22), p.Red.Alpha(.30), p.Red, ui.Transparent, 650
	case VariantGhost:
		return ui.Transparent, p.Hover, p.Press, p.Text, ui.Transparent, 550
	default:
		hoverMix, pressMix := float32(.06), float32(.13)
		if p.Dark {
			hoverMix, pressMix = .10, .18
		}
		return p.Track, p.Track.Mix(p.Accent, hoverMix), p.Track.Mix(p.Accent, pressMix), p.Text, p.LineSoft, 550
	}
}

// PillSurface renders a vector-only background and border layer inside interactive
// controls so press scaling animates the surface without re-bucketing child glyphs.
func PillSurface(ctx *ui.Context, owner ui.Element, radius float32, bg ui.Color, borderW float32, border ui.Color, pressScale float32) {
	surface := ui.Box(ctx).Absolute().Fill().ZIndex(-1).
		Radius(radius).PassThrough()
	surface.Background(surface.AnimateColor("pill-bg", bg, MotionFast))
	if borderW > 0 {
		surface.Border(borderW, surface.AnimateColor("pill-bd", border, MotionFast))
	}
	if pressScale > 0 && pressScale != 1 {
		pose := ui.Pose{}
		if !owner.IsDisabled() && owner.Pressed() {
			pose.Scale = pressScale
		}
		surface.TransformOrigin(0.5, 0.5).AnimatePose(pose, MotionSpring)
	}
}

func buttonGlassStyle(p Palette, variant ButtonVariant) (tint ui.Color, visibleAtRest bool) {
	switch variant {
	case VariantPrimary:
		return GlassAccentTint(p, 0.72, 0.64), true
	case VariantDanger:
		return p.Red.Alpha(0.28), true
	case VariantGhost:
		return ui.Color{}, false
	default:
		return ui.Color{}, true
	}
}

func ActionButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func()) ui.Element {
	return actionButton(ctx, p, variant, icon, label, touch, onClick, true)
}

// ValueButton shows a live value while retaining the button's interaction feedback.
func ValueButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func()) ui.Element {
	return actionButton(ctx, p, variant, icon, label, touch, onClick, false)
}

func actionButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func(), animateLabel bool) ui.Element {
	radius := RadiusM
	if p.LiquidGlass {
		radius = RadiusFull
	}
	el := ui.ButtonBase(ctx).Height(ControlHeight(touch)).Shrink(0).Padding(0, 12).Gap(7).
		Radius(radius).AlignItems(ui.Center).Justify(ui.Center).Label(label).OnClick(onClick)
	bg, hover, press, ink, border, weight := buttonColors(p, variant)
	if !el.IsDisabled() {
		switch {
		case el.Pressed():
			bg = press
		case el.Hovered() || el.FocusVisible():
			bg = hover
		}
	}
	borderW := float32(0)
	if border.A > 0 {
		borderW = 1
	}
	glassTint, visibleAtRest := buttonGlassStyle(p, variant)
	return ControlMotion(el).Children(func() {
		GlassPillControl(ctx.Key("bg"), el, p, radius, glassTint, visibleAtRest, bg, borderW, border, 0.96)
		if icon != nil {
			ui.Icon(ctx, icon).FontSize(15).TextColor(ink).Shrink(0)
		}
		if label != "" {
			text := ui.Text(ctx, label).FontSize(13).FontWeight(weight).TextColor(ink).SingleLine().Shrink(0)
			if animateLabel {
				text.ContentTransition(LocaleTextTransition)
			}
		}
	})
}

func IconActionButton(ctx *ui.Context, p Palette, icon *ui.SVG, label string, size float32, active bool, onClick func()) ui.Element {
	radius := min(RadiusM, max(RadiusS, size/3))
	if p.LiquidGlass {
		radius = RadiusFull
	}
	el := ui.ButtonBase(ctx).Size(size, size).Shrink(0).Radius(radius).Center().Label(label).Tooltip(label).OnClick(onClick)
	bg, fg := ui.Transparent, p.Muted
	switch {
	case active:
		bg, fg = p.AccentSoft, p.AccentText
	case el.Pressed():
		bg, fg = p.Press, p.Text
	case el.Hovered() || el.FocusVisible():
		bg, fg = p.Hover, p.Text
	}
	var glassTint ui.Color
	visibleAtRest := false
	if active {
		glassTint = GlassAccentTint(p, 0.28, 0.32)
		visibleAtRest = true
	}
	return ControlMotion(el).Children(func() {
		GlassPillControl(ctx.Key("bg"), el, p, radius, glassTint, visibleAtRest, bg, 0, ui.Transparent, 0.94)
		pop := float32(0)
		if !el.IsDisabled() && el.Pressed() {
			pop = 1
		}
		GlassIconPop(ui.Icon(ctx, icon).FontSize(16).TextColor(el.AnimateColor("icon-fg", fg, MotionFast)), p, pop)
	})
}

type segTrackKey struct{}

func segmentStyle(ctx *ui.Context, p Palette, seg ui.Element, label string, active, touch bool, matchKey *segTrackKey) ui.Element {
	segH, padX := float32(20), float32(8)
	if touch {
		segH, padX = 24, 10
	}
	segRadius := RadiusS
	if p.LiquidGlass {
		segRadius = RadiusFull
	}
	seg.Height(segH).Shrink(0).Padding(0, padX).
		Radius(segRadius).AlignItems(ui.Center).Justify(ui.Center).
		Transition(LocaleSizeTransition)
	bg, fg, weight := ui.Transparent, p.Muted, 550
	switch {
	case active && seg.Pressed():
		seg.ZIndex(0)
		fg = p.TrackOnInk.Alpha(0.90)
	case active:
		seg.ZIndex(0)
		fg = p.TrackOnInk
	case seg.Pressed():
		seg.ZIndex(1)
		bg, fg = p.Press, p.Text
	case seg.Hovered() || seg.FocusVisible():
		seg.ZIndex(1)
		bg, fg = p.Hover, p.Text
	default:
		seg.ZIndex(1)
	}
	return seg.Children(func() {
		if active {
			GlassSelectionPill(ctx.Key("seg-pill"), seg, p, segRadius, p.TrackOn, matchKey, true)
		} else {
			GlassPillControl(ctx.Key("seg-state"), seg, p, segRadius, ui.Color{}, false, bg, 0, ui.Transparent, 0.96)
		}
		AnimatedLabel(ctx, label).FontSize(11.5).FontWeight(weight).
			TextColor(seg.AnimateColor("seg-fg", fg, MotionFast)).SingleLine().Shrink(0)
	})
}

func Segmented(ctx *ui.Context, p Palette, selected *int, labels []string, touch bool, onChange func()) ui.Element {
	parts := ui.SegmentedBase(ctx, selected, len(labels))
	trackH := float32(26)
	if touch {
		trackH = 30
	}
	trackRadius := RadiusM
	trackBg := p.Track
	if p.LiquidGlass {
		trackRadius = RadiusFull
		if p.Dark {
			trackBg = ui.RGBA(255, 255, 255, 0.08)
		} else {
			trackBg = ui.RGBA(15, 23, 42, 0.06)
		}
	}
	track := parts.Track.Height(trackH).Shrink(0).Padding(3).Gap(2).
		Radius(trackRadius).Background(trackBg).Transition(LocaleSizeTransition)
	matchKey := ui.Local(track, "seg-id", func() segTrackKey { return segTrackKey{} })
	if onChange != nil {
		track.OnChange(onChange)
	}
	track.Children(func() {
		for index, label := range labels {
			segmentStyle(ctx, p, parts.Segment(index), label, index == *selected, touch, matchKey)
		}
	})
	return track
}

func ColorSwatch(ctx *ui.Context, p Palette, color ui.Color, selected bool, onClick func()) ui.Element {
	el := ui.ButtonBase(ctx).Size(24, 24).Shrink(0).Radius(RadiusFull).Background(color).
		Label(HexColor(color)).OnClick(onClick)
	if selected || el.FocusVisible() {
		el.Draw(func(painter *ui.Painter, rect ui.Rect) {
			painter.Stroke(ui.Rect{X: rect.X - 2, Y: rect.Y - 2, W: rect.W + 4, H: rect.H + 4}, p.Surface, RadiusFull, 2)
			painter.Stroke(ui.Rect{X: rect.X - 4, Y: rect.Y - 4, W: rect.W + 8, H: rect.H + 8}, color, RadiusFull, 2)
		})
	}
	return el
}

func ToastViewport(ctx *ui.Context, p Palette, bottom float32, centered bool) {
	ui.ToastViewportBase(ctx, func(viewport ui.Element, toasts []ui.Toast) {
		align := ui.End
		if centered {
			align = ui.Center
		}
		viewport.Padding(16, 16, bottom, 16).Gap(8).AlignItems(align).Justify(ui.End)
		for index := range toasts {
			notice := toasts[index]
			parts := ui.ToastBase(ctx, notice)
			PopupMotion(parts.Root)
			toastRadius := RadiusL
			if p.LiquidGlass {
				toastRadius = RadiusXL
			}
			GlassFloating(parts.Root.Width(340).MaxWidthPercent(94).Row().AlignItems(ui.Center).Gap(10).
				Padding(10, 12, 10, 14), p, toastRadius, 0.1, ui.Color{}, true, p.Surface, p.Line).
				Shadow(0, 8, 24, 0, p.Shadow)
			parts.Root.Children(func() {
				icon, tone := IconCheck, p.Green
				switch notice.Type {
				case "error":
					icon, tone = IconClose, p.Red
				case "info":
					icon, tone = IconActivity, p.AccentText
				}
				ui.Icon(ctx, icon).FontSize(16).TextColor(tone).Shrink(0)
				ui.Column(ctx).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(ctx, notice.Title).FontSize(13).FontWeight(650).MaxLines(2).Wrap()
					if notice.Description != "" {
						ui.Text(ctx, notice.Description).FontSize(11).TextColor(p.Muted).MaxLines(2).Wrap()
					}
				})
				parts.CloseButton().Size(24, 24).Shrink(0).Radius(RadiusS).Center().Children(func() {
					ui.Icon(ctx, IconClose).FontSize(12).TextColor(p.Muted)
				})
			})
		}
	})
}
