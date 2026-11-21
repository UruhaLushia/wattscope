package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

type BottomNavStyle uint8

const (
	BottomNavCapsule BottomNavStyle = iota
	BottomNavStandard
)

const (
	BottomNavHeight         float32 = 56
	BottomNavStandardHeight float32 = 58
	BottomNavDockMargin     float32 = 16
	DesktopTopNavHeight     float32 = 32
	desktopTopNavControl    float32 = 26
	DesktopSidebarWidth     float32 = 196
	DesktopCompactRailWidth float32 = 60
	DesktopSidebarMargin    float32 = 12
)

func BottomNavHeightFor(style BottomNavStyle) float32 {
	if style == BottomNavStandard {
		return BottomNavStandardHeight
	}
	return BottomNavHeight
}

// BottomNavReserve returns the bottom clearance needed above a floating mobile
// navigation bar (or 0 when hidden) relative to the physical viewport bottom.
func BottomNavReserve(bottomInset float32, visible bool) float32 {
	return BottomNavReserveFor(bottomInset, visible, BottomNavCapsule)
}

func BottomNavReserveFor(bottomInset float32, visible bool, style BottomNavStyle) float32 {
	if !visible {
		return 0
	}
	return bottomInset + bottomNavDockGap(bottomInset) + BottomNavHeightFor(style)
}

func bottomNavDockGap(bottomInset float32) float32 {
	return max(10, BottomNavDockMargin-bottomInset)
}

type navPanelMatchKey struct{}
type navToggleMatchKey struct{}
type navTabMatchKey struct {
	index int
}

func navShadowColor(p Palette, strong bool) ui.Color {
	if p.Dark {
		if strong {
			return ui.RGBA(0, 0, 0, 0.36)
		}
		return ui.RGBA(0, 0, 0, 0.28)
	}
	if strong {
		return ui.RGBA(15, 23, 42, 0.11)
	}
	return ui.RGBA(15, 23, 42, 0.08)
}

func navBarChrome(bar ui.Element, p Palette, radius, clarity float32, fallbackBg, fallbackBorder ui.Color, strongShadow bool) ui.Element {
	if p.LiquidGlass {
		bar.Shadow(0, 4, 16, 0, navShadowColor(p, strongShadow))
		return glassFloatingUnclipped(bar, p, radius, clarity, ui.Color{}, fallbackBg, fallbackBorder)
	}
	if strongShadow || !p.Dark {
		bar.Shadow(0, 4, 16, 0, navShadowColor(p, strongShadow))
	}
	return GlassFloating(bar, p, radius, clarity, ui.Color{}, false, fallbackBg, fallbackBorder)
}

func navInactiveOverlayColors(p Palette, standard bool) (hoverBg, pressBg ui.Color) {
	if !standard {
		return p.Hover, p.Press
	}
	if p.Dark {
		return ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.12)
	}
	return ui.RGBA(15, 23, 42, 0.045), ui.RGBA(15, 23, 42, 0.085)
}

