package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// BottomNavStyle selects the mobile bottom navigation bar layout style.
type BottomNavStyle uint8

// Bottom navigation style presets.
const (
	BottomNavCapsule BottomNavStyle = iota
	BottomNavStandard
)

// Navigation layout dimensions and margins shared between kit navigation bars and workspace layouts.
const (
	BottomNavHeight              float32 = 56
	BottomNavStandardHeight      float32 = 58
	BottomNavDockMargin          float32 = 16
	DesktopTopNavHeight          float32 = 32
	mobileTopNavHeight           float32 = 38
	desktopTopNavControl         float32 = 26
	mobileTopNavControl          float32 = 32
	DesktopSidebarWidth          float32 = 196
	DesktopCompactRailWidth      float32 = 60
	DesktopCompactLandscapeRailW float32 = 52
	DesktopSidebarMargin         float32 = 12
	CompactSidebarMargin         float32 = 8
	compactSidebarMaxHeight      float32 = 520
)

// CompactSidebarSideMargin returns the outer inset margin of the floating sidebar card.
func CompactSidebarSideMargin(winH float32) float32 {
	if winH > 0 && winH < compactSidebarMaxHeight {
		return CompactSidebarMargin
	}
	return DesktopSidebarMargin
}

// DesktopSidebarRailWidth returns the width of the floating sidebar rail for the given mode and window height.
func DesktopSidebarRailWidth(iconsOnly bool, winH float32) float32 {
	if !iconsOnly {
		return DesktopSidebarWidth
	}
	if winH > 0 && winH < compactSidebarMaxHeight {
		return DesktopCompactLandscapeRailW
	}
	return DesktopCompactRailWidth
}

func desktopNavTopBarHeight(touch bool) float32 {
	if touch {
		return mobileTopNavHeight
	}
	return DesktopTopNavHeight
}

// DesktopNavTopControlHeight returns the control/tab height inside the top navigation capsule.
func DesktopNavTopControlHeight(touch bool) float32 {
	if touch {
		return mobileTopNavControl
	}
	return desktopTopNavControl
}

func bottomNavHeightFor(style BottomNavStyle) float32 {
	if style == BottomNavStandard {
		return BottomNavStandardHeight
	}
	return BottomNavHeight
}

