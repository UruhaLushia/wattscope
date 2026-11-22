package kit

import "github.com/egoist/mygo/ui"

// BadgeTone selects the semantic color pair for Badge, ValueBadge, and BadgeButton.
type BadgeTone uint8

// Badge tone presets.
const (
	TonePlain BadgeTone = iota
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
)

// ButtonVariant selects the visual hierarchy and fill style for action and menu buttons.
type ButtonVariant uint8

// Button variant presets.
const (
	VariantSecondary ButtonVariant = iota
	VariantPrimary
	VariantDanger
	VariantGhost
)

// PillRadius returns RadiusFull when LiquidGlass is enabled, or solid otherwise.
func PillRadius(p Palette, solid float32) float32 {
	if p.LiquidGlass {
		return RadiusFull
	}
	return solid
}

func glassRadius(p Palette, solid, glass float32) float32 {
	if p.LiquidGlass {
		return glass
	}
	return solid
}

// InteractiveBg returns the hover, focus, or press background color for el.
func InteractiveBg(el ui.Element, p Palette) ui.Color {
	switch {
	case el.Pressed():
		return p.Press
	case el.Hovered() || el.FocusVisible():
		return p.Hover
	default:
		return ui.Transparent
	}
}

// GlassIconPopOnPress animates iconEl with a Liquid Glass press pop while owner is pressed.
func GlassIconPopOnPress(ctx *ui.Context, p Palette, owner, iconEl ui.Element) {
	var pop float32
	if !owner.IsDisabled() && owner.Pressed() {
		pop = 1
	}
	glassIconPop(ctx, iconEl, p, pop)
}

// ApplyCardShadow attaches the palette card drop shadow when non-transparent.
func ApplyCardShadow(el ui.Element, p Palette) ui.Element {
	if p.CardShadow.A > 0 {
		return el.Shadow(0, 1, 3, 0, p.CardShadow)
	}
	return el
}

// Card builds a padded surface card with smooth locale size transitions.
func Card(ctx *ui.Context, p Palette) ui.Element {
	return StaticCard(ctx, p).Transition(localeSizeTransition)
}

// StaticCard builds a Card without locale size transition so scrollable columns can cull off-screen cards.
func StaticCard(ctx *ui.Context, p Palette) ui.Element {
	return GlassPanel(ui.Column(ctx).FillWidth().MinWidth(0).Gap(12).Padding(16), p, RadiusL)
}

// TitledCard builds a Card with a section title header, optional trailing controls, divider, and content body.
func TitledCard(ctx *ui.Context, p Palette, title string, trailing, content func()) ui.Element {
	return Card(ctx, p).Gap(10).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(28).AlignItems(ui.Center).Gap(8).Children(func() {
			SectionTitle(ctx, title)
			ui.Spacer(ctx)
			if trailing != nil {
				trailing()
			}
		})
		Divider(ctx, p)
		if content != nil {
			content()
		}
	})
}

// SectionTitle renders a primary level-2 card section heading.
func SectionTitle(ctx *ui.Context, title string) {
	AnimatedLabel(ctx, title).FontSize(13).FontWeight(700).Role(ui.RoleHeading).Level(2)
}

