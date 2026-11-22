package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

type MenuItem struct {
	Value    string
	Label    string
	Icon     *ui.SVG
	Radio    bool
	Check    bool
	Active   bool
	Danger   bool
	Disabled bool
	KeepOpen bool
	OnSelect func()
}

func SurfaceChrome(ctx *ui.Context, el ui.Element, p Palette, radius float32) ui.Element {
	return GlassFloating(ctx, el, p, radius, 0.1, ui.Color{}, false, p.Surface, p.Line.Mix(p.Text, 0.08)).
		Shadow(0, 10, 28, 0, p.Shadow)
}

func animatedChevron(ctx *ui.Context, open bool, color ui.Color, size float32) {
	angle := float32(0)
	if open {
		angle = 180
	}
	chev := ui.Box(ctx).Shrink(0).Center().TransformOrigin(0.5, 0.5)
	progress := chev.AnimateTo("chev-rot", angle, MotionPopup)
	chev.Rotate(progress).Children(func() {
		ui.Icon(ctx, IconChevronDown).FontSize(size).TextColor(color)
	})
}

func menuItemMetrics(touch, optionStyle bool) (minH, fontSize, padY, padX float32) {
	if optionStyle {
		return 36, 14, 5, 12
	}
	minH, fontSize, padY, padX = 36, 13, 7, 10
	if touch {
		minH, fontSize = 42, 14
	}
	return
}

func adaptiveMenuWidth(ctx *ui.Context, items []MenuItem, touch, optionStyle bool) float32 {
	_, fontSize, _, padX := menuItemMetrics(touch, optionStyle)
	maxLabelW := float32(0)
	hasIcon := false
	hasCheck := false
	for _, item := range items {
		if item.Icon != nil {
			hasIcon = true
		}
		if item.Active || item.Check || item.Radio {
			hasCheck = true
		}
		w, _ := ctx.MeasureText(0, ui.Span{Text: item.Label, Size: fontSize, Weight: 650})
		if w > maxLabelW {
			maxLabelW = w
		}
	}
	const panelInsets = 2 * (6 + 1)
	const textSlack = 2
	width := maxLabelW + padX*2 + panelInsets + textSlack
	if hasIcon {
		width += 15 + 10
	}
	if hasCheck {
		trailGap := float32(14)
		if optionStyle {
			trailGap = 16
		}
		width += 16 + trailGap
	}
	minW := float32(88)
	if optionStyle {
		minW = 96
	}
	windowW, _ := ctx.Size()
	maxW := min(260, max(1, windowW-16))
	return min(maxW, max(minW, float32(math.Ceil(float64(width)))))
}

func styleMenuRow(ctx *ui.Context, p Palette, row ui.Element, entry MenuItem, fontSize, minH, padY, padX float32, highlighted, optionStyle bool) {
	row.FillWidth().MinHeight(minH).Padding(padY, padX).Gap(10).Radius(RadiusM).
		AlignItems(ui.Center).Disabled(entry.Disabled).Label(entry.Label)

	bg, fg, weight := ui.Transparent, p.Text, 500
	switch {
	case entry.Disabled:
		fg = p.Faint
	case entry.Active && !entry.Danger:
		weight = 650
		if optionStyle {
			fg = p.Text
			if row.Pressed() {
				bg = p.Press
			} else if highlighted {
				bg = p.Hover
			}
		} else {
			bg, fg = p.AccentSoft, p.AccentText
		}
	case entry.Danger:
		fg = p.Red
		if row.Pressed() {
			bg = p.RedSoft
		} else if highlighted {
			bg = p.RedSoft.Alpha(.65)
		}
	case row.Pressed():
		bg = p.Press
	case highlighted:
		bg = p.Hover
	}

	row.Background(row.AnimateColor("km-bg", bg, MotionFast))
	row.Children(func() {
		if entry.Icon != nil {
			iconColor := fg
			if optionStyle {
				iconColor = p.Muted
			}
			ui.Icon(ctx, entry.Icon).FontSize(15).TextColor(iconColor).Shrink(0)
		}
		ui.Text(ctx, entry.Label).FontSize(fontSize).FontWeight(weight).TextColor(fg).
			Grow(1).MinWidth(0).Wrap()
		if entry.Active {
			checkColor := p.AccentText
			if optionStyle {
				checkColor = p.Text
			}
			ui.Icon(ctx, IconCheck).FontSize(16).TextColor(checkColor).Shrink(0)
		}
	})
}

func MenuPopup(ctx *ui.Context, p Palette, menu ui.MenuParts, anchor ui.Element, touch bool, items []MenuItem) {
	menu.Popup(func(panel ui.Element) {
		panelW := adaptiveMenuWidth(ctx, items, touch, false)
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0).
			Scrollbars(false).Width(panelW).Padding(6).Gap(2)
		PopupMotion(panel)
		SurfaceChrome(ctx, panel, p, RadiusXL)

		itemMinH, fontSize, padY, padX := menuItemMetrics(touch, false)
		for i, item := range items {
			entry := item
			ctx.Key(i)
			var row ui.Element
			switch {
			case entry.Check:
				row = menu.CheckItem(entry.Active)
			case entry.Radio:
				row = menu.RadioItem(entry.Active)
			default:
				row = menu.Item()
			}
			if entry.KeepOpen {
				row.KeepOpen()
			}
			styleMenuRow(ctx, p, row, entry, fontSize, itemMinH, padY, padX, row.Highlighted(), false)
			if row.Clicked() && entry.OnSelect != nil {
				RecordClickOrigin(row)
				entry.OnSelect()
			}
		}
	})
}