// DesktopNav renders desktop navigation as either a floating header capsule or
// an Apple-style floating inset sidebar card, keeping element identity alive
// across transitions so every destination flies cleanly between layouts.
func DesktopNav(ctx *ui.Context, p Palette, label string, selected *int, items []NavTab, sidebar, iconsOnly bool, tabPad, tabGap float32, onToggle func(), toggleLabel, toggleTooltip string) {
	if selected == nil {
		var scratch int
		selected = &scratch
	}
	navKey, selKey := "top-pages", "top-nav-selection"
	if sidebar {
		navKey, selKey = "side-pages", "side-nav-selection"
	}

	tabs := ui.TabsBase(ctx.Key(navKey), selected, len(items))
	bar := tabs.List.Label(label).Transition(navMatchTransition(navPanelMatchKey{}, true))

	if sidebar {
		railW := DesktopSidebarWidth
		if iconsOnly {
			railW = DesktopCompactRailWidth
		}
		sideRadius := RadiusL + 2
		if p.LiquidGlass {
			sideRadius = RadiusXL
		}
		bar.Column().Width(railW).Shrink(0).
			Margin(DesktopSidebarMargin, 0, DesktopSidebarMargin, DesktopSidebarMargin).
			Padding(8).Gap(3)
		navBarChrome(bar, p, sideRadius, 0.22, p.Surface, p.LineSoft, true)
		if iconsOnly {
			bar.AlignItems(ui.Center)
		} else {
			bar.AlignItems(ui.Start)
		}
	} else {
		topRadius := RadiusM
		if p.LiquidGlass {
			// Interpolate from the visible corner radius during layout changes.
			topRadius = DesktopTopNavHeight / 2
		}
		bar.Row().Height(DesktopTopNavHeight).Shrink(0).AlignItems(ui.Center).
			Padding(3).Gap(2)
		navBarChrome(bar, p, topRadius, 0, p.Track, p.LineSoft, false)
		if !iconsOnly {
			bar.Margin(0, 0, 0, 6)
		}
	}

	bar.Children(func() {
		if onToggle != nil {
			toggleIcon, toggleSize := IconSidebar, desktopTopNavControl
			toggleRadius := RadiusS
			if p.LiquidGlass {
				toggleRadius = toggleSize / 2
			}
			if sidebar {
				toggleIcon, toggleSize = IconTopBar, 28
				if p.LiquidGlass {
					toggleRadius = RadiusM + 2
				}
			}
			toggle := ui.ButtonBase(ctx.Key("nav-toggle")).Size(toggleSize, toggleSize).
				Shrink(0).Radius(toggleRadius).Center().
				Label(toggleLabel).Tooltip(toggleTooltip).
				Transition(navMatchTransition(navToggleMatchKey{}, false)).
				OnClick(onToggle)
			if sidebar {
				if iconsOnly {
					toggle.Margin(0, 0, 4, 0)
				} else {
					toggle.Margin(0, 0, 4, 5)
				}
			}
			bg, fg := ui.Transparent, p.Muted
			switch {
			case toggle.Pressed():
				bg, fg = p.Press, p.Text
			case toggle.Hovered() || toggle.FocusVisible():
				bg, fg = p.Hover, p.Text
			}
			toggle.Children(func() {
				GlassPillControl(ctx.Key("ntog-state"), toggle, p, toggleRadius, ui.Color{}, false, bg, 0, ui.Transparent, 0.95)
				pop := float32(0)
				if toggle.Pressed() {
					pop = 1
				}
				GlassIconPop(ui.Icon(ctx, toggleIcon).FontSize(15).
					TextColor(toggle.AnimateColor("ntog-fg", fg, MotionFast)), p, pop)
			})
		}

		tabRadius := RadiusS
		switch {
		case sidebar && p.LiquidGlass:
			tabRadius = RadiusM + 2
		case sidebar:
			tabRadius = RadiusM
		case p.LiquidGlass:
			tabRadius = desktopTopNavControl / 2
		}
		for index, item := range items {
			entry := item
			hasChildren := sidebar && !iconsOnly && len(entry.Children) > 0
			open := hasChildren && (index == *selected || entry.Selected >= 0)
			active := index == *selected && !hasChildren
			buildTab := func() {
				tab := tabs.Tab(index).Label(entry.Label).
					Radius(tabRadius).AlignItems(ui.Center).Gap(tabGap).
					Background(ui.Transparent).
					Transition(navMatchTransition(navTabMatchKey{index: index}, false)).
					OnClick(func() {
						if entry.OnSelect != nil {
							entry.OnSelect()
						}
					})
				if hasChildren {
					tab.Expanded(open)
				}

				if sidebar {
					if iconsOnly {
						tab.Size(36, 36).Padding(0).Justify(ui.Center).Tooltip(entry.Label)
					} else {
						tab.FillWidth().Height(36).Padding(0, 10)
					}
				} else {
					if iconsOnly {
						tab.Size(desktopTopNavControl, desktopTopNavControl).Padding(0).Justify(ui.Center).Tooltip(entry.Label)
					} else {
						tab.Height(desktopTopNavControl).Padding(0, tabPad).Shrink(0)
					}
				}

				var bg ui.Color
				fg, weight := p.Muted, 550
				switch {
				case active && tab.Pressed():
					tab.ZIndex(0)
					fg = p.TrackOnInk.Alpha(0.90)
				case active:
					tab.ZIndex(0)
					fg = p.TrackOnInk
				case open && tab.Pressed():
					tab.ZIndex(1)
					bg, fg, weight = p.Press, p.AccentText, 600
				case open && (tab.Hovered() || tab.FocusVisible()):
					tab.ZIndex(1)
					bg, fg, weight = p.Hover, p.AccentText, 600
				case open:
					tab.ZIndex(1)
					fg, weight = p.AccentText, 600
				case tab.Pressed():
					tab.ZIndex(1)
					bg, fg = p.Press, p.Text
				case tab.Hovered() || tab.FocusVisible():
					tab.ZIndex(1)
					bg, fg = p.Hover, p.Text
				default:
					tab.ZIndex(1)
				}

				tab.Children(func() {
					if active {
						GlassSelectionPill(ctx.Key(selKey), tab, p, tabRadius, p.TrackOn, selKey, !sidebar)
					} else {
						GlassPillControl(ctx.Key("tab-state"), tab, p, tabRadius, ui.Color{}, false, bg, 0, ui.Transparent, 0.95)
					}
					if entry.Icon != nil {
						iconSize := float32(15)
						if sidebar {
							iconSize = 17
						}
						pop := float32(0)
						if tab.Pressed() {
							pop = 1
						}
						GlassIconPop(ui.Icon(ctx, entry.Icon).FontSize(iconSize).
							TextColor(tab.AnimateColor("tab-fg", fg, MotionNav)), p, pop)
					}
					if iconsOnly {
						return
					}
					AnimatedLabel(ctx, entry.Label).FontSize(13).FontWeight(weight).SingleLine().Shrink(0).
						TextColor(tab.AnimateColor("tab-text", fg, MotionNav))
				})
			}

			if !hasChildren {
				buildTab()
				continue
			}

			ui.Column(ctx.Key("sec-" + entry.Label)).FillWidth().Gap(0).Children(func() {
				buildTab()

				if !open {
					return
				}
				const (
					subRowH   float32 = 30
					subRowGap float32 = 3
				)
				subMotion := ui.ElementTransition{
					Position:  true,
					Animation: &MotionRail,
					Enter:     &ui.Motion{Collapse: true},
					Exit:      &ui.Motion{Collapse: true},
				}
				ui.Column(ctx.Key("sub-wrap")).FillWidth().
					Padding(2, 0, 2, 28).Gap(subRowGap).Transition(subMotion).
					Children(func() {
						ui.Box(ctx.Key("sub-track")).Absolute().Left(18).Top(4).Bottom(4).Width(1).
							Background(p.LineSoft).PassThrough()
						if entry.Selected >= 0 && entry.Selected < len(entry.Children) {
							indY := float32(2) + float32(entry.Selected)*(subRowH+subRowGap) + (subRowH-16)/2
							ui.Box(ctx.Key("sub-indicator")).Absolute().Left(17.5).Top(0).Width(2).Height(16).
								Radius(RadiusFull).Background(p.Accent).PassThrough().
								AnimatePose(ui.Pose{Y: indY}, LayoutAnimation(MotionRail))
						}
						for subIdx, sub := range entry.Children {
							child := sub
							subActive := entry.Selected == subIdx
							subRow := ui.ButtonBase(ctx.Key(subIdx)).Role(ui.RoleTab).AccessSelected(subActive).
								FillWidth().Height(subRowH).Padding(0, 10).Radius(RadiusS).
								AlignItems(ui.Center).Justify(ui.Start).Label(child.Label).
								Background(ui.Transparent).
								OnClick(func() {
									if child.OnSelect != nil {
										child.OnSelect()
									}
								})
							subBg, subFg, subWeight := ui.Transparent, p.Muted, 500
							switch {
							case subActive && subRow.Pressed():
								subBg, subFg, subWeight = p.Press, p.AccentText, 650
							case subActive:
								subFg, subWeight = p.AccentText, 650
							case subRow.Pressed():
								subBg, subFg = p.Press, p.Text
							case subRow.Hovered() || subRow.FocusVisible():
								subBg, subFg = p.Hover, p.Text
							}
							subAnim := MotionRail
							subAnim.Delay = time.Duration(subIdx) * 16 * time.Millisecond
							subRow.Transition(ui.ElementTransition{
								Position:  true,
								Animation: &subAnim,
								Enter:     &ui.Motion{Y: -4},
							}).Children(func() {
								GlassPillControl(ctx.Key("bg"), subRow, p, RadiusS, ui.Color{}, false, subBg, 0, ui.Transparent, 0.96)
								AnimatedLabel(ctx, child.Label).FontSize(12.5).FontWeight(subWeight).SingleLine().
									TextColor(subRow.AnimateColor("sub-fg", subFg, MotionFast))
							})
						}
					})
			})
		}
	})
}

