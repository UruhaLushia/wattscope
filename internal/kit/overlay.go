package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// MenuItem describes one selectable entry inside MenuButton, OptionRow, or context menus.
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

func surfaceChrome(ctx *ui.Context, el ui.Element, p Palette, radius float32) {
	if !p.LiquidGlass || IsSoftware(ctx) {
		el.Cache()
	}
	glassFloatingEl(ctx, el, p, radius, 0.1, false, p.Surface, p.Line.Mix(p.Text, 0.08)).
		Shadow(0, 10, 28, 0, p.Shadow)
}

func animatedChevron(ctx *ui.Context, open bool, color ui.Color, size float32) {
	angle := float32(0)
	if open {
		angle = 180
	}
	chev := ui.Box(ctx).Shrink(0).Center().TransformOrigin(0.5, 0.5)
	progress := chev.AnimateTo("chev-rot", angle, motionPopup)
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

type menuWidthCacheKey struct{}

type menuWidthCache struct {
	valid       bool
	windowW     float32
	zoom        float32
	textScale   float32
	touch       bool
	optionStyle bool
	hasIcon     bool
	hasCheck    bool
	labels      []string
	width       float32
}

func cachedAdaptiveMenuWidth(ctx *ui.Context, host, fallback ui.Element, items []MenuItem, touch, optionStyle bool) float32 {
	owner := host
	if !owner.Valid() {
		owner = fallback
	}
	cache := ui.Local(owner, menuWidthCacheKey{}, func() menuWidthCache { return menuWidthCache{} })
	if cache == nil {
		return adaptiveMenuWidth(ctx, items, touch, optionStyle)
	}
	windowW, _ := ctx.Size()
	zoom := ctx.Zoom()
	textScale := ctx.Preferences().TextScale
	hasIcon := false
	hasCheck := false
	same := cache.valid &&
		cache.windowW == windowW &&
		cache.zoom == zoom &&
		cache.textScale == textScale &&
		cache.touch == touch &&
		cache.optionStyle == optionStyle &&
		len(cache.labels) == len(items)
	for i, item := range items {
		if item.Icon != nil {
			hasIcon = true
		}
		if item.Active || item.Check || item.Radio {
			hasCheck = true
		}
		if same && cache.labels[i] != item.Label {
			same = false
		}
	}
	if same && cache.hasIcon == hasIcon && cache.hasCheck == hasCheck {
		return cache.width
	}
	width := adaptiveMenuWidth(ctx, items, touch, optionStyle)
	if cap(cache.labels) < len(items) {
		cache.labels = make([]string, len(items))
	} else {
		cache.labels = cache.labels[:len(items)]
	}
	for i, item := range items {
		cache.labels[i] = item.Label
	}
	cache.valid = true
	cache.windowW = windowW
	cache.zoom = zoom
	cache.textScale = textScale
	cache.touch = touch
	cache.optionStyle = optionStyle
	cache.hasIcon = hasIcon
	cache.hasCheck = hasCheck
	cache.width = width
	return width
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

func menuPopup(ctx *ui.Context, p Palette, menu ui.MenuParts, anchor ui.Element, touch bool, items []MenuItem) {
	menu.Popup(func(panel ui.Element) {
		panelW := cachedAdaptiveMenuWidth(ctx, anchor, panel, items, touch, false)
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0).
			Scrollbars(false).Width(panelW).Padding(6).Gap(2)
		popupMotion(ctx, panel)
		surfaceChrome(ctx, panel, p, RadiusXL)

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
				recordClickOrigin(row)
				entry.OnSelect()
			}
		}
	})
}

