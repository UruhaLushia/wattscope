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
	BottomNavStandardHeight float32 = 62
	BottomNavDockMargin     float32 = 12
	DesktopTopNavHeight     float32 = 32
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
	return bottomInset + max(float32(6), BottomNavDockMargin-bottomInset) + BottomNavHeightFor(style)
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
	} else if strongShadow || !p.Dark {
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
	bar := tabs.List.Label(label).Transition(NavPanelTransition(navPanelMatchKey{}))

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
			topRadius = RadiusFull
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
			toggleIcon, toggleSize := IconSidebar, float32(26)
			toggleRadius := RadiusS
			if p.LiquidGlass {
				toggleRadius = RadiusFull
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
				Transition(NavToggleTransition(navToggleMatchKey{})).
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
			statePose, stateAnim := ui.Pose{}, MotionRail
			if toggle.Pressed() {
				statePose.Scale = 0.95
				stateAnim = MotionFast
			}
			toggle.Children(func() {
				ui.Box(ctx.Key("ntog-state")).Absolute().Fill().ZIndex(-1).Radius(toggleRadius).
					TransformOrigin(0.5, 0.5).AnimatePose(statePose, stateAnim).
					Background(toggle.AnimateColor("ntog-bg", bg, MotionFast)).PassThrough()
				ui.Icon(ctx, toggleIcon).FontSize(15).
					TextColor(toggle.AnimateColor("ntog-fg", fg, MotionFast))
			})
		}

		for index, item := range items {
			entry := item
			hasChildren := sidebar && !iconsOnly && len(entry.Children) > 0
			open := hasChildren && (index == *selected || entry.Selected >= 0)
			active := index == *selected && !hasChildren
			tabRadius := RadiusS
			if sidebar {
				tabRadius = RadiusM
			}
			if p.LiquidGlass {
				if sidebar {
					tabRadius = RadiusM + 2
				} else {
					tabRadius = RadiusFull
				}
			}
			buildTab := func() {
				tab := tabs.Tab(index).Label(entry.Label).
					Radius(tabRadius).AlignItems(ui.Center).Gap(tabGap).
					Background(ui.Transparent).
					Transition(NavTabTransition(index, navTabMatchKey{index: index})).
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
						tab.Size(26, 26).Padding(0).Justify(ui.Center).Tooltip(entry.Label)
					} else {
						tab.Height(26).Padding(0, tabPad).Shrink(0)
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
						pillBg := p.TrackOn
						pillPose, pillAnim := ui.Pose{}, MotionRail
						if tab.Pressed() {
							pillBg = p.TrackOn.Mix(p.Text, 0.10)
							pillPose.Scale = 0.96
							pillAnim = MotionFast
						}
						pill := ui.Box(ctx.Key(selKey)).Absolute().Fill().ZIndex(-1).Radius(tabRadius).
							TransformOrigin(0.5, 0.5).AnimatePose(pillPose, pillAnim).
							Background(tab.AnimateColor("pill-bg", pillBg, MotionFast)).PassThrough().
							Transition(PillMatchTransition(selKey))
						if !sidebar && p.CardShadow.A > 0 && !tab.Pressed() {
							pill.Shadow(0, 1, 3, 0, p.CardShadow)
						}
					} else {
						statePose, stateAnim := ui.Pose{}, MotionRail
						if tab.Pressed() {
							statePose.Scale = 0.95
							stateAnim = MotionFast
						}
						ui.Box(ctx.Key("tab-state")).Absolute().Fill().ZIndex(-1).Radius(tabRadius).
							TransformOrigin(0.5, 0.5).AnimatePose(statePose, stateAnim).
							Background(tab.AnimateColor("tab-bg", bg, MotionFast)).PassThrough()
					}
					if entry.Icon != nil {
						iconSize := float32(15)
						if sidebar {
							iconSize = 17
						}
						ui.Icon(ctx, entry.Icon).FontSize(iconSize).
							TextColor(tab.AnimateColor("tab-fg", fg, MotionNav))
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
								PillSurface(ctx.Key("bg"), subRow, RadiusS, subBg, 0, ui.Transparent, 0.96)
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
	bottomNavPad        float32 = 7
	bottomNavGap        float32 = 2
	bottomNavStdPad     float32 = 5
	bottomNavStdGap     float32 = 2
	bottomNavInactiveW  float32 = 44
	bottomNavIconSize   float32 = 20
	bottomNavLabelGap   float32 = 6
	bottomNavLabelPadX  float32 = 13
	bottomNavDragThresh float32 = 12
)

type mnbDragState struct {
	pressing      bool
	active        bool
	upPending     bool
	canceled      bool
	suppressClick bool
	originX       float32
	lastX         float32
	dragIndex     int
	labels        []string
	widths        []float32
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

func mnbLayout(widths []float32, active int) (barW float32, tabLeft, tabW []float32) {
	tabLeft = make([]float32, len(widths))
	tabW = make([]float32, len(widths))
	x := bottomNavPad
	for i, aw := range widths {
		w := bottomNavInactiveW
		if i == active {
			w = aw
		}
		tabLeft[i] = x
		tabW[i] = w
		x += w + bottomNavGap
	}
	if len(widths) == 0 {
		return bottomNavPad * 2, tabLeft, tabW
	}
	return x - bottomNavGap + bottomNavPad, tabLeft, tabW
}

func mnbStandardLayout(ctx *ui.Context, count int, leftInset, rightInset float32) (barW float32, tabLeft, tabW []float32) {
	availW, _ := ctx.Size()
	if availW <= 0 {
		availW = 390
	}
	sidePad := max(float32(14), max(leftInset, rightInset)+8)
	barW = min(float32(420), max(float32(220), availW-sidePad*2))
	innerW := max(float32(120), barW-bottomNavStdPad*2)
	eachW := (innerW - bottomNavStdGap*float32(max(0, count-1))) / float32(max(1, count))
	tabLeft = make([]float32, count)
	tabW = make([]float32, count)
	x := bottomNavStdPad
	for i := range count {
		tabLeft[i] = x
		tabW[i] = eachW
		x += eachW + bottomNavStdGap
	}
	return barW, tabLeft, tabW
}

func mnbSlotIndexAt(x, gap float32, tabLeft, tabW []float32) int {
	for i := range tabW {
		if i == len(tabW)-1 || x < tabLeft[i]+tabW[i]+gap/2 {
			return i
		}
	}
	return max(0, len(tabW)-1)
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
	barHeight := BottomNavHeightFor(style)
	barPad, barGap := bottomNavPad, bottomNavGap
	if standard {
		barPad, barGap = bottomNavStdPad, bottomNavStdGap
	}
	tabHeight := barHeight - barPad*2

	gapAboveSafeArea := max(float32(6), BottomNavDockMargin-bottomInset)
	dock := ui.Row(ctx.Key(key+"-dock")).Absolute().Left(0).Right(0).Bottom(0).ZIndex(10).
		Justify(ui.Center).AlignItems(ui.End).PassThrough().
		Padding(0, 12, gapAboveSafeArea, 12)

	barSolid := ui.Hex("#f7fafc").Mix(p.Accent, 0.07)
	barBg := barSolid.Alpha(0.90)
	barBorder := ui.RGBA(17, 24, 39, 0.08)
	accentPill := p.Accent.Alpha(0.92).Over(barSolid)
	stdPillBg := ui.RGBA(15, 23, 42, 0.065)
	stdPillBorder := ui.Transparent
	var stdPillShadow ui.Color
	if standard {
		barBg = ui.Hex("#ffffff").Alpha(0.96)
		if p.LiquidGlass {
			stdPillBg = ui.Hex("#ffffff").Mix(p.Accent, 0.04).Alpha(0.84)
			stdPillBorder = ui.RGBA(15, 23, 42, 0.06)
			stdPillShadow = ui.RGBA(15, 23, 42, 0.09)
		}
	}
	if p.Dark {
		barSolid = p.SurfaceHi.Mix(p.Accent, 0.09)
		barBg = barSolid.Alpha(0.92)
		if standard {
			barBg = p.SurfaceHi.Alpha(0.95)
		}
		barBorder = ui.RGBA(255, 255, 255, 0.10)
		accentPill = p.Accent.Alpha(0.76).Over(barSolid)
		stdPillBg = ui.RGBA(255, 255, 255, 0.13)
		if standard && p.LiquidGlass {
			stdPillBg = ui.RGBA(255, 255, 255, 0.16)
			stdPillBorder = ui.RGBA(255, 255, 255, 0.16)
			stdPillShadow = ui.RGBA(0, 0, 0, 0.28)
		}
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
		navBarChrome(bar, p, RadiusFull, 0, barBg, barBorder, true)

		st := ui.Local(bar, "mnb-drag", func() mnbDragState { return mnbDragState{} })
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
		px, _, _ := bar.PointerPosition()

		bar.Children(func() {
			tabEls := make([]ui.Element, len(items))
			tabPressed := false
			tabCanceled := false
			tabDragEnded := false
			for index, item := range items {
				tab := tabs.Tab(index).Label(item.Label).Radius(RadiusFull).
					AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
					Clip().ZIndex(1).Background(ui.Transparent)
				if standard {
					tab.Column().Gap(2)
				} else {
					tab.Row()
				}
				tab.PointerPosition()
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
			releasedCleanly := st.upPending || barDragEnded || tabDragEnded

			if !releasedCleanly && !anyPressed {
				st.suppressClick = false
			}

			layoutFor := func(activeIdx int) (float32, []float32, []float32) {
				if standard {
					return mnbStandardLayout(ctx, len(items), leftInset, rightInset)
				}
				return mnbLayout(st.widths, activeIdx)
			}

			curIdx := clamp32Index(*selected, len(items))
			if st.active {
				curIdx = clamp32Index(st.dragIndex, len(items))
			}
			barW, tabLeft, tabW := layoutFor(curIdx)
			if standard {
				bar.Width(barW)
			}

			switch {
			case releasedCleanly:
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
					st.lastX = px
					st.dragIndex = clamp32Index(*selected, len(items))
				}
				st.lastX = px
				if !st.active && abs32(px-st.originX) >= bottomNavDragThresh {
					st.active = true
				}
				if st.active {
					st.dragIndex = mnbSlotIndexAt(px, barGap, tabLeft, tabW)
				}
			}

			displayIndex := clamp32Index(*selected, len(items))
			if st.active {
				displayIndex = clamp32Index(st.dragIndex, len(items))
			}
			if displayIndex != curIdx {
				barW, tabLeft, tabW = layoutFor(displayIndex)
			}

			capsulePose, capsuleAnim := ui.Pose{}, MotionNav
			if st.active && barW > 0 {
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

			pillLeft, pillW := tabLeft[displayIndex], tabW[displayIndex]
			pillBg := accentPill
			if standard {
				pillBg = stdPillBg
			}
			pillAnim := MotionCapsulePill
			pillPose, pillPoseAnim := ui.Pose{}, MotionNav
			if st.active {
				innerMin := barPad
				innerMax := max(innerMin, barW-barPad-pillW)
				pillLeft = clamp32(st.lastX-pillW/2, innerMin, innerMax)
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
			if stdPillBorder.A > 0 {
				pill.Border(1, stdPillBorder)
			}
			if stdPillShadow.A > 0 {
				pill.Shadow(0, 2, 8, 0, stdPillShadow)
			} else if !standard && p.CardShadow.A > 0 && !tabEls[displayIndex].Pressed() {
				pill.Shadow(0, 1, 3, 0, p.CardShadow)
			}

			hoverBg, pressBg := navInactiveOverlayColors(p, standard)

			for index, item := range items {
				entry := item
				active := index == displayIndex
				tab := tabEls[index].
					Size(tabW[index], tabHeight).
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

				cx := tabLeft[index] + tabW[index]/2
				coveredByPill := st.active && cx >= pillLeft+6 && cx <= pillLeft+pillW-6

				var stateBg ui.Color
				fg := p.Text.Alpha(0.84)
				if standard {
					fg = p.Text.Alpha(0.76)
				}
				activeFg := pillInk
				if standard {
					activeFg = p.AccentText
				}
				colorAnim := MotionNav
				switch {
				case active && !st.active && tab.Pressed():
					fg = activeFg.Alpha(0.90)
				case active || coveredByPill:
					fg = activeFg
					if st.active {
						colorAnim = MotionFast
					}
				case !st.active && tab.Pressed():
					stateBg, fg = pressBg, p.Text
				case !st.active && (tab.Hovered() || tab.FocusVisible()):
					stateBg, fg = hoverBg, p.Text
				default:
					if st.active {
						colorAnim = MotionFast
					}
				}

				tab.Children(func() {
					if !active && !st.active {
						statePose, stateAnim := ui.Pose{}, MotionNav
						if tab.Pressed() {
							statePose.Scale = 0.95
							stateAnim = MotionFast
						}
						ui.Box(ctx.Key("mnb-state")).Absolute().Fill().ZIndex(-1).Radius(RadiusFull).
							TransformOrigin(0.5, 0.5).AnimatePose(statePose, stateAnim).
							Background(tab.AnimateColor("mnb-bg", stateBg, MotionFast)).PassThrough()
					}
					if entry.Icon != nil {
						ui.Icon(ctx, entry.Icon).FontSize(bottomNavIconSize).Shrink(0).
							TextColor(tab.AnimateColor("mnb-fg", fg, colorAnim))
					}
					if standard {
						weight := 500
						if active || coveredByPill {
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
			}
		})
	})
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