type NavTab struct {
	Label    string
	Icon     *ui.SVG
	Children []NavTab
	Selected int
	OnSelect func()
}

func SectionTabs(ctx *ui.Context, p Palette, label string, selected int, items []NavTab) {
	tabRadius := RadiusM
	if p.LiquidGlass {
		tabRadius = RadiusFull
	}
	ui.Row(ctx).Role(ui.RoleTabList).FocusGroup(ui.Horizontal).Label(label).Gap(4).AlignItems(ui.Center).Children(func() {
		for index, item := range items {
			active := index == selected
			tab := ui.ButtonBase(ctx.Key(index)).Role(ui.RoleTab).AccessSelected(active).Label(item.Label).
				Height(32).Padding(0, 12).Radius(tabRadius).AlignItems(ui.Center).
				Background(ui.Transparent).OnClick(item.OnSelect)
			bg, fg, weight := ui.Transparent, p.Muted, 500
			var glassTint ui.Color
			visibleAtRest := false
			switch {
			case active:
				bg, fg, weight = p.AccentSoft, p.AccentText, 650
				glassTint, visibleAtRest = GlassAccentTint(p, 0.24, 0.28), true
			case tab.Pressed():
				bg, fg = p.Press, p.Text
			case tab.Hovered() || tab.FocusVisible():
				bg, fg = p.Hover, p.Text
			}
			ControlMotion(tab).Children(func() {
				GlassPillControl(ctx.Key("bg"), tab, p, tabRadius, glassTint, visibleAtRest, bg, 0, ui.Transparent, 0.96)
				AnimatedLabel(ctx, item.Label).FontSize(13).FontWeight(weight).TextColor(fg).SingleLine()
			})
		}
	})
}

const (
	bottomNavPad             float32 = 7
	bottomNavGap             float32 = 2
	bottomNavStdPad          float32 = 4
	bottomNavStdGap          float32 = 2
	bottomNavStdSlot         float32 = 63
	bottomNavInactiveW       float32 = 44
	bottomNavIconSize        float32 = 20
	bottomNavLabelGap        float32 = 6
	bottomNavLabelPadX       float32 = 13
	bottomNavDragThresh      float32 = 12
	bottomNavGlassDragThresh float32 = 6
)

type mnbDragState struct {
	pressing      bool
	active        bool
	upPending     bool
	canceled      bool
	suppressClick bool
	originX       float32
	lastX         float32
	lastY         float32
	dragIndex     int
	prevLeft      float32
	lastTime      time.Time
	hasPrev       bool
	lastStandard  bool
	lastCount     int
	labels        []string
	widths        []float32
	slots         []bottomNavSlot
}

type bottomNavSlot struct {
	left, width float32
}

func (s *mnbDragState) ensureWidths(ctx *ui.Context, items []NavTab) {
	if len(s.widths) == len(items) && len(s.labels) == len(items) {
		same := true
		for i, it := range items {
			if s.labels[i] != it.Label {
				same = false
				break
			}
		}
		if same {
			return
		}
	}
	s.labels = make([]string, len(items))
	s.widths = make([]float32, len(items))
	for i, it := range items {
		s.labels[i] = it.Label
		tw, _ := ctx.MeasureText(1<<20, ui.Span{Text: it.Label, Size: 12.5, Weight: 650})
		s.widths[i] = max(bottomNavInactiveW, bottomNavLabelPadX*2+bottomNavIconSize+bottomNavLabelGap+tw)
	}
}