func optionMenuPopup(ctx *ui.Context, p Palette, trigger ui.Element, open *bool, topInset float32, items []MenuItem) {
	ui.PopoverBase(ctx, trigger, open, func(panel ui.Element) {
		panelW := cachedAdaptiveMenuWidth(ctx, trigger, panel, items, false, true)
		panel.AttachTo(trigger, ui.AnchorTopEnd, ui.AnchorTopEnd).Margin(-topInset-3, -formGroupInset-3, 0, 0).
			Scrollbars(false).Width(panelW).Padding(5, 6).Gap(1)
		surfaceChrome(ctx, panel, p, RadiusXL)
		optionMenuMotion(ctx, panel)

		itemMinH, fontSize, padY, padX := menuItemMetrics(false, true)
		for i, item := range items {
			entry := item
			row := ui.ButtonBase(ctx.Key(i)).Role(ui.RoleMenuItemRadio).Selected(entry.Active)
			styleMenuRow(ctx, p, row, entry, fontSize, itemMinH, padY, padX, row.Hovered() || row.FocusVisible(), true)
			row.OnClick(func() {
				if entry.OnSelect != nil {
					recordClickOrigin(row)
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

// MenuButtonOptions configures MenuButton trigger appearance, popup state, and items.
type MenuButtonOptions struct {
	Open     *bool
	Variant  ButtonVariant
	Icon     *ui.SVG
	Label    string
	Touch    bool
	IconOnly bool
	Size     float32
	Items    []MenuItem
}

// MenuButton renders a trigger button attached to an anchored dropdown MenuItem popup.
func MenuButton(ctx *ui.Context, p Palette, opts MenuButtonOptions) ui.Element {
	open := opts.Open
	variant := opts.Variant
	icon := opts.Icon
	label := opts.Label
	touch := opts.Touch
	iconOnly := opts.IconOnly
	size := opts.Size
	items := opts.Items
	if open == nil {
		var scratch bool
		open = &scratch
	}
	if iconOnly && size <= 0 {
		size = ControlHeight(touch)
	}

	h := size
	if !iconOnly {
		h = ControlHeight(touch)
	}
	targetR := RadiusM
	if p.LiquidGlass {
		targetR = h / 2
	}
	menu := ui.MenuBase(ctx, open)
	trigger := menu.Trigger.Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Label(label)
	btnRadius := morphRadius(ctx, trigger, glassRadiusKey{}, targetR)
	trigger.Radius(btnRadius)
	if iconOnly {
		trigger.Size(size, size).Tooltip(label)
	} else {
		trigger.Height(h).Padding(0, 9, 0, 11).Gap(4)
		if label == "" {
			trigger.Padding(0, 9)
		}
	}

	bg, ink, border, glassTint, borderW, weight, visibleAtRest := resolveButtonStyle(trigger, p, variant, menu.Open())
	chevColor := p.Muted
	if menu.Open() && variant != VariantPrimary {
		chevColor = ink
	}
	menuOpen := menu.Open()
	ControlMotion(trigger).Children(func() {
		GlassPillControl(ctx.Key("bg"), trigger, p, GlassPillOptions{
			Radius:             btnRadius,
			Tint:               glassTint,
			VisibleAtRest:      visibleAtRest,
			FallbackBackground: bg,
			BorderWidth:        borderW,
			FallbackBorder:     border,
			PressScale:         0.96,
		})
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
			animatedChevron(ctx, menuOpen, chevColor, 14)
		}
	})
	menuPopup(ctx, p, menu, trigger, touch, items)
	return trigger
}

// OptionRowOptions configures OptionRow label, current value chip, and popup choices.
type OptionRowOptions struct {
	Open  *bool
	Icon  *ui.SVG
	Label string
	Value string
	Touch bool
	Items []MenuItem
}

// OptionRow renders a settings row with a clickable value chip that opens an option menu popup.
func OptionRow(ctx *ui.Context, p Palette, opts OptionRowOptions) {
	open := opts.Open
	icon := opts.Icon
	label := opts.Label
	value := opts.Value
	touch := opts.Touch
	items := opts.Items

	if open == nil {
		var scratch bool
		open = &scratch
	}
	minH, padY, labelSize, valueSize := float32(48), float32(8), float32(13), float32(13)
	if touch {
		minH, padY, labelSize, valueSize = 52, 10, 14, 14
	}
	row := ui.Row(ctx).FillWidth().MinHeight(minH).Padding(padY, 0).Gap(12).
		AlignItems(ui.Center).Justify(ui.SpaceBetween).Transition(localeSizeTransition)

	var trigger ui.Element
	row.Children(func() {
		ui.Row(ctx).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(10).Children(func() {
			if icon != nil {
				ui.Icon(ctx, icon).FontSize(16).TextColor(p.Muted).Shrink(0)
			}
			AnimatedLabel(ctx, label).FontSize(labelSize).FontWeight(550).SingleLine()
		})

		targetAlpha, chipAnim := float32(1), motionChipIn
		if *open {
			targetAlpha, chipAnim = 0, motionChipOut
		}
		chipWrap := ui.Box(ctx.Key("slot")).Shrink(0).Transition(localeSizeTransition)
		chipOpacity := chipWrap.AnimateTo("koc-a", targetAlpha, chipAnim)
		chipWrap.Opacity(min(chipOpacity, targetAlpha)).Children(func() {
			trigger = ui.ButtonBase(ctx.Key("chip")).Shrink(0).Padding(6, 10).Gap(5).
				AlignItems(ui.Center).Label(label + " · " + value).Expanded(*open)
			targetR := RadiusM + 2
			if p.LiquidGlass {
				targetR = 16
			}
			chipRadius := morphRadius(ctx, trigger, glassRadiusKey{}, targetR)
			trigger.Radius(chipRadius)

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
				GlassPillControl(ctx.Key("bg"), trigger, p, GlassPillOptions{
					Radius:             chipRadius,
					VisibleAtRest:      p.LiquidGlass,
					FallbackBackground: chipBg,
					FallbackBorder:     ui.Transparent,
					PressScale:         0.96,
				})
				AnimatedLabel(ctx, value).FontSize(valueSize).FontWeight(550).TextColor(p.Muted).SingleLine()
				ui.Icon(ctx, IconChevronsUpDown).FontSize(13).TextColor(p.Muted).Shrink(0)
			})
		})
	})
	optionMenuPopup(ctx, p, trigger, open, padY, items)
}

// Popover displays an anchored floating popup surface controlled by open.
func Popover(ctx *ui.Context, p Palette, open *bool, anchor ui.Element, build func(panel ui.Element)) {
	ui.PopoverBase(ctx, anchor, open, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(6, 0, 0, 0)
		popupMotion(ctx, panel)
		surfaceChrome(ctx, panel, p, RadiusL)
		build(panel)
	})
}

// Dialog displays a centered modal dialog bounded by maxWidth and maxHeight when *open is true.
func Dialog(ctx *ui.Context, p Palette, open *bool, maxWidth, maxHeight float32, content func()) {
	width, height := ctx.Size()
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	ui.DialogBase(ctx, open, func(back, panel ui.Element) {
		backdropMotion(ctx, back, p)
		panelWidth := min(max(240, width-leftInset-rightInset-32), maxWidth)
		panelHeight := min(max(200, height-topInset-bottomInset-40), maxHeight)
		pad := float32(20)
		if width < 480 {
			pad = 16
		}
		dialogRadius := glassRadius(p, RadiusL, RadiusXL)
		if !p.LiquidGlass || IsSoftware(ctx) {
			panel.Cache()
		}
		glassFloatingEl(ctx, modalMotion(ctx, panel).Width(panelWidth).MaxHeight(panelHeight).Padding(pad).Gap(14),
			p, dialogRadius, 0.15, false, p.Surface, p.Line).
			Shadow(0, 14, 36, 0, p.Shadow).
			Children(content)
	})
}

// BottomSheet displays a swipe-dismissible bottom modal sheet up to maxHeightFrac of available height.
func BottomSheet(ctx *ui.Context, p Palette, open *bool, maxHeightFrac float32, closeLabel string, content func()) {
	_, height := ctx.Size()
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	ui.DialogBase(ctx, open, func(back, panel ui.Element) {
		backdropMotion(ctx, back, p).AlignItems(ui.Stretch).Justify(ui.End)
		availH := max(160, height-topInset-24)
		maxH := min(availH, max(260, availH*maxHeightFrac))
		if !p.LiquidGlass || IsSoftware(ctx) {
			panel.Cache()
		}
		sheetMotion(ctx, panel).FillWidth().MaxHeight(maxH).
			Padding(0, rightInset+16, max(16, bottomInset+8), leftInset+16).Gap(10).
			Radius(RadiusL+6, RadiusL+6, 0, 0).
			Shadow(0, -8, 28, 0, p.Shadow)
		if p.LiquidGlass {
			glassSurface(panel, p.Surface, ui.Transparent)
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
			animation := motionSheet
			if _, delta, held := handle.Dragged(); held {
				*offset += delta
				animation = motionChipOut
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