func optionMenuPopup(ctx *ui.Context, p Palette, trigger ui.Element, open *bool, topInset float32, items []MenuItem) {
	ui.PopoverBase(ctx, trigger, open, func(panel ui.Element) {
		panelW := adaptiveMenuWidth(ctx, items, false, true)
		panel.AttachTo(trigger, ui.AnchorTopEnd, ui.AnchorTopEnd).Margin(-topInset-3, -formGroupInset-3, 0, 0).
			Scrollbars(false).Width(panelW).Padding(5, 6).Gap(1)
		SurfaceChrome(ctx, panel, p, RadiusXL)
		OptionMenuMotion(panel)

		itemMinH, fontSize, padY, padX := menuItemMetrics(false, true)
		for i, item := range items {
			entry := item
			row := ui.ButtonBase(ctx.Key(i)).Role(ui.RoleMenuItemRadio).Selected(entry.Active)
			styleMenuRow(ctx, p, row, entry, fontSize, itemMinH, padY, padX, row.Hovered() || row.FocusVisible(), true)
			row.OnClick(func() {
				if entry.OnSelect != nil {
					RecordClickOrigin(row)
					entry.OnSelect()
				}
				if !entry.KeepOpen {
					*open = false
					ctx.After(time.Nanosecond)
				}
			})
		}
	})
}

func MenuButton(ctx *ui.Context, p Palette, open *bool, label string, icon *ui.SVG, variant ButtonVariant, touch bool, items []MenuItem) ui.Element {
	return menuButton(ctx, p, open, label, icon, 0, variant, touch, items, false)
}

// IconMenuButton renders a compact square/pill icon-only menu trigger without text or chevron.
func IconMenuButton(ctx *ui.Context, p Palette, open *bool, label string, icon *ui.SVG, size float32, variant ButtonVariant, touch bool, items []MenuItem) ui.Element {
	return menuButton(ctx, p, open, label, icon, size, variant, touch, items, true)
}

func menuButton(ctx *ui.Context, p Palette, open *bool, label string, icon *ui.SVG, size float32, variant ButtonVariant, touch bool, items []MenuItem, iconOnly bool) ui.Element {
	btnRadius := PillRadius(p, RadiusM)
	menu := ui.MenuBase(ctx, open)
	trigger := menu.Trigger.Shrink(0).Radius(btnRadius).AlignItems(ui.Center).Justify(ui.Center).Label(label)
	if iconOnly {
		trigger.Size(size, size).Tooltip(label)
	} else {
		trigger.Height(ControlHeight(touch)).Padding(0, 9, 0, 11).Gap(4)
		if label == "" {
			trigger.Padding(0, 9)
		}
	}

	bg, ink, border, glassTint, borderW, weight, visibleAtRest := resolveButtonStyle(trigger, p, variant, menu.Open())
	chevColor := p.Muted
	if menu.Open() && variant != VariantPrimary {
		chevColor = ink
	}
	ControlMotion(trigger).Children(func() {
		GlassPillControl(ctx.Key("bg"), trigger, p, btnRadius, glassTint, visibleAtRest, bg, borderW, border, 0.96)
		if icon != nil {
			iconSize := float32(14)
			if iconOnly {
				iconSize = 15
			}
			ui.Icon(ctx, icon).FontSize(iconSize).TextColor(ink).Shrink(0)
		}
		if !iconOnly {
			if label != "" {
				AnimatedLabel(ctx, label).FontSize(13).FontWeight(weight).TextColor(ink).SingleLine().Shrink(0)
			}
			animatedChevron(ctx, menu.Open(), chevColor, 14)
		}
	})
	MenuPopup(ctx, p, menu, trigger, touch, items)
	return trigger
}