func layoutBottomNav(slots []bottomNavSlot, pad, gap, width float32, active int, activeWidth float32) float32 {
	x := pad
	for i := range slots {
		w := width
		if i == active {
			w = activeWidth
		}
		slots[i] = bottomNavSlot{left: x, width: w}
		x += w + gap
	}
	return x - gap + pad
}

func layoutStandardBottomNav(ctx *ui.Context, slots []bottomNavSlot, leftInset, rightInset float32) float32 {
	availW, _ := ctx.Size()
	if availW <= 0 {
		availW = 390
	}
	count := len(slots)
	fixedW := bottomNavStdGap*float32(count-1) + bottomNavStdPad*2
	maxBarW := min(344, float32(count)*bottomNavStdSlot+fixedW)
	sidePad := max(22, max(leftInset, rightInset)+14)
	barW := min(maxBarW, max(220, availW-sidePad*2))
	width := (barW - fixedW) / float32(count)
	layoutBottomNav(slots, bottomNavStdPad, bottomNavStdGap, width, -1, 0)
	return barW
}

func mnbSlotIndexAt(x, gap float32, slots []bottomNavSlot) int {
	for i, slot := range slots {
		if x < slot.left+slot.width+gap/2 {
			return i
		}
	}
	return len(slots) - 1
}

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mnbRubberBand(overscroll, limit float32) float32 {
	if overscroll == 0 || limit <= 0 {
		return 0
	}
	return limit * float32(math.Tanh(float64(overscroll)/float64(limit*2.5)))
}