// BottomNavReserveFor returns the bottom viewport height reserved for the floating bottom navigation dock.
func BottomNavReserveFor(bottomInset float32, visible bool, style BottomNavStyle) float32 {
	if !visible {
		return 0
	}
	return bottomInset + bottomNavDockGap(bottomInset) + bottomNavHeightFor(style)
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

func navBarChrome(ctx *ui.Context, bar ui.Element, p Palette, radius, clarity float32, fallbackBg, fallbackBorder ui.Color, strongShadow bool) {
	if p.LiquidGlass {
		bar.Shadow(0, 4, 16, 0, navShadowColor(p, strongShadow))
		glassFloatingUnclipped(ctx, bar, p, radius, clarity, fallbackBg, fallbackBorder)
		return
	}
	if strongShadow || !p.Dark {
		bar.Shadow(0, 4, 16, 0, navShadowColor(p, strongShadow))
	}
	glassFloatingEl(ctx, bar, p, radius, clarity, false, fallbackBg, fallbackBorder)
}

func navInactiveOverlayColors(p Palette, standard bool) (hoverBg, pressBg ui.Color) {
	if !standard {
		return p.Hover, p.Press
	}
	return glassNeutral(p.Dark, 0.045, 0.07), glassNeutral(p.Dark, 0.085, 0.12)
}

func glassNavPillBg(p Palette) ui.Color {
	return glassNeutral(p.Dark, 0.065, 0.14)
}

// DesktopNavOptions configures DesktopNav layout mode, spacing, and collapse toggle.
type DesktopNavOptions struct {
	Sidebar       bool
	IconsOnly     bool
	Touch         bool
	TabPad        float32
	TabGap        float32
	OnToggle      func()
	ToggleLabel   string
	ToggleTooltip string
}

type desktopNavScrubState struct {
	navScrubState
	slots []navSlot
}

// DesktopNav renders desktop navigation as either a floating header capsule or
// an Apple-style floating inset sidebar card, keeping element identity alive
// across transitions so every destination flies cleanly between layouts.
func DesktopNav(ctx *ui.Context, p Palette, label string, selected *int, items []NavTab, opts DesktopNavOptions) {
	if len(items) == 0 {
		return
	}
	if selected == nil {
		var scratch int
		selected = &scratch
	}
	navKey, selKey := "top-pages", "top-nav-selection"
	if opts.Sidebar {
		navKey, selKey = "side-pages", "side-nav-selection"
	}

	tabs := ui.TabsBase(ctx.Key(navKey), selected, len(items))
	bar := tabs.List.Label(label)
	if opts.Sidebar {
		bar.FocusGroup(ui.Vertical)
	}
	if !IsOrientationRotating(ctx) {
		bar.Transition(navMatchTransition(ctx, navPanelMatchKey{}, true))
	} else {
		startPose := ui.Pose{Y: -10, Scale: 0.94, Fade: 0.65}
		if opts.Sidebar {
			startPose = ui.Pose{X: -16, Scale: 0.94, Fade: 0.65}
		}
		ApplyRotateEnterPose(ctx, bar.TransformOrigin(0.5, 0.5), "rot-dnav", startPose, motionRotatePanel)
	}
	_, winH := ctx.Size()
	compactSidebar := opts.Sidebar && winH > 0 && winH < compactSidebarMaxHeight
	sideTabH := float32(36)
	if compactSidebar {
		sideTabH = 34
	}
	topBarH := desktopNavTopBarHeight(opts.Touch)
	topCtrlH := DesktopNavTopControlHeight(opts.Touch)
	railW := DesktopSidebarRailWidth(opts.IconsOnly, winH)
	sideMargin, sidePad, sideGap := CompactSidebarSideMargin(winH), float32(8), float32(3)
	if compactSidebar {
		sidePad, sideGap = 6, 2
	}

	if opts.Sidebar {
		sideRadius := RadiusL + 2
		if p.LiquidGlass {
			sideRadius = RadiusXL
		}
		if compactSidebar && opts.IconsOnly {
			sideRadius = RadiusM + sidePad
		}
		bar.Column().Width(railW).Shrink(0).
			Margin(sideMargin, 0, sideMargin, sideMargin).
			Padding(sidePad).Gap(sideGap)
		navBarChrome(ctx, bar, p, sideRadius, 0.22, p.Surface, p.LineSoft, true)
		if opts.IconsOnly {
			bar.AlignItems(ui.Center)
		} else {
			bar.AlignItems(ui.Start)
		}
	} else {
		topRadius := RadiusM
		if p.LiquidGlass || opts.Touch {
			// Interpolate from the visible corner radius during layout changes.
			topRadius = topBarH / 2
		}
		bar.Row().Height(topBarH).Shrink(0).AlignItems(ui.Center).
			Padding(3).Gap(2)
		navBarChrome(ctx, bar, p, topRadius, 0, p.Track, p.LineSoft, false)
		if opts.Touch {
			bar.Margin(0, 0, 0, 4)
		} else if !opts.IconsOnly {
			bar.Margin(0, 0, 0, 6)
		}
	}

	bar.Children(func() {
		if opts.OnToggle != nil {
			desktopNavToggle(ctx, p, opts, compactSidebar, topCtrlH)
		}

		axis, stripGap := navAxisHorizontal, float32(2)
		var tabsStrip ui.Element
		if opts.Sidebar {
			axis, stripGap = navAxisVertical, sideGap
			tabsStrip = ui.Column(ctx.Key("dnav-strip")).Gap(stripGap)
			if opts.IconsOnly {
				tabsStrip.AlignItems(ui.Center).Shrink(0)
			} else {
				tabsStrip.FillWidth().AlignItems(ui.Start)
			}
			tabsStrip.Draggable(nil, ui.DraggableOptions{Axis: ui.Vertical, Bounds: &ui.Rect{}, Rigid: true})
		} else {
			tabsStrip = ui.Row(ctx.Key("dnav-strip")).AlignItems(ui.Center).Gap(stripGap).Shrink(0).
				Draggable(nil, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &ui.Rect{}, Rigid: true})
		}

		st := ui.Local(tabsStrip, "dnav-scrub", func() desktopNavScrubState { return desktopNavScrubState{} })
		if len(st.slots) != len(items) {
			st.slots = make([]navSlot, len(items))
		}
		tabsStrip.ObserveInput(st.observePointer(axis, stripGap, st.slots, *selected, len(items), true))
		stripBounds := tabsStrip.LayoutBounds()

		tabRadius := RadiusS
		if opts.Sidebar {
			tabRadius = glassRadius(p, RadiusM, RadiusM+2)
		} else if p.LiquidGlass || opts.Touch {
			tabRadius = topCtrlH / 2
		}

		tabsStrip.Children(func() {
			var inlineTabs [8]ui.Element
			tabEls := inlineTabs[:0]
			if len(items) > len(inlineTabs) {
				tabEls = make([]ui.Element, 0, len(items))
			}

			for index, entry := range items {
				tab := tabs.Tab(index).Label(entry.Label).
					Radius(tabRadius).AlignItems(ui.Center).Gap(opts.TabGap).
					Background(ui.Transparent).
					Transition(navMatchTransition(ctx, navTabMatchKey{index: index}, false))
				if opts.Sidebar {
					if opts.IconsOnly {
						tab.Size(sideTabH, sideTabH).Padding(0).Justify(ui.Center).Tooltip(entry.Label)
					} else {
						tab.FillWidth().Height(sideTabH).Padding(0, 10)
					}
				} else {
					if opts.IconsOnly {
						tab.Size(topCtrlH, topCtrlH).Padding(0).Justify(ui.Center).Tooltip(entry.Label)
					} else {
						tab.Height(topCtrlH).Padding(0, opts.TabPad).Shrink(0)
					}
				}
				tabEls = append(tabEls, tab)
			}

			mode := desktopNavMode(opts, p.LiquidGlass, compactSidebar)
			modeChanged := !st.hasPrev || st.lastMode != mode || st.lastCount != len(items)
			sideBorder := float32(0)
			if !p.LiquidGlass {
				sideBorder = 2
			}
			stripMain, stripCross, crossOff := resolveDesktopNavSlots(ctx, st.slots, tabEls, items, opts, stripBounds, stripGap, sideTabH, topCtrlH, railW-2*sidePad-sideBorder, modeChanged)

			displayIndex, _ := st.stepTabs(ctx, tabsStrip, tabEls, axis, stripGap, st.slots, p.LiquidGlass, true, selected, items)

			if !p.LiquidGlass && !IsOrientationRotating(ctx) {
				stripPose, stripAnim := navSolidScrubPose(axis, st.active, navMainCoord(axis, st.lastX, st.lastY), stripMain, 3.0, 6)
				tabsStrip.TransformOrigin(0.5, 0.5).AnimatePose(stripPose, stripAnim)
			}

			pillStart, pillExtent := st.slots[displayIndex].start, st.slots[displayIndex].extent
			lastMain := navMainCoord(axis, st.lastX, st.lastY)
			if st.active {
				pillStart = clamp32(lastMain-pillExtent*0.5, 0, max(0, stripMain-pillExtent))
			}

			reduceTransparency := ctx.Preferences().ReduceTransparency
			target := ui.Rect{X: pillStart, Y: crossOff, W: pillExtent, H: stripCross}
			if opts.Sidebar {
				target = ui.Rect{X: crossOff, Y: pillStart, W: stripCross, H: pillExtent}
			}
			var lens navGlassLens
			if p.LiquidGlass {
				lens = st.animateGlassLens(ctx, tabsStrip, navLensConfig{
					axis: axis, target: target, mode: mode, count: len(items), keyPrefix: "dnav",
					style: navLensDesktopStyle, tabPressed: tabEls[displayIndex].Pressed(),
					forceJump: modeChanged,
					expand: func(mainExt, crossExt, elastic, stretch float32) (dx, dy, padCross float32) {
						return desktopNavLensExpand(axis, mainExt, crossExt, elastic, stretch)
					},
				})
				pillBg := glassNavPillBg(p)
				if !reduceTransparency {
					pillBg = scaleAlpha(pillBg, 1-lens.alpha)
				}
				pill := lens.box(ctx.Key(selKey), tabRadius, 0).
					Background(tabsStrip.AnimateColor("dnav-pill-bg", pillBg, MotionFast))
				if !opts.Sidebar && p.CardShadow.A > 0 && lens.alpha < 0.5 {
					pill.Shadow(0, 1, 2, 0, scaleAlpha(p.CardShadow, 1-lens.alpha))
				}
			} else {
				st.hasPrev, st.lastMode, st.lastCount = true, mode, len(items)
				pillBg, pillPose, pillAnim, pillPoseAnim := navSolidPillFeedback(
					st.active, tabEls[displayIndex].Pressed(), lastMain, stripMain,
					p.TrackOn, p.Text, 0.08, 0.10,
				)
				renderNavSolidPill(ctx, tabsStrip, selKey, target, tabRadius, "dnav-pill-bg",
					pillBg, pillPose, pillAnim, pillPoseAnim,
					!modeChanged && !isStaticSnapshot(ctx) && !isScaleReflow(ctx),
					!opts.Sidebar && !tabEls[displayIndex].Pressed() && !st.active, 2, p.CardShadow,
				)
			}

			lensMoving := st.active || lens.inFlight
			glassOptics := p.LiquidGlass && !reduceTransparency
			hoverBg, pressBg := navInactiveOverlayColors(p, p.LiquidGlass)
			idleFg, activeFg := p.Muted, p.TrackOnInk
			if p.LiquidGlass {
				idleFg, activeFg = p.Text.Alpha(0.76), p.AccentText
			}

			for index, item := range items {
				entry := item
				active := index == displayIndex
				tab := tabEls[index].OnClick(navTabClick(&st.navScrubState, entry.OnSelect))

				cMain := st.slots[index].start + st.slots[index].extent*0.5
				cover, _ := lens.coverAt(axis, cMain, 0.85)
				pressed := tab.Pressed()

				var stateBg ui.Color
				fg, weight := idleFg, 550
				colorAnim := motionNav
				iconPop := float32(0)

				z := 1
				if glassOptics {
					z = 2
				}
				tab.ZIndex(z)

				handled := false
				if glassOptics {
					switch {
					case lensMoving:
						handled, colorAnim = true, MotionFast
						fg = idleFg.Mix(activeFg, clamp32(cover*lens.alpha, 0, 1))
						iconPop = cover * lens.alpha
					case active:
						handled, fg = true, activeFg
						if pressed {
							iconPop = max(lens.alpha, 1)
						} else if lens.alpha > 0.01 {
							iconPop = cover * lens.alpha
						}
					}
				} else {
					covered := active || (p.LiquidGlass && lensMoving && cover > 0.5)
					switch {
					case active && !st.active && pressed:
						handled, fg = true, activeFg
						if !p.LiquidGlass {
							fg = activeFg.Alpha(0.90)
						}
					case covered:
						handled, fg = true, activeFg
						if lensMoving {
							colorAnim = MotionFast
						}
					}
				}
				if !handled {
					allowHover := glassOptics || !st.active
					switch {
					case allowHover && pressed:
						stateBg, fg = pressBg, p.Text
					case allowHover && (tab.Hovered() || tab.FocusVisible()):
						stateBg, fg = hoverBg, p.Text
					case st.active:
						colorAnim = MotionFast
					}
					if glassOptics && pressed {
						iconPop = 1
					}
				}

				tab.Children(func() {
					if !active && !st.active {
						GlassPillControl(ctx.Key("tab-state"), tab, p, GlassPillOptions{
							Radius:             tabRadius,
							FallbackBackground: stateBg,
							PressScale:         0.95,
						})
					}
					if entry.Icon != nil {
						iconSize := float32(15)
						if opts.Sidebar {
							iconSize = 17
						}
						glassIconPop(ctx, ui.Icon(ctx, entry.Icon).FontSize(iconSize).
							TextColor(tab.AnimateColor("tab-fg", fg, colorAnim)), p, iconPop)
					}
					if !opts.IconsOnly {
						AnimatedLabel(ctx, entry.Label).FontSize(13).FontWeight(weight).SingleLine().Shrink(0).
							TextColor(tab.AnimateColor("tab-text", fg, colorAnim))
					}
				})
			}

			if lens.optics {
				lensRadius := max(1, tabRadius+max(lens.padCross, 0))
				lensEl := lens.box(ctx.Key(selKey+"-lens"), lensRadius, 3)
				navLensShadow(lensEl, p.Dark, lens.alpha, false)
				spec := liquidLensSpec{
					Alpha:     lens.alpha,
					Glow:      lens.glow,
					Touch:     lens.touch,
					ClearLens: true,
				}
				if opts.Sidebar {
					spec.Zoom = 0.90
					spec.ZoomY = 1 + 0.14*lens.alpha
				} else {
					spec.Zoom = 1 + 0.14*lens.alpha
					spec.ZoomY = 0.90
				}
				drawNavGlassLens(ctx, lensEl, lensRadius, spec)
			}
		})
	})
}