func sectionHeader(ctx *ui.Context, p Palette, title string, trailing func()) {
	ui.Row(ctx).FillWidth().MinHeight(24).Padding(0, 0, 0, 4).AlignItems(ui.Center).Gap(8).Children(func() {
		AnimatedLabel(ctx, title).FontSize(11).FontWeight(650).LetterSpacing(.4).TextColor(p.Muted).Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

const formGroupInset = 16

// FormGroup renders an optional muted header and grouped rows inside a rounded surface panel.
func FormGroup(ctx *ui.Context, p Palette, title string, trailing, rows func()) {
	ui.Column(ctx).FillWidth().Shrink(0).Gap(6).Children(func() {
		if title != "" {
			sectionHeader(ctx, p, title, trailing)
		}
		GlassPanel(ui.Column(ctx).FillWidth().Padding(0, formGroupInset), p, RadiusL).
			Transition(localeSizeTransition).Children(rows)
	})
}

// FormColumn centers page content within a 1600dp maximum width container.
func FormColumn(ctx *ui.Context, build func()) ui.Element {
	return ui.Column(ctx).FillWidth().MaxWidth(1600).AlignSelf(ui.Center).MinWidth(0).MinHeight(0).Children(build)
}

// NarrowColumn centers page content within an 840dp maximum width column.
func NarrowColumn(ctx *ui.Context, build func()) ui.Element {
	return ui.Column(ctx).FillWidth().MaxWidth(840).AlignSelf(ui.Center).MinWidth(0).Gap(16).Children(build)
}

// SettingRow renders a labeled setting row with an optional hint and trailing control.
func SettingRow(ctx *ui.Context, p Palette, label, hint string, control func()) {
	IconSettingRow(ctx, p, nil, label, hint, control)
}

// IconSettingRow renders a SettingRow with a leading SVG icon.
func IconSettingRow(ctx *ui.Context, p Palette, icon *ui.SVG, label, hint string, control func()) {
	row := ui.Row(ctx).FillWidth().Wrap().MinHeight(52).Padding(10, 0).Gap(12).
		AlignItems(ui.Center).AlignContent(ui.Center).Justify(ui.SpaceBetween).
		Transition(localeSizeTransition)
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

func kvRow(ctx *ui.Context, p Palette, label, value string, mobile bool, trailing func()) {
	if value == "" && trailing == nil {
		return
	}
	minH, padY := float32(42), float32(8)
	if mobile {
		minH, padY = 46, 10
	}
	ui.Row(ctx).FillWidth().MinHeight(minH).Padding(padY, 0).Gap(16).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		ui.Text(ctx, label).FontSize(13).FontWeight(500).TextColor(p.Muted).Shrink(0)
		if value != "" {
			ui.Text(ctx, value).FontSize(13).FontWeight(550).FontFeatures("tnum").
				Grow(1).MinWidth(0).Wrap().TextAlign(ui.End).Selectable()
		}
		if trailing != nil {
			trailing()
		}
	})
}

// KVItem defines a single label-value pair for KVList.
type KVItem struct {
	Label string
	Value string
}

// KVList renders non-empty key-value items separated by dividers.
func KVList(ctx *ui.Context, p Palette, items []KVItem, mobile bool) {
	first := true
	for _, item := range items {
		if item.Value == "" {
			continue
		}
		if !first {
			Divider(ctx, p)
		}
		first = false
		kvRow(ctx, p, item.Label, item.Value, mobile, nil)
	}
}

// ActionBar builds a wrapping horizontal row for toolbar action controls.
func ActionBar(ctx *ui.Context) ui.Element {
	return ui.Row(ctx).FillWidth().Wrap().Gap(8)
}

// Divider renders a 1dp horizontal hairline separator.
func Divider(ctx *ui.Context, p Palette) {
	ui.Box(ctx).FillWidth().Height(1).Shrink(0).Background(p.LineSoft)
}

// VerticalDivider renders a 1dp vertical hairline separator of height.
func VerticalDivider(ctx *ui.Context, p Palette, height float32) {
	ui.Box(ctx).Width(1).Height(height).Shrink(0).Background(p.LineSoft)
}

// BadgeColors returns the background and foreground colors for tone.
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

// Badge renders a pill badge with locale transitions for static or status text.
func Badge(ctx *ui.Context, p Palette, text string, tone BadgeTone) ui.Element {
	return badge(ctx, p, text, tone, true)
}

// ValueBadge shows a live value without animating text or size transitions as it changes.
func ValueBadge(ctx *ui.Context, p Palette, text string, tone BadgeTone) ui.Element {
	return badge(ctx, p, text, tone, false)
}

func badge(ctx *ui.Context, p Palette, text string, tone BadgeTone, animate bool) ui.Element {
	bg, fg := BadgeColors(p, tone)
	box := ui.Box(ctx).Height(22).Shrink(0).Padding(0, 8).AlignItems(ui.Center).Justify(ui.Center).
		Radius(PillRadius(p, RadiusS)).Background(bg)
	if animate {
		box.Transition(localeSizeTransition)
	}
	if p.LiquidGlass && tone == TonePlain {
		box.Background(glassNeutral(p.Dark, 0.05, 0.08)).Border(1, glassNeutral(p.Dark, 0.06, 0.12))
	}
	return box.Children(func() {
		label := ui.Text(ctx, text).FontSize(11).FontWeight(650).TextColor(fg).SingleLine().Shrink(0)
		if animate {
			label.ContentTransition(localeTextTransition)
		}
	})
}

// BadgeButton renders a clickable pill badge of height.
func BadgeButton(ctx *ui.Context, p Palette, text string, tone BadgeTone, height float32, onClick func()) ui.Element {
	bg, fg := BadgeColors(p, tone)
	fontSize := float32(11)
	if height <= 20 {
		fontSize = 10.5
	}
	btn := ui.ButtonBase(ctx).Height(height).Shrink(0).Padding(0, 8).AlignItems(ui.Center).Justify(ui.Center).
		Radius(PillRadius(p, RadiusS)).Background(bg).Label(text).OnClick(onClick)
	return ControlMotion(btn).Children(func() {
		ui.Text(ctx, text).FontSize(fontSize).FontWeight(650).TextColor(fg).SingleLine()
	})
}

// StatusDot renders a circular indicator dot whose fill animates cleanly between color states.
func StatusDot(ctx *ui.Context, color ui.Color, size float32) ui.Element {
	el := ui.Box(ctx).Size(size, size).Shrink(0).Radius(RadiusFull)
	return el.Background(el.AnimateColor("dot", color, MotionFast))
}

// IconBadge renders an icon centered in a square rounded background badge.
func IconBadge(ctx *ui.Context, icon *ui.SVG, size float32, bg, fg ui.Color) ui.Element {
	radius, iconSize := RadiusS, float32(16)
	if size > 32 {
		radius, iconSize = RadiusM, 20
	}
	return ui.Box(ctx).Size(size, size).Shrink(0).Radius(radius).Center().Background(bg).Children(func() {
		ui.Icon(ctx, icon).FontSize(iconSize).TextColor(fg)
	})
}

// MetricSummaryItem holds the label, numeric string, and unit rendered in MetricSummaryRow.
type MetricSummaryItem struct {
	Label string
	Value string
	Unit  string
}

// MetricSummaryOptions configures layout density and wrapping for MetricSummaryRow.
type MetricSummaryOptions struct {
	Compact bool
	Wrap    bool
}

// MetricSummaryRow renders summary metrics separated by vertical dividers.
func MetricSummaryRow(ctx *ui.Context, p Palette, items []MetricSummaryItem, opts MetricSummaryOptions) ui.Element {
	compact, wrap := opts.Compact, opts.Wrap
	gap, divH := float32(12), float32(24)
	if compact {
		gap, divH = 6, 20
	}
	row := ui.Row(ctx).FillWidth().AlignItems(ui.Center).Gap(gap)
	if wrap {
		row.Wrap()
	}
	return row.Children(func() {
		for i, item := range items {
			if i > 0 {
				VerticalDivider(ctx, p, divH)
			}
			if !compact {
				metricSummaryCell(ctx, p, item.Label, item.Value, item.Unit)
				continue
			}
			ui.Column(ctx).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
				ui.Text(ctx, item.Label).FontSize(10).FontWeight(600).TextColor(p.Muted).SingleLine()
				ui.Row(ctx).AlignItems(ui.Baseline).Gap(2).Children(func() {
					ui.Text(ctx, item.Value).FontFeatures("tnum").FontSize(11.5).FontWeight(700).MinWidth(0).SingleLine()
					if item.Unit != "" {
						ui.Text(ctx, item.Unit).FontSize(9.5).FontWeight(600).TextColor(p.Muted).Shrink(0).SingleLine()
					}
				})
			})
		}
	})
}