// BottomNavigationBar renders a centered floating navigation bar above the bottom
// safe area in either compact Capsule style (active tab unfolds label) or Standard
// style (equal icon-above-label tabs with a sliding glass highlight pill). Holding
// and dragging horizontally scrubs the active indicator pill across destinations.
func BottomNavigationBar(ctx *ui.Context, p Palette, key, label string, selected *int, items []NavTab, style BottomNavStyle, bottomInset, leftInset, rightInset float32) {
	if len(items) == 0 {
		return
	}
	if selected == nil {
		var scratch int
		selected = &scratch
	}
	standard := style == BottomNavStandard
	standardGlass := standard && p.LiquidGlass
	capsuleGlass := !standard && p.LiquidGlass
	barHeight := BottomNavHeightFor(style)
	barPad, barGap := bottomNavPad, bottomNavGap
	if standard {
		barPad, barGap = bottomNavStdPad, bottomNavStdGap
	} else if p.LiquidGlass {
		barPad = 5
	}
	tabHeight := barHeight - barPad*2

	dock := ui.Row(ctx.Key(key+"-dock")).Absolute().Left(0).Right(0).Bottom(0).ZIndex(10).
		Justify(ui.Center).AlignItems(ui.End).PassThrough().
		Padding(0, 12, bottomNavDockGap(bottomInset), 12)

	barSolid := ui.Hex("#f7fafc").Mix(p.Accent, 0.07)
	barBg := barSolid.Alpha(0.90)
	barBorder := ui.RGBA(17, 24, 39, 0.08)
	accentPill := p.Accent.Alpha(0.92).Over(barSolid)
	stdPillBg := ui.RGBA(15, 23, 42, 0.065)
	if standard {
		barBg = ui.Hex("#ffffff").Alpha(0.96)
	}
	if p.Dark {
		barSolid = p.SurfaceHi.Mix(p.Accent, 0.09)
		barBg = barSolid.Alpha(0.92)
		if standard {
			barBg = p.SurfaceHi.Alpha(0.95)
		}
		barBorder = ui.RGBA(255, 255, 255, 0.10)
		accentPill = p.Accent.Alpha(0.76).Over(barSolid)
		stdPillBg = ui.RGBA(255, 255, 255, 0.14)
	}
	pillInk := AccentInk(accentPill)

	selKey := key + "-pill"

	dock.Children(func() {
		tabs := ui.TabsBase(ctx.Key(key), selected, len(items))
		bar := tabs.List.Label(label).Row().Shrink(0).Height(barHeight).AlignItems(ui.Center).
			Padding(barPad).Gap(barGap).
			Draggable(nil, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &ui.Rect{}, Rigid: true}).
			Transition(ui.ElementTransition{
				Position: true, Size: true, Animation: &MotionNav,
			})
		if !p.LiquidGlass {
			navBarChrome(bar, p, RadiusFull, 0, barBg, barBorder, true)
		} else if !standard && !p.Dark {
			bar.Shadow(0, 2, 12, 0, ui.RGBA(12, 18, 30, 0.065))
		}

		st := ui.Local(bar, "mnb-drag", func() mnbDragState { return mnbDragState{} })
		if len(st.slots) != len(items) {
			st.slots = make([]bottomNavSlot, len(items))
		}
		slots := st.slots
		if !standard {
			st.ensureWidths(ctx, items)
		}

		bar.ObserveInput(func(ev ui.InputEvent) {
			if ev.Button != 0 {
				return
			}
			switch ev.Kind {
			case ui.InputPointerDown:
				st.pressing = true
				st.active = false
				st.upPending = false
				st.canceled = false
				st.suppressClick = false
				st.originX = ev.X
				st.lastX = ev.X
				st.lastY = ev.Y
				st.dragIndex = clamp32Index(*selected, len(items))
			case ui.InputPointerUp:
				if st.pressing {
					st.upPending = true
					if ev.Canceled {
						st.canceled = true
					}
				}
			}
		})

		barPressed := bar.Pressed()
		if b, ok := bar.PressedButton(); ok && b == 0 {
			barPressed = true
		}
		_, _, barDragHeld := bar.Dragged()
		_, _, barDragEnded := bar.DragEnded()
		barCanceled := bar.PressCanceled()
		px, py, _ := bar.PointerPosition()

		bar.Children(func() {
			var inlineTabs [4]ui.Element
			tabEls := inlineTabs[:]
			if len(items) > len(tabEls) {
				tabEls = make([]ui.Element, len(items))
			}
			tabPressed := false
			tabCanceled := false
			tabDragEnded := false
			for index, item := range items {
				tab := tabs.Tab(index).Label(item.Label).Radius(RadiusFull).
					AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
					Background(ui.Transparent)
				if !p.LiquidGlass {
					tab.Clip()
				}
				if standard {
					tab.Column().Gap(2)
				} else {
					tab.Row()
				}
				if tab.Pressed() {
					tabPressed = true
				} else if b, ok := tab.PressedButton(); ok && b == 0 {
					tabPressed = true
				}
				if _, _, ended := tab.DragEnded(); ended {
					tabDragEnded = true
				}
				if tab.PressCanceled() {
					tabCanceled = true
				}
				tabEls[index] = tab
			}

			anyPressed := barPressed || barDragHeld || tabPressed
			anyCanceled := st.canceled || barCanceled || tabCanceled
			dragEnded := barDragEnded || tabDragEnded
			// A tab can emit a synthetic release when the dock takes over the drag.
			if anyPressed && !anyCanceled && !dragEnded {
				st.upPending = false
			}
			released := st.upPending || dragEnded

			if !released && !anyPressed {
				st.suppressClick = false
			}

			curIdx := clamp32Index(*selected, len(items))
			if st.active && !p.LiquidGlass {
				curIdx = clamp32Index(st.dragIndex, len(items))
			}
			var barW float32
			if standard {
				barW = layoutStandardBottomNav(ctx, slots, leftInset, rightInset)
				bar.Width(barW)
			} else {
				barW = layoutBottomNav(slots, barPad, barGap, bottomNavInactiveW, curIdx, st.widths[curIdx])
			}

			dragThresh := bottomNavDragThresh
			if p.LiquidGlass {
				dragThresh = bottomNavGlassDragThresh
			}

			switch {
			case released:
				if st.pressing && st.active && !anyCanceled {
					target := clamp32Index(st.dragIndex, len(items))
					*selected = target
					if items[target].OnSelect != nil {
						items[target].OnSelect()
					}
					st.suppressClick = true
				}
				st.pressing = false
				st.active = false
				st.upPending = false
				st.canceled = false
			case anyCanceled || (st.pressing && !anyPressed):
				st.pressing = false
				st.active = false
				st.upPending = false
				st.canceled = false
			case anyPressed:
				if !st.pressing {
					st.pressing = true
					st.originX = px
					st.dragIndex = clamp32Index(*selected, len(items))
				}
				st.lastX = px
				st.lastY = py
				if !st.active && abs32(px-st.originX) >= dragThresh {
					st.active = true
				}
				if st.active {
					st.dragIndex = mnbSlotIndexAt(px, barGap, slots)
				}
			}

			displayIndex := clamp32Index(*selected, len(items))
			if st.active {
				displayIndex = clamp32Index(st.dragIndex, len(items))
			}
			if !standard && !p.LiquidGlass && displayIndex != curIdx {
				barW = layoutBottomNav(slots, barPad, barGap, bottomNavInactiveW, displayIndex, st.widths[displayIndex])
			}

			capsulePose, capsuleAnim := ui.Pose{}, MotionNav
			if !p.LiquidGlass && st.active && barW > 0 {
				var overscroll float32
				switch {
				case st.lastX < 0:
					overscroll = st.lastX
				case st.lastX > barW:
					overscroll = st.lastX - barW
				}
				nudge := clamp32((st.lastX-barW/2)*0.035, -3.5, 3.5) + mnbRubberBand(overscroll, 8)
				capsulePose.X = nudge
				capsuleAnim = MotionCapsuleDrag
			}
			bar.TransformOrigin(0.5, 0.5).AnimatePose(capsulePose, capsuleAnim)

			pillLeft, pillW := slots[displayIndex].left, slots[displayIndex].width
			pillBg := accentPill
			if standard || p.LiquidGlass {
				pillBg = stdPillBg
			}
			if st.active {
				innerMin := barPad
				innerMax := max(innerMin, barW-barPad-pillW)
				if standard {
					targetLeft := clamp32(st.lastX-pillW/2, innerMin, innerMax)
					if standardGlass {
						pillLeft = targetLeft
					} else {
						pillLeft += (targetLeft - pillLeft) * 0.82
					}
				} else {
					delta := clamp32((st.lastX-(pillLeft+pillW/2))*0.18, -5, 5)
					pillLeft = clamp32(pillLeft+delta, innerMin, innerMax)
				}
			}

			pillCheckLeft, pillCheckW := pillLeft, pillW
			var lens bottomNavLens
			if p.LiquidGlass {
				st.applyDockGlass(bar, p, barW, barHeight, anyPressed || st.active, standard)
				lens = st.animateLens(ctx, bar, ui.Rect{X: pillLeft, Y: barPad, W: pillW, H: tabHeight}, standard, len(items), tabEls[displayIndex].Pressed())
				pillCheckLeft, pillCheckW = lens.left, lens.width

				if !ctx.Preferences().ReduceTransparency {
					pillBg = scaleAlpha(pillBg, 1-lens.alpha)
				}
				pill := lens.box(ctx.Key(selKey), 0).
					Background(bar.AnimateColor("mnb-pill-bg", pillBg, MotionFast))
				if lens.alpha > 0.01 {
					bottomNavLensShadow(pill, p.Dark, lens.alpha, standard)
				}
			} else {
				st.hasPrev = false
				pillAnim := MotionCapsulePill
				pillPose, pillPoseAnim := ui.Pose{}, MotionNav
				if st.active {
					pillBg = pillBg.Mix(p.Text, 0.06)
					pillAnim = MotionCapsuleDrag
					align := clamp32((st.lastX/max(barW, 1))*2-1, -1, 1)
					edge := clamp32((abs32(align)-0.72)/0.28, 0, 1)
					pillPose.Scale = 1 + edge*0.025
					pillPoseAnim = MotionCapsuleDrag
				} else if tabEls[displayIndex].Pressed() {
					pillBg = pillBg.Mix(p.Text, 0.08)
					pillPose.Scale = 0.96
					pillPoseAnim = MotionFast
				}
				pill := ui.Box(ctx.Key(selKey)).Absolute().Left(pillLeft).Top(barPad).
					Width(pillW).Height(tabHeight).ZIndex(0).Radius(RadiusFull).
					TransformOrigin(0.5, 0.5).AnimatePose(pillPose, pillPoseAnim).
					Background(bar.AnimateColor("mnb-pill-bg", pillBg, MotionFast)).
					Transition(ui.ElementTransition{
						Position: true, Size: true, Animation: &pillAnim,
					}).PassThrough()
				if !standard && p.CardShadow.A > 0 && !tabEls[displayIndex].Pressed() {
					pill.Shadow(0, 1, 3, 0, p.CardShadow)
				}
			}

			hoverBg, pressBg := navInactiveOverlayColors(p, standard)
			idleFg, activeFg := p.Text.Alpha(0.84), pillInk
			if standard {
				idleFg = p.Text.Alpha(0.76)
			}
			if standard || p.LiquidGlass {
				activeFg = p.AccentText
			}

			lensMoving := st.active || lens.inFlight
			followLens := standardGlass && lens.optics && lensMoving
			lensMid := lens.bounds.X + lens.bounds.W*0.5
			lensHalf := max(lens.bounds.W*0.5, 1)

			for index, item := range items {
				entry := item
				active := index == displayIndex
				slot := slots[index]
				tab := tabEls[index].
					Size(slot.width, tabHeight).
					Transition(ui.ElementTransition{
						Position: true, Size: true, Animation: &MotionNav,
					}).
					OnClick(func() {
						if st.suppressClick {
							st.suppressClick = false
							return
						}
						if entry.OnSelect != nil {
							entry.OnSelect()
						}
					})

				cx := slot.left + slot.width/2
				coveredByPill := lensMoving && cx >= pillCheckLeft+6 && cx <= pillCheckLeft+pillCheckW-6
				pressed := tab.Pressed()
				tapPressed := !active && !st.active && pressed
				iconPop := float32(0)
				if standardGlass {
					tab.ZIndex(2)
				} else {
					if active || coveredByPill {
						tab.ZIndex(3)
						iconPop = lens.alpha
					} else if tapPressed && p.LiquidGlass {
						tab.ZIndex(3)
						iconPop = 1.0
					} else {
						tab.ZIndex(1)
					}
					if capsuleGlass && !StaticSnapshot {
						tabScale := float32(1)
						tabScaleAnim := MotionGlassRelease
						if iconPop > 0.005 {
							tabScale = 1 + 0.30*clamp32(iconPop, 0, 1.25)
							tabScaleAnim = MotionGlassPress
						}
						tab.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: tabScale}, tabScaleAnim)
					}
				}

				var stateBg ui.Color
				fg := idleFg
				colorAnim := MotionNav
				var lensDistance float32
				if standardGlass {
					switch {
					case followLens:
						lensDistance = abs32(cx - lensMid)
						u := clamp32(1-lensDistance/(lensHalf*0.80), 0, 1)
						cover := u * u * (3 - 2*u)
						fg = idleFg.Mix(activeFg, clamp32(cover*lens.alpha, 0, 1))
						colorAnim = MotionFast
					case active:
						fg = activeFg
					case tapPressed:
						stateBg, fg = pressBg, p.Text
					case !st.active && (tab.Hovered() || tab.FocusVisible()):
						stateBg, fg = hoverBg, p.Text
					}
				} else {
					switch {
					case active && !st.active && pressed:
						if p.LiquidGlass {
							fg = activeFg
						} else {
							fg = activeFg.Alpha(0.90)
						}
					case active || coveredByPill:
						fg = activeFg
						if lensMoving {
							colorAnim = MotionFast
						}
					case tapPressed:
						stateBg, fg = pressBg, p.Text
					case !st.active && (tab.Hovered() || tab.FocusVisible()):
						stateBg, fg = hoverBg, p.Text
					default:
						if st.active {
							colorAnim = MotionFast
						}
					}
				}

				tab.Children(func() {
					if !active && !st.active {
						if p.LiquidGlass {
							if standard {
								GlassPillControl(ctx.Key("mnb-state"), tab, p, RadiusFull, ui.Color{}, false, stateBg, 0, ui.Transparent, 0.985)
							} else if !tapPressed {
								GlassPillControl(ctx.Key("mnb-state"), tab, p, RadiusFull, ui.Color{}, false, stateBg, 0, ui.Transparent, 0.95)
							}
						} else {
							statePose, stateAnim := ui.Pose{}, MotionNav
							if pressed {
								statePose.Scale = 0.95
								stateAnim = MotionFast
							}
							ui.Box(ctx.Key("mnb-state")).Absolute().Fill().ZIndex(-1).Radius(RadiusFull).
								TransformOrigin(0.5, 0.5).AnimatePose(statePose, stateAnim).
								Background(tab.AnimateColor("mnb-bg", stateBg, MotionFast)).PassThrough()
						}
					}
					if entry.Icon != nil {
						GlassIconPop(ui.Icon(ctx, entry.Icon).FontSize(bottomNavIconSize).Shrink(0).
							TextColor(tab.AnimateColor("mnb-fg", fg, colorAnim)), p, iconPop*0.18)
					}
					if standard {
						weight := 500
						emphasized := active || (!standardGlass && coveredByPill)
						if followLens {
							emphasized = lensDistance < lensHalf*0.48
						}
						if emphasized {
							weight = 650
						}
						AnimatedLabel(ctx, entry.Label).FontSize(11).FontWeight(weight).
							SingleLine().Shrink(0).
							TextColor(tab.AnimateColor("mnb-text", fg, colorAnim))
					} else if active {
						ui.Row(ctx.Key("mnb-lbl")).Shrink(0).Padding(0, 0, 0, bottomNavLabelGap).
							Transition(ui.ElementTransition{
								Enter:     &ui.Motion{Opacity: 0},
								Animation: &MotionCapsuleIn,
							}).
							Children(func() {
								AnimatedLabel(ctx, entry.Label).FontSize(12.5).FontWeight(650).
									SingleLine().Shrink(0).
									TextColor(tab.AnimateColor("mnb-text", fg, colorAnim))
							})
					}
				})
				if capsuleGlass && tapPressed {
					interaction := trackGlassInteraction(tab)
					dx, dy := bottomNavLensExpand(slot.width, tabHeight, interaction.elastic, 0, false)
					tapBox := expandGlass(ui.Rect{X: slot.left, Y: barPad, W: slot.width, H: tabHeight}, dx, dy)
					tap := ui.Box(ctx.Key(mnbTapKey(index))).Absolute().Left(tapBox.X).Top(tapBox.Y).
						Width(tapBox.W).Height(tapBox.H).ZIndex(2).Radius(RadiusFull).PassThrough()
					bottomNavLensShadow(tap, p.Dark, interaction.glow, false)
					drawBottomNavGlass(tap, liquidLensSpec{
						Alpha: interaction.glow, Glow: interaction.glow, ZoomY: 0.86, Touch: interaction.touch, ClearLens: true,
					})
				}
			}

			if lens.optics {
				spec := liquidLensSpec{
					Alpha: lens.alpha, Glow: lens.glow, Touch: lens.touch, ClearLens: true,
				}
				z := 2
				if standard {
					const standardZoom = 1.32
					lensZoom := 1 + 0.32*lens.alpha
					barScaleY := 1 - 0.05*lens.alpha
					veil := ui.RGBA(255, 255, 255, 0.52*lens.alpha)
					if p.Dark {
						veil = ui.RGBA(34, 40, 54, 0.24*lens.alpha)
					}
					drawBottomNavGlass(lens.box(ctx.Key(selKey+"-warp"), 1), liquidLensSpec{
						Alpha: 1, Tint: veil, Zoom: 1 / lensZoom, ZoomY: barScaleY / lensZoom, WarpOnly: true,
					})
					spec.Zoom = standardZoom
					z = 3
				} else {
					spec.ZoomY = 0.86
				}
				drawBottomNavGlass(lens.box(ctx.Key(selKey+"-lens"), z), spec)
			}
		})
	})
}