func desktopNavMode(opts DesktopNavOptions, liquidGlass, compactSidebar bool) uint8 {
	var mode uint8
	if opts.Sidebar {
		mode |= 1
	}
	if opts.IconsOnly {
		mode |= 2
	}
	if liquidGlass {
		mode |= 4
	}
	if opts.Touch {
		mode |= 8
	}
	if compactSidebar {
		mode |= 16
	}
	return mode
}

func resolveDesktopNavSlots(
	ctx *ui.Context,
	slots []navSlot,
	tabEls []ui.Element,
	items []NavTab,
	opts DesktopNavOptions,
	stripBounds ui.Rect,
	stripGap, sideTabH, topCtrlH, sideFullW float32,
	modeChanged bool,
) (stripMain, stripCross, crossOff float32) {
	if !modeChanged && stripBounds.W > 0 && stripBounds.H > 0 {
		allValid := true
		for i, tab := range tabEls {
			tb := tab.LayoutBounds()
			if tb.W <= 0 || tb.H <= 0 {
				allValid = false
				break
			}
			if opts.Sidebar {
				slots[i] = navSlot{start: tb.Y - stripBounds.Y, extent: tb.H}
			} else {
				slots[i] = navSlot{start: tb.X - stripBounds.X, extent: tb.W}
			}
		}
		if allValid {
			tb0 := tabEls[0].LayoutBounds()
			if opts.Sidebar {
				return stripBounds.H, tb0.W, tb0.X - stripBounds.X
			}
			return stripBounds.W, tb0.H, tb0.Y - stripBounds.Y
		}
	}

	if opts.Sidebar {
		y := float32(0)
		for i := range items {
			slots[i] = navSlot{start: y, extent: sideTabH}
			y += sideTabH
			if i+1 < len(items) {
				y += stripGap
			}
		}
		cross := sideTabH
		if !opts.IconsOnly {
			cross = sideFullW
		}
		return y, cross, 0
	}

	x := float32(0)
	for i, entry := range items {
		w := topCtrlH
		if !opts.IconsOnly {
			tw, _ := ctx.MeasureText(1<<20, ui.Span{Text: entry.Label, Size: 13, Weight: 550})
			w = 2*opts.TabPad + tw
			if entry.Icon != nil {
				w += 15 + opts.TabGap
			}
		}
		slots[i] = navSlot{start: x, extent: w}
		x += w
		if i+1 < len(items) {
			x += stripGap
		}
	}
	return x, topCtrlH, 0
}