func metricSummaryCell(ctx *ui.Context, p Palette, label, value, unit string) ui.Element {
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

// EmptyState renders a centered empty-state placeholder with icon badge, title, optional hint, and actions.
func EmptyState(ctx *ui.Context, p Palette, icon *ui.SVG, title, hint string, actions func()) {
	ui.Column(ctx).Grow(1).FillWidth().Padding(24, 16).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Children(func() {
		if icon != nil {
			IconBadge(ctx, icon, 44, p.Inset, p.Muted).Margin(0, 0, 6, 0)
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

func pillSurface(ctx *ui.Context, owner ui.Element, radius float32, bg ui.Color, borderW float32, border ui.Color, pressScale float32) {
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
		surface.TransformOrigin(0.5, 0.5).AnimatePose(pose, motionSpring)
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

// ActionButton renders a themed push button with locale label transitions.
func ActionButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func()) ui.Element {
	return actionButton(ctx, p, variant, icon, label, touch, onClick, true)
}

// ValueButton shows a live value while retaining the button's interaction feedback.
func ValueButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func()) ui.Element {
	return actionButton(ctx, p, variant, icon, label, touch, onClick, false)
}

func resolveButtonStyle(el ui.Element, p Palette, variant ButtonVariant, activeOpen bool) (bg, ink, border, glassTint ui.Color, borderW float32, weight int, visibleAtRest bool) {
	var hover, press ui.Color
	bg, hover, press, ink, border, weight = buttonColors(p, variant)
	glassTint, visibleAtRest = buttonGlassStyle(p, variant)
	if activeOpen && variant != VariantPrimary {
		bg = p.AccentSoft
		ink = p.AccentText
		glassTint, visibleAtRest = GlassAccentTint(p, 0.28, 0.32), true
		if border.A > 0 {
			border = p.AccentLine
		}
	} else if !el.IsDisabled() {
		switch {
		case el.Pressed():
			bg = press
		case el.Hovered() || el.FocusVisible():
			bg = hover
		}
	}
	if border.A > 0 {
		borderW = 1
	}
	return bg, ink, border, glassTint, borderW, weight, visibleAtRest
}

func actionButton(ctx *ui.Context, p Palette, variant ButtonVariant, icon *ui.SVG, label string, touch bool, onClick func(), animateLabel bool) ui.Element {
	radius := PillRadius(p, RadiusM)
	el := ui.ButtonBase(ctx).Height(ControlHeight(touch)).Shrink(0).Padding(0, 12).Gap(7).
		Radius(radius).AlignItems(ui.Center).Justify(ui.Center).Label(label).OnClick(onClick)
	bg, ink, border, glassTint, borderW, weight, visibleAtRest := resolveButtonStyle(el, p, variant, false)
	return ControlMotion(el).Children(func() {
		GlassPillControl(ctx.Key("bg"), el, p, GlassPillOptions{
			Radius:             radius,
			Tint:               glassTint,
			VisibleAtRest:      visibleAtRest,
			FallbackBackground: bg,
			BorderWidth:        borderW,
			FallbackBorder:     border,
			PressScale:         0.96,
		})
		if icon != nil {
			ui.Icon(ctx, icon).FontSize(15).TextColor(ink).Shrink(0)
		}
		if label != "" {
			text := ui.Text(ctx, label).FontSize(13).FontWeight(weight).TextColor(ink).SingleLine().Shrink(0)
			if animateLabel {
				text.ContentTransition(localeTextTransition)
			}
		}
	})
}

// IconActionButton renders a square icon-only action button sized to size.
func IconActionButton(ctx *ui.Context, p Palette, icon *ui.SVG, label string, size float32, onClick func()) ui.Element {
	return iconActionButton(ctx, p, icon, label, size, PillRadius(p, min(RadiusM, max(RadiusS, size/3))), onClick)
}

// RoundIconActionButton renders an icon-only action button with circular pill corners.
func RoundIconActionButton(ctx *ui.Context, p Palette, icon *ui.SVG, label string, size float32, onClick func()) ui.Element {
	return iconActionButton(ctx, p, icon, label, size, RadiusFull, onClick)
}

func iconActionButton(ctx *ui.Context, p Palette, icon *ui.SVG, label string, size, radius float32, onClick func()) ui.Element {
	el := ui.ButtonBase(ctx).Size(size, size).Shrink(0).Radius(radius).Center().Label(label).Tooltip(label).OnClick(onClick)
	bg, fg := ui.Transparent, p.Muted
	switch {
	case el.Pressed():
		bg, fg = p.Press, p.Text
	case el.Hovered() || el.FocusVisible():
		bg, fg = p.Hover, p.Text
	}
	return ControlMotion(el).Children(func() {
		GlassPillControl(ctx.Key("bg"), el, p, GlassPillOptions{
			Radius:             radius,
			FallbackBackground: bg,
			PressScale:         0.94,
		})
		GlassIconPopOnPress(ctx, p, el, ui.Icon(ctx, icon).FontSize(16).TextColor(el.AnimateColor("icon-fg", fg, MotionFast)))
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
		Transition(localeSizeTransition)
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
			glassSelectionPill(ctx.Key("seg-pill"), seg, p, segRadius, p.TrackOn, matchKey, true)
		} else {
			GlassPillControl(ctx.Key("seg-state"), seg, p, GlassPillOptions{
				Radius:             segRadius,
				FallbackBackground: bg,
				PressScale:         0.96,
			})
		}
		AnimatedLabel(ctx, label).FontSize(11.5).FontWeight(weight).
			TextColor(seg.AnimateColor("seg-fg", fg, MotionFast)).SingleLine().Shrink(0)
	})
}

// Segmented renders a sliding pill segmented control bound to selected.
func Segmented(ctx *ui.Context, p Palette, selected *int, labels []string, touch bool, onChange func()) ui.Element {
	parts := ui.SegmentedBase(ctx, selected, len(labels))
	trackH := float32(26)
	if touch {
		trackH = 30
	}
	trackRadius := PillRadius(p, RadiusM)
	trackBg := p.Track
	if p.LiquidGlass {
		trackBg = glassNeutral(p.Dark, 0.06, 0.08)
	}
	track := parts.Track.Height(trackH).Shrink(0).Padding(3).Gap(2).
		Radius(trackRadius).Background(trackBg).Transition(localeSizeTransition)
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

// ColorSwatch renders a selectable circular color swatch with an active halo ring.
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

// ToastViewport renders active toast notifications aligned to the bottom-end or bottom-center safe area.
func ToastViewport(ctx *ui.Context, p Palette, bottom float32, centered bool) {
	topInset, rightInset, _, leftInset := ctx.SafeArea()
	ui.ToastViewportBase(ctx, func(viewport ui.Element, toasts []ui.Toast) {
		align := ui.End
		if centered {
			align = ui.Center
		}
		viewport.Padding(max(16, topInset+8), max(16, rightInset+12), bottom, max(16, leftInset+12)).Gap(8).AlignItems(align).Justify(ui.End)
		for index := range toasts {
			notice := toasts[index]
			parts := ui.ToastBase(ctx, notice)
			popupMotion(parts.Root)
			toastRadius := glassRadius(p, RadiusL, RadiusXL)
			glassFloatingEl(ctx, parts.Root.Width(340).MaxWidthPercent(94).Row().AlignItems(ui.Center).Gap(10).
				Padding(10, 12, 10, 14), p, toastRadius, 0.1, true, p.Surface, p.Line).
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