type mnbTapKey int

func bottomNavLensShadow(el ui.Element, dark bool, amount float32, standard bool) {
	if standard {
		alpha := float32(0.068)
		if dark {
			alpha = 0.26
		}
		el.Shadow(0, 2.0*amount, 9.5*amount, 0, ui.RGBA(12, 18, 30, alpha*min(amount, 1)))
		return
	}
	alpha := float32(0.075)
	if dark {
		alpha = 0.24
	}
	el.Shadow(0, 1.5*amount, 8*amount, 0, ui.RGBA(12, 18, 30, alpha*min(amount, 1)))
}

func drawBottomNavGlass(el ui.Element, spec liquidLensSpec) {
	if StaticSnapshot || spec.Alpha <= 0.005 {
		return
	}
	el.Draw(func(p *ui.Painter, box ui.Rect) {
		paintLiquidGlass(p, box, box.H*0.5, spec)
	})
}

func bottomNavLensExpand(width, height, elastic, stretch float32, standard bool) (dx, dy float32) {
	if standard {
		dx = clamp32(width*0.26, 15.5, 18.5)*elastic + width*stretch*0.25
		dy = clamp32(height*0.24, 11.0, 13.0)*elastic - height*stretch*0.05
		return dx, dy
	}
	dx = clamp32(width*0.18, 12.0, 17.0)*elastic + width*stretch*0.38
	dy = clamp32(height*0.22, 10.0, 12.0)*elastic - height*stretch*0.06
	return dx, dy
}