func desktopNavLensExpand(axis navScrubAxis, mainExtent, crossExtent, elastic, stretch float32) (dx, dy, padCross float32) {
	if axis == navAxisVertical {
		mainEx := clamp32(mainExtent*0.22, 6.0, 10.5)*elastic + mainExtent*stretch*0.35
		crossEx := clamp32(crossExtent*0.08, 4.0, 7.5)*elastic - min(crossExtent, 48)*stretch*0.04
		return crossEx, mainEx, crossEx
	}
	mainEx := clamp32(mainExtent*0.18, 6.0, 13.0)*elastic + mainExtent*stretch*0.32
	crossEx := clamp32(crossExtent*0.20, 4.5, 7.5)*elastic - crossExtent*stretch*0.05
	return mainEx, crossEx, crossEx
}

func desktopNavToggle(ctx *ui.Context, p Palette, opts DesktopNavOptions, compactSidebar bool, topCtrlH float32) {
	toggleIcon, toggleSize := IconSidebar, topCtrlH
	toggleRadius := RadiusS
	if p.LiquidGlass || opts.Touch {
		toggleRadius = toggleSize / 2
	}
	if opts.Sidebar {
		toggleIcon, toggleSize = IconTopBar, 28
		if compactSidebar && opts.IconsOnly {
			toggleSize = 30
		}
		if p.LiquidGlass {
			toggleRadius = RadiusM + 2
		}
	}
	toggle := ui.ButtonBase(ctx.Key("nav-toggle")).Size(toggleSize, toggleSize).
		Shrink(0).Radius(toggleRadius).Center().
		Label(opts.ToggleLabel).Tooltip(opts.ToggleTooltip).
		Transition(navMatchTransition(ctx, navToggleMatchKey{}, false)).
		OnClick(opts.OnToggle)
	if opts.Sidebar {
		marginBot := float32(4)
		if compactSidebar && opts.IconsOnly {
			marginBot = 2
		}
		if opts.IconsOnly {
			toggle.Margin(0, 0, marginBot, 0)
		} else {
			toggle.Margin(0, 0, marginBot, 5)
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
		GlassPillControl(ctx.Key("ntog-state"), toggle, p, GlassPillOptions{
			Radius:             toggleRadius,
			FallbackBackground: bg,
			PressScale:         0.95,
		})
		GlassIconPopOnPress(ctx, p, toggle, ui.Icon(ctx, toggleIcon).FontSize(15).
			TextColor(toggle.AnimateColor("ntog-fg", fg, MotionFast)))
	})
}