func OptionRow(ctx *ui.Context, p Palette, open *bool, icon *ui.SVG, label, value string, touch bool, items []MenuItem) {
	if open == nil {
		var scratch bool
		open = &scratch
	}
	minH, padY, labelSize, valueSize := float32(48), float32(8), float32(13), float32(13)
	if touch {
		minH, padY, labelSize, valueSize = 52, 10, 14, 14
	}
	row := ui.Row(ctx).FillWidth().MinHeight(minH).Padding(padY, 0).Gap(12).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).Transition(LocaleSizeTransition)

	var trigger ui.Element
	row.Children(func() {
		ui.Row(ctx).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(10).Children(func() {
			if icon != nil {
				ui.Icon(ctx, icon).FontSize(16).TextColor(p.Muted).Shrink(0)
			}
			AnimatedLabel(ctx, label).FontSize(labelSize).FontWeight(550).SingleLine()
		})

		targetAlpha, chipAnim := float32(1), MotionChipIn
		if *open {
			targetAlpha, chipAnim = 0, MotionChipOut
		}
		chipWrap := ui.Box(ctx.Key("slot")).Shrink(0).Transition(LocaleSizeTransition)
		chipOpacity := chipWrap.AnimateTo("koc-a", targetAlpha, chipAnim)
		chipWrap.Opacity(min(chipOpacity, targetAlpha)).Children(func() {
			chipRadius := PillRadius(p, RadiusM+2)
			trigger = ui.ButtonBase(ctx.Key("chip")).Shrink(0).Padding(6, 10).Gap(5).Radius(chipRadius).
				AlignItems(ui.Center).Label(label + " · " + value).Expanded(*open)

			type optionState struct {
				openOnPress bool
			}
			st := ui.Local(trigger, "opt-press", func() optionState { return optionState{} })
			if trigger.Pressed() && *open {
				st.openOnPress = true
			} else if !trigger.Pressed() && !trigger.Clicked() {
				st.openOnPress = false
			}
			trigger.OnClick(func() {
				if st.openOnPress {
					st.openOnPress = false
					*open = false
					return
				}
				*open = !*open
			})

			chipBg := InteractiveBg(trigger, p)
			ControlMotion(trigger).Children(func() {
				GlassPillControl(ctx.Key("bg"), trigger, p, chipRadius, ui.Color{}, p.LiquidGlass, chipBg, 0, ui.Transparent, 0.96)
				AnimatedLabel(ctx, value).FontSize(valueSize).FontWeight(550).TextColor(p.Muted).SingleLine()
				ui.Icon(ctx, IconChevronsUpDown).FontSize(13).TextColor(p.Muted).Shrink(0)
			})
		})
	})
	optionMenuPopup(ctx, p, trigger, open, padY, items)
}

func Popover(ctx *ui.Context, p Palette, anchor ui.Element, open *bool, build func(panel ui.Element)) {
	ui.PopoverBase(ctx, anchor, open, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0)
		PopupMotion(panel)
		SurfaceChrome(ctx, panel, p, RadiusL)
		build(panel)
	})
}

func Dialog(ctx *ui.Context, p Palette, open *bool, maxWidth, maxHeight float32, content func()) {
	width, height := ctx.Size()
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	ui.DialogBase(ctx, open, func(back, panel ui.Element) {
		BackdropMotion(back, p)
		panelWidth := min(max(240, width-leftInset-rightInset-32), maxWidth)
		panelHeight := min(max(200, height-topInset-bottomInset-40), maxHeight)
		pad := float32(20)
		if width < 480 {
			pad = 16
		}
		dialogRadius := GlassRadius(p, RadiusL, RadiusXL)
		GlassFloating(ctx, ModalMotion(panel).Width(panelWidth).MaxHeight(panelHeight).Padding(pad).Gap(14),
			p, dialogRadius, 0.15, ui.Color{}, false, p.Surface, p.Line).
			Shadow(0, 14, 36, 0, p.Shadow).
			Children(content)
	})
}

func BottomSheet(ctx *ui.Context, p Palette, open *bool, maxHeightFrac float32, closeLabel string, content func()) {
	_, height := ctx.Size()
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	ui.DialogBase(ctx, open, func(back, panel ui.Element) {
		BackdropMotion(back, p).AlignItems(ui.Stretch).Justify(ui.End)
		availH := max(160, height-topInset-24)
		maxH := min(availH, max(260, availH*maxHeightFrac))
		SheetMotion(panel).FillWidth().MaxHeight(maxH).
			Padding(0, rightInset+16, max(16, bottomInset+8), leftInset+16).Gap(10).
			Radius(RadiusL+6, RadiusL+6, 0, 0).
			Shadow(0, -8, 28, 0, p.Shadow)
		if p.LiquidGlass {
			GlassSurface(panel, p.Surface, ui.Transparent)
		} else {
			panel.Background(p.Surface).BorderWidth(1, 1, 0, 1).BorderColor(p.LineSoft)
		}
		panel.Children(func() {
			closeSheet := func() { *open = false }
			ctx.OnWindowShortcut(0, ui.KeyBack, closeSheet)
			offset := ui.Local(panel, "drag-offset", func() float32 { return 0 })
			handle := ui.ButtonBase(ctx.Key("grabber")).Label(closeLabel).
				Width(104).Height(24).Margin(0, 0, -12, 0).AlignSelf(ui.Center).Center().Shrink(0).
				OnClick(closeSheet).Children(func() {
				ui.Box(ctx).Width(36).Height(4).Radius(RadiusFull).Background(p.Line)
			})
			animation := MotionSheet
			if _, delta, held := handle.Dragged(); held {
				*offset += delta
				animation = MotionChipOut
			} else if _, speed, ended := handle.DragEnded(); ended && (*offset >= 72 || *offset >= 18 && speed > 480) {
				closeSheet()
			} else {
				*offset = 0
			}
			panel.AnimatePose(ui.Pose{Y: max(0, *offset)}, animation)
			content()
		})
	})
}