func (s *mnbDragState) applyDockGlass(bar ui.Element, p Palette, width, height float32, pressed, standard bool) {
	var progress float32
	if !StaticSnapshot {
		var target float32
		if pressed {
			target = 1
		}
		sp := GlassPressAnimation(pressed).Spring
		if standard {
			if pressed {
				sp = ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.14}
			} else {
				sp = ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.16}
			}
		}
		progress = bar.Spring("mnb-bar-press", target, sp)
	}
	glow := clamp32(progress, 0, 1.25)
	touch := ui.Point{X: 0.5, Y: 0.5}
	if width > 0 && s.pressing {
		touch.X = clamp32(s.lastX/width, 0.06, 0.94)
		touch.Y = clamp32(s.lastY/max(height, 1), 0.1, 0.9)
	}
	bar.Radius(RadiusFull).Material(dockGlass{glassInteraction{
		elastic: clamp32(progress, -0.2, 1.25),
		glow:    glow,
		touch:   touch,
	}})
	if !p.Dark {
		if standard {
			g := clamp32(glow, 0, 1)
			bar.Shadow(0, 2+1.0*g, 11+2*g, 0, ui.RGBA(12, 18, 30, 0.052-0.014*g))
		} else if glow > 0.01 {
			bar.Shadow(0, 3+2*glow, 12+4*glow, 0, ui.RGBA(12, 18, 30, 0.06+0.04*glow))
		}
	}
}