// NavTab describes one destination tab for DesktopNav, SectionTabs, and BottomNavigationBar.
type NavTab struct {
	Label    string
	Icon     *ui.SVG
	OnSelect func()
}

// SectionTabs renders horizontal in-page section tabs with Liquid Glass pill feedback.
func SectionTabs(ctx *ui.Context, p Palette, label string, selected int, items []NavTab) {
	tabRadius := PillRadius(p, RadiusM)
	hoverBg, pressBg := navInactiveOverlayColors(p, p.LiquidGlass)
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
				fg, weight = p.AccentText, 650
				visibleAtRest = true
				if p.LiquidGlass {
					bg = glassNavPillBg(p)
				} else {
					bg = p.AccentSoft
					glassTint = GlassAccentTint(p, 0.24, 0.28)
				}
			case tab.Pressed():
				bg, fg = pressBg, p.Text
			case tab.Hovered() || tab.FocusVisible():
				bg, fg = hoverBg, p.Text
			}
			ControlMotion(tab).Children(func() {
				GlassPillControl(ctx.Key("bg"), tab, p, GlassPillOptions{
					Radius:             tabRadius,
					Tint:               glassTint,
					VisibleAtRest:      visibleAtRest,
					FallbackBackground: bg,
					PressScale:         0.96,
				})
				AnimatedLabel(ctx, item.Label).FontSize(13).FontWeight(weight).TextColor(fg).SingleLine()
			})
		}
	})
}