type bottomNavLens struct {
	bounds      ui.Rect
	touch       ui.Point
	left, width float32
	alpha, glow float32
	inFlight    bool
	optics      bool
}

func (l bottomNavLens) box(ctx *ui.Context, z int) ui.Element {
	return ui.Box(ctx).Absolute().Left(l.bounds.X).Top(l.bounds.Y).
		Width(l.bounds.W).Height(l.bounds.H).ZIndex(z).Radius(RadiusFull).PassThrough()
}

func (s *mnbDragState) animateLens(ctx *ui.Context, bar ui.Element, target ui.Rect, standard bool, count int, tabPressed bool) bottomNavLens {
	now := ctx.Now()
	left, width := target.X, target.W
	var stretch, lift float32
	var inFlight bool
	if StaticSnapshot || ScaleReflow || !s.hasPrev || s.lastStandard != standard || s.lastCount != count {
		bar.Jump("mnb-px", left)
		bar.Jump("mnb-pw", width)
		bar.Jump("mnb-stretch", 0)
		bar.Jump("mnb-lift", 0)
		s.hasPrev, s.lastStandard, s.lastCount = true, standard, count
	} else {
		posSpring := MotionGlassScrub.Spring
		if standard {
			if s.active {
				posSpring = ui.SpringOptions{Duration: 40 * time.Millisecond, Bounce: 0}
			} else {
				posSpring = ui.SpringOptions{Duration: 190 * time.Millisecond, Bounce: 0.14}
			}
		} else if s.active {
			posSpring = ui.SpringOptions{Duration: 155 * time.Millisecond, Bounce: 0.16}
		}
		left = bar.Spring("mnb-px", left, posSpring)
		width = bar.Spring("mnb-pw", width, MotionGlassScrub.Spring)
		var stretchTarget float32
		if dt := now.Sub(s.lastTime).Seconds(); dt > 0.0005 && dt < 0.25 {
			speed := abs32(left-s.prevLeft) / float32(dt)
			if standard {
				stretchTarget = clamp32(speed/1400, 0, 0.10)
			} else {
				stretchTarget = clamp32(speed/750, 0, 0.20)
			}
		}
		stretch = clamp32(bar.Spring("mnb-stretch", stretchTarget, MotionGlassScrub.Spring), 0, 0.24)
		inFlight = abs32(left-target.X) > 3 || abs32(width-target.W) > 3
		var liftTarget float32
		switch {
		case s.active || tabPressed:
			liftTarget = 1
		case inFlight:
			liftTarget = 0.76
		}
		liftSpring := GlassPressAnimation(liftTarget > 0.5).Spring
		if standard {
			if liftTarget > 0.5 {
				liftSpring = ui.SpringOptions{Duration: 200 * time.Millisecond, Bounce: 0.16}
			} else {
				liftSpring = ui.SpringOptions{Duration: 290 * time.Millisecond, Bounce: 0.18}
			}
		}
		lift = bar.Spring("mnb-lift", liftTarget, liftSpring)
	}
	s.prevLeft, s.lastTime = left, now
	alpha := clamp32(lift, 0, 1)
	elastic := clamp32(lift, -0.18, 1.28)
	dx, dy := bottomNavLensExpand(width, target.H, elastic, stretch, standard)
	r := expandGlass(ui.Rect{X: left, Y: target.Y, W: width, H: target.H}, dx, dy)
	touch := ui.Point{X: 0.5, Y: 0.5}
	optics := alpha > 0.005 && !ctx.Preferences().ReduceTransparency
	if optics && s.pressing {
		touch.X = clamp32((s.lastX-r.X)/r.W, 0.08, 0.92)
		touch.Y = clamp32((s.lastY-r.Y)/r.H, 0.12, 0.88)
	}
	return bottomNavLens{
		bounds: r, touch: touch, left: left, width: width, alpha: alpha,
		glow: max(alpha, stretch*1.25), inFlight: inFlight, optics: optics,
	}
}

func clamp32Index(v, n int) int {
	if n <= 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v >= n {
		return n - 1
	}
	return v
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