const (
	bottomNavPad       float32 = 7
	bottomNavGap       float32 = 2
	bottomNavStdPad    float32 = 4
	bottomNavStdGap    float32 = 2
	bottomNavStdSlot   float32 = 63
	bottomNavInactiveW float32 = 44
	bottomNavIconSize  float32 = 20
	bottomNavLabelGap  float32 = 6
	bottomNavLabelPadX float32 = 13
)

type mnbDragState struct {
	navScrubState
	labels []string
	widths []float32
	slots  []navSlot
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

func layoutBottomNav(slots []navSlot, pad, gap, width float32, active int, activeWidth float32) float32 {
	x := pad
	for i := range slots {
		w := width
		if i == active {
			w = activeWidth
		}
		slots[i] = navSlot{start: x, extent: w}
		x += w + gap
	}
	return x - gap + pad
}

func layoutStandardBottomNav(ctx *ui.Context, slots []navSlot, leftInset, rightInset float32) float32 {
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

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// BottomNavigationBarOptions configures BottomNavigationBar presentation style and safe-area insets.
type BottomNavigationBarOptions struct {
	Style       BottomNavStyle
	BottomInset float32
	LeftInset   float32
	RightInset  float32
}

// BottomNavigationBar renders a centered floating navigation bar above the bottom
// safe area in either compact Capsule style (active tab unfolds label) or Standard
// style (equal icon-above-label tabs with a sliding glass highlight pill). Holding
// and dragging horizontally scrubs the active indicator pill across destinations.
func BottomNavigationBar(ctx *ui.Context, p Palette, key, label string, selected *int, items []NavTab, opts BottomNavigationBarOptions) {
	if len(items) == 0 {
		return
	}
	if selected == nil {
		var scratch int
		selected = &scratch
	}
	standard := opts.Style == BottomNavStandard
	standardGlass := standard && p.LiquidGlass
	capsuleGlass := !standard && p.LiquidGlass
	barHeight := bottomNavHeightFor(opts.Style)
	barPad, barGap := bottomNavPad, bottomNavGap
	if standard {
		barPad, barGap = bottomNavStdPad, bottomNavStdGap
	} else if p.LiquidGlass {
		barPad = 5
	}
	tabHeight := barHeight - barPad*2

	dock := ui.Row(ctx.Key(key+"-dock")).Absolute().Left(0).Right(0).Bottom(0).ZIndex(10).
		Justify(ui.Center).AlignItems(ui.End).PassThrough().
		Padding(0, opts.RightInset+12, opts.BottomInset+bottomNavDockGap(opts.BottomInset), opts.LeftInset+12).
		TransformOrigin(0.5, 0.85)
	ApplyRotateEnterPose(ctx, dock, "rot-mnb", ui.Pose{Y: 20, Scale: 0.94, Fade: 0.70}, motionRotatePanel)

	barSolid := ui.Hex("#f7fafc").Mix(p.Accent, 0.07)
	barBg := barSolid.Alpha(0.90)
	barBorder := ui.RGBA(17, 24, 39, 0.08)
	accentPill := p.Accent.Alpha(0.92).Over(barSolid)
	stdPillBg := glassNavPillBg(p)
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
	}
	pillInk := AccentInk(accentPill)

	selKey := key + "-pill"

	dock.Children(func() {
		tabs := ui.TabsBase(ctx.Key(key), selected, len(items))
		bar := tabs.List.Label(label).Row().Shrink(0).Height(barHeight).AlignItems(ui.Center).
			Padding(barPad).Gap(barGap).
			Draggable(nil, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &ui.Rect{}, Rigid: true})
		if !isStaticSnapshot(ctx) {
			bar.Transition(ui.ElementTransition{Colors: true, Shadows: true, Animation: &motionNavPanel})
		}
		if !p.LiquidGlass {
			navBarChrome(ctx, bar, p, RadiusFull, 0, barBg, barBorder, true)
		} else if !standard && !p.Dark {
			bar.Shadow(0, 2, 12, 0, ui.RGBA(12, 18, 30, 0.065))
		}

		st := ui.Local(bar, "mnb-drag", func() mnbDragState { return mnbDragState{} })
		if len(st.slots) != len(items) {
			st.slots = make([]navSlot, len(items))
		}
		slots := st.slots
		if !standard {
			st.ensureWidths(ctx, items)
		}

		bar.ObserveInput(st.observePointer(navAxisHorizontal, barGap, slots, *selected, len(items), false))

		bar.Children(func() {
			var inlineTabs [4]ui.Element
			tabEls := inlineTabs[:]
			if len(items) > len(tabEls) {
				tabEls = make([]ui.Element, len(items))
			}
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
				tabEls[index] = tab
			}

			curIdx := clamp32Index(*selected, len(items))
			if st.active && !p.LiquidGlass {
				curIdx = clamp32Index(st.dragIndex, len(items))
			}
			var barW float32
			if standard {
				barW = layoutStandardBottomNav(ctx, slots, opts.LeftInset, opts.RightInset)
				bar.Width(barW)
			} else {
				barW = layoutBottomNav(slots, barPad, barGap, bottomNavInactiveW, curIdx, st.widths[curIdx])
			}

			displayIndex, anyPressed := st.stepTabs(ctx, bar, tabEls, navAxisHorizontal, barGap, slots, p.LiquidGlass, false, selected, items)
			if !standard && !p.LiquidGlass && displayIndex != curIdx {
				barW = layoutBottomNav(slots, barPad, barGap, bottomNavInactiveW, displayIndex, st.widths[displayIndex])
			}

			capsulePose, capsuleAnim := ui.Pose{}, motionNav
			if !p.LiquidGlass {
				capsulePose, capsuleAnim = navSolidScrubPose(navAxisHorizontal, st.active, st.lastX, barW, 3.5, 8)
			}
			bar.TransformOrigin(0.5, 0.5).AnimatePose(capsulePose, capsuleAnim)

			pillLeft, pillW := slots[displayIndex].start, slots[displayIndex].extent
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
			var lens navGlassLens
			if p.LiquidGlass {
				st.applyDockGlass(ctx, bar, p, barW, barHeight, anyPressed || st.active, standard)
				var mode uint8
				lensStyle := navLensBottomCapsuleStyle
				if standard {
					mode = 1
					lensStyle = navLensBottomStandardStyle
				}
				lens = st.animateGlassLens(ctx, bar, navLensConfig{
					axis: navAxisHorizontal, target: ui.Rect{X: pillLeft, Y: barPad, W: pillW, H: tabHeight},
					mode: mode, count: len(items), keyPrefix: "mnb", style: lensStyle,
					tabPressed: tabEls[displayIndex].Pressed(),
					expand: func(mainExt, crossExt, elastic, stretch float32) (dx, dy, padCross float32) {
						dx, dy = bottomNavLensExpand(mainExt, crossExt, elastic, stretch, standard)
						return dx, dy, 0
					},
				})
				pillCheckLeft, pillCheckW = lens.base.X, lens.base.W

				if !ctx.Preferences().ReduceTransparency {
					pillBg = scaleAlpha(pillBg, 1-lens.alpha)
				}
				pill := lens.box(ctx.Key(selKey), RadiusFull, 0).
					Background(bar.AnimateColor("mnb-pill-bg", pillBg, MotionFast))
				if lens.alpha > 0.01 {
					navLensShadow(pill, p.Dark, lens.alpha, standard)
				}
			} else {
				st.hasPrev = false
				pillBg, pillPose, pillAnim, pillPoseAnim := navSolidPillFeedback(
					st.active, tabEls[displayIndex].Pressed(), st.lastX, barW,
					pillBg, p.Text, 0.06, 0.08,
				)
				renderNavSolidPill(ctx, bar, selKey, ui.Rect{X: pillLeft, Y: barPad, W: pillW, H: tabHeight},
					RadiusFull, "mnb-pill-bg", pillBg, pillPose, pillAnim, pillPoseAnim,
					true, !standard && !tabEls[displayIndex].Pressed(), 3, p.CardShadow,
				)
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
			lensHalf := max(lens.bounds.W*0.5, 1)

			for index, item := range items {
				entry := item
				active := index == displayIndex
				slot := slots[index]
				tab := tabEls[index].
					Size(slot.extent, tabHeight).
					Transition(ui.ElementTransition{
						Position: true, Size: true, Animation: &motionNav,
					}).
					OnClick(navTabClick(&st.navScrubState, entry.OnSelect))

				cx := slot.start + slot.extent/2
				coveredByPill := lensMoving && cx >= pillCheckLeft+6 && cx <= pillCheckLeft+pillCheckW-6
				pressed := tab.Pressed()
				tapPressed := !active && !st.active && pressed
				iconPop := float32(0)
				if standardGlass {
					tab.ZIndex(2)
				} else {
					switch {
					case active || coveredByPill:
						tab.ZIndex(3)
						iconPop = lens.alpha
					case tapPressed && p.LiquidGlass:
						tab.ZIndex(3)
						iconPop = 1.0
					default:
						tab.ZIndex(1)
					}
					if capsuleGlass && !isStaticSnapshot(ctx) {
						tabScale := float32(1)
						tabScaleAnim := motionGlassRelease
						if iconPop > 0.005 {
							tabScale = 1 + 0.30*clamp32(iconPop, 0, 1.25)
							tabScaleAnim = motionGlassPress
						}
						tab.TransformOrigin(0.5, 0.5).AnimatePose(ui.Pose{Scale: tabScale}, tabScaleAnim)
					}
				}

				var stateBg ui.Color
				fg := idleFg
				colorAnim := motionNav
				var lensDistance float32
				switch {
				case followLens:
					var cover float32
					cover, lensDistance = lens.coverAt(navAxisHorizontal, cx, 0.80)
					fg = idleFg.Mix(activeFg, clamp32(cover*lens.alpha, 0, 1))
					colorAnim = MotionFast
				case !standardGlass && active && !st.active && pressed:
					fg = activeFg
					if !p.LiquidGlass {
						fg = activeFg.Alpha(0.90)
					}
				case active || (!standardGlass && coveredByPill):
					fg = activeFg
					if !standardGlass && lensMoving {
						colorAnim = MotionFast
					}
				case tapPressed:
					stateBg, fg = pressBg, p.Text
				case !st.active && (tab.Hovered() || tab.FocusVisible()):
					stateBg, fg = hoverBg, p.Text
				case st.active:
					colorAnim = MotionFast
				}

				tab.Children(func() {
					if !active && !st.active {
						if p.LiquidGlass {
							if standard {
								GlassPillControl(ctx.Key("mnb-state"), tab, p, GlassPillOptions{
									Radius:             RadiusFull,
									FallbackBackground: stateBg,
									PressScale:         0.985,
								})
							} else if !tapPressed {
								GlassPillControl(ctx.Key("mnb-state"), tab, p, GlassPillOptions{
									Radius:             RadiusFull,
									FallbackBackground: stateBg,
									PressScale:         0.95,
								})
							}
						} else {
							statePose, stateAnim := ui.Pose{}, motionNav
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
						glassIconPop(ctx, ui.Icon(ctx, entry.Icon).FontSize(bottomNavIconSize).Shrink(0).
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
								Animation: &motionCapsuleIn,
							}).
							Children(func() {
								AnimatedLabel(ctx, entry.Label).FontSize(12.5).FontWeight(650).
									SingleLine().Shrink(0).
									TextColor(tab.AnimateColor("mnb-text", fg, colorAnim))
							})
					}
				})
				if capsuleGlass && tapPressed {
					snap := isStaticSnapshot(ctx)
					interaction := trackGlassInteraction(tab, snap)
					dx, dy := bottomNavLensExpand(slot.extent, tabHeight, interaction.elastic, 0, false)
					tapBox := expandGlass(ui.Rect{X: slot.start, Y: barPad, W: slot.extent, H: tabHeight}, dx, dy)
					tap := ui.Box(ctx.Key(mnbTapKey(index))).Absolute().Left(tapBox.X).Top(tapBox.Y).
						Width(tapBox.W).Height(tapBox.H).ZIndex(2).Radius(RadiusFull).PassThrough()
					navLensShadow(tap, p.Dark, interaction.glow, false)
					drawNavGlassLens(ctx, tap, 0, liquidLensSpec{
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
					drawNavGlassLens(ctx, lens.box(ctx.Key(selKey+"-warp"), RadiusFull, 1), 0, liquidLensSpec{
						Alpha: 1, Tint: veil, Zoom: 1 / lensZoom, ZoomY: barScaleY / lensZoom, WarpOnly: true,
					})
					spec.Zoom = standardZoom
					z = 3
				} else {
					spec.ZoomY = 0.86
				}
				drawNavGlassLens(ctx, lens.box(ctx.Key(selKey+"-lens"), RadiusFull, z), 0, spec)
			}
		})
	})
}

type mnbTapKey int

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

func (s *mnbDragState) applyDockGlass(ctx *ui.Context, bar ui.Element, p Palette, width, height float32, pressed, standard bool) {
	snap := isStaticSnapshot(ctx)
	var progress float32
	if !snap {
		var target float32
		if pressed {
			target = 1
		}
		sp := glassPressAnimation(pressed).Spring
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
	interaction := glassInteraction{
		elastic: clamp32(progress, -0.2, 1.25),
		glow:    glow,
		touch:   touch,
	}
	bar.Radius(RadiusFull).Material(dockGlass{
		glassInteraction: interaction,
		staticSnapshot:   snap,
	})
	if !p.Dark {
		if standard {
			g := clamp32(glow, 0, 1)
			bar.Shadow(0, 2+1.0*g, 11+2*g, 0, ui.RGBA(12, 18, 30, 0.052-0.014*g))
		} else if glow > 0.01 {
			bar.Shadow(0, 3+2*glow, 12+4*glow, 0, ui.RGBA(12, 18, 30, 0.06+0.04*glow))
		}
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
