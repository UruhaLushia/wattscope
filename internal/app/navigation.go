package app

import (
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

const (
	pageOverview = iota
	pagePD
	pageRecords
	pageDevice
	pageSettings
	pageMore
	pageConnectionLog
)

type navigationPosition uint8

const (
	navigationTop navigationPosition = iota
	navigationSide
	navigationBottom
)

type navHeaderTitleMatchKey struct{}
type headerTitleKey struct{ landscape bool }
type statusPillWrapKey struct{ landscape bool }

const (
	fullSidebarMinWidth    = 680
	mobileSidebarMinAvailW = 960
	navigationSidebarWidth = kit.DesktopSidebarWidth + kit.DesktopSidebarMargin
	compactSidebarWidth    = kit.DesktopCompactRailWidth + kit.DesktopSidebarMargin
	workspacePadding       = 16
	headerHeight           = 44
	mobileHeaderHeight     = 50
	headerLeading          = 16
	headerGap              = 8
	navTabPadding          = 10
	navLabelGap            = 7
	navTabGap              = 2
	navIconSize            = 15
	keyboardAtLeast        = 120
)

type workspacePage struct {
	label       string
	icon        *ui.SVG
	content     func()
	list        func(bottomPadding float32)
	fixed       func()
	overlay     func()
	headerExtra func()
}

func (d *dashboard) isLandscape(ctx *ui.Context) bool {
	if !d.mobile || ctx == nil {
		return false
	}
	w, h := ctx.Size()
	return w > h && w >= 560
}

func (d *dashboard) isPortraitMobile(ctx *ui.Context) bool {
	return d.mobile && !d.isLandscape(ctx)
}

func horizontalContentPadding(portraitMobile, landscape bool, position navigationPosition, padX, leftInset, rightInset float32) (start, end float32) {
	start, end = padX, padX
	switch {
	case portraitMobile:
		return leftInset + padX, rightInset + padX
	case landscape:
		if position != navigationSide {
			start = leftInset + padX
		}
		return start, rightInset + padX
	default:
		return start, end
	}
}

func navigationPlacement(mobile, landscape bool, preference string) navigationPosition {
	if mobile && !landscape {
		return navigationBottom
	}
	if preference == navigationSidebar {
		return navigationSide
	}
	return navigationTop
}

func (d *dashboard) sidebarWidthFor(ctx *ui.Context) float32 {
	width, winH := ctx.Size()
	if d.mobile {
		_, rightInset, _, leftInset := ctx.SafeArea()
		iconsOnly := width-leftInset-rightInset < mobileSidebarMinAvailW
		return kit.DesktopSidebarRailWidth(iconsOnly, winH) + kit.CompactSidebarSideMargin(winH)
	}
	return sidebarWidth(width)
}

func sidebarWidth(width float32) float32 {
	if width < fullSidebarMinWidth {
		return compactSidebarWidth
	}
	return navigationSidebarWidth
}

func isMobileSubpage(page int) bool {
	return page == pageDevice || page == pageConnectionLog || page == pageSettings
}

func pageScrollsUnderHeader(page workspacePage, pageID int, portraitMobile bool) bool {
	return page.fixed == nil && page.list == nil && (portraitMobile || (pageID != pageDevice && pageID != pageConnectionLog))
}

func (d *dashboard) renderTopScrollEdge(ctx *ui.Context, p kit.Palette, page workspacePage, portraitMobile bool, floatingTop float32) {
	if pageScrollsUnderHeader(page, d.page, portraitMobile) && d.scroll.Y > 0.5 {
		kit.GlassScrollEdge(ctx, p, 0, floatingTop+4, 8)
	}
}

func (d *dashboard) workspace(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pages []workspacePage, position navigationPosition, name, status string, tone ui.Color) {
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	landscape := d.isLandscape(ctx)
	portraitMobile := d.isPortraitMobile(ctx)
	keyboardOpen := d.mobile && bottomInset > keyboardAtLeast
	hasBottomBar := position == navigationBottom && !keyboardOpen

	if !portraitMobile && d.page == pageMore {
		d.page = pageOverview
		d.activePage.Store(int32(d.page))
	}

	if !d.snapshotOnly && portraitMobile && isMobileSubpage(d.page) {
		goBack := func() {
			if d.hasOpenMenu() {
				d.closeAllMenus()
				return
			}
			d.goPage(pageMore)
		}
		ctx.OnWindowShortcut(0, ui.KeyBack, goBack)
		ctx.OnWindowShortcut(ui.Alt, ui.KeyLeft, goBack)
	}

	var topPages []workspacePage
	if position == navigationTop {
		topPages = pages[:pageMore]
	}

	root := ui.Column(ctx).Fill()
	if !d.mobile {
		kit.CanvasBackdrop(root, p).Padding(topInset, rightInset, 0, leftInset)
	} else {
		kit.CanvasBackdrop(root, p)
	}
	navStyle := d.bottomBarStyle()
	navReserve := kit.BottomNavReserveFor(bottomInset, hasBottomBar, navStyle)
	activeHeaderHeight := float32(headerHeight)
	if d.mobile {
		activeHeaderHeight = float32(mobileHeaderHeight)
	}
	floatingTop := float32(0)
	if p.LiquidGlass {
		if d.mobile {
			floatingTop = activeHeaderHeight + topInset
		} else {
			floatingTop = activeHeaderHeight
		}
	}

	root.Children(func() {
		mobileTitle := pages[d.page].label
		if portraitMobile && d.page == pageDevice {
			mobileTitle = msgs.GuiDeviceInfo()
		}
		renderContent := func() {
			d.renderPageContent(ctx, p, msgs, pages, position, portraitMobile, landscape, hasBottomBar, floatingTop, navReserve, rightInset, bottomInset, leftInset)
		}
		if portraitMobile {
			d.renderPortraitWorkspace(ctx, p, msgs, pages, mobileTitle, name, status, tone, hasBottomBar, floatingTop, topInset, rightInset, bottomInset, leftInset, renderContent)
		} else {
			d.renderDesktopWorkspace(ctx, p, msgs, pages, topPages, position, landscape, mobileTitle, name, status, tone, floatingTop, bottomInset, leftInset, renderContent)
		}
	})

	if !d.snapshotOnly {
		if overlay := pages[d.page].overlay; overlay != nil {
			overlay()
		}

		toastBottom := float32(16) + bottomInset
		if hasBottomBar {
			toastBottom = kit.BottomNavReserveFor(bottomInset, true, navStyle) + 12
		}
		kit.ToastViewport(ctx, p, toastBottom, d.mobile || kit.NarrowWidth(ctx))
	}
}

func (d *dashboard) renderPageContent(
	ctx *ui.Context,
	p kit.Palette,
	msgs locales.Messages,
	pages []workspacePage,
	position navigationPosition,
	portraitMobile, landscape, hasBottomBar bool,
	floatingTop, navReserve, rightInset, bottomInset, leftInset float32,
) {
	page := pages[d.page]
	contentFloatingTop := floatingTop
	if !portraitMobile && (d.page == pageDevice || d.page == pageConnectionLog) {
		d.deviceSubnav(ctx, p, msgs, position, floatingTop, leftInset, rightInset)
		contentFloatingTop = 0
	}
	horizontalAnim := position == navigationTop || position == navigationBottom
	motionWrap := ui.Column(ctx.Key(d.page)).Fill().MinWidth(0).MinHeight(0)
	if !d.snapshotOnly {
		kit.NavigationPageMotion(motionWrap, horizontalAnim, d.pageDirection)
	}
	motionWrap.Children(func() {
		if page.list != nil || page.fixed != nil {
			basePadY, padX, gap := float32(16), float32(20), float32(12)
			switch {
			case portraitMobile:
				basePadY, padX, gap = 10, 14, 10
			case landscape && page.list != nil:
				basePadY, padX, gap = 4, 12, 6
			case landscape:
				basePadY, padX, gap = 8, 12, 10
			}
			padTop := basePadY + contentFloatingTop
			padBottom := basePadY
			innerBottom := float32(0)
			inStart, inEnd := horizontalContentPadding(portraitMobile, landscape, position, padX, leftInset, rightInset)
			switch {
			case portraitMobile:
				if hasBottomBar {
					padBottom, innerBottom = 0, navReserve+basePadY
				} else {
					padBottom = bottomInset + basePadY
				}
			case landscape:
				if page.fixed != nil {
					padBottom = bottomInset + 8
				} else {
					padBottom = max(6, bottomInset+4)
				}
			}
			container := ui.Column(ctx).Fill().MinWidth(0).MinHeight(0).
				Padding(padTop, inEnd, padBottom, inStart).
				Gap(gap)
			if page.fixed != nil {
				container.Children(page.fixed)
			} else {
				container.Children(func() { page.list(innerBottom) })
			}
			return
		}

		padY, padX, colGap := float32(16), float32(20), float32(16)
		switch {
		case portraitMobile:
			padY, padX, colGap = 12, 14, 12
		case landscape:
			padY, padX, colGap = 10, 12, 10
		}
		padBottom := padY + 6
		switch {
		case hasBottomBar:
			padBottom = navReserve + 12
		case portraitMobile:
			padBottom = bottomInset + padY + 6
		case landscape:
			padBottom = max(padY, bottomInset+8)
		}
		padTop := padY + contentFloatingTop
		inStart, inEnd := horizontalContentPadding(portraitMobile, landscape, position, padX, leftInset, rightInset)
		scroll := ui.Scroll(ctx.Key(d.page)).Scrollbars(false).Fill().MinWidth(0).MinHeight(0).TrackScroll(&d.scroll)
		if contentFloatingTop > 0 || navReserve > 0 {
			scroll.ScrollbarInsets(contentFloatingTop, 0, navReserve)
		}
		scroll.Children(func() {
			ui.Column(ctx).FillWidth().Shrink(0).
				Padding(padTop, inEnd, padBottom, inStart).
				Gap(colGap).Children(func() {
				kit.FormColumn(ctx, func() {
					ui.Column(ctx).FillWidth().Gap(colGap).Children(page.content)
				})
			})
		})
	})
}

func (d *dashboard) renderPortraitWorkspace(
	ctx *ui.Context,
	p kit.Palette,
	msgs locales.Messages,
	pages []workspacePage,
	mobileTitle, name, status string,
	tone ui.Color,
	hasBottomBar bool,
	floatingTop, topInset, rightInset, bottomInset, leftInset float32,
	pageContent func(),
) {
	if !p.LiquidGlass {
		d.mobileTopBar(ctx, p, msgs, mobileTitle, name, status, tone, topInset, rightInset, leftInset)
		if hasBottomBar {
			d.mobileBottomBar(ctx, p, msgs.Workspace(), pages, bottomInset, leftInset, rightInset)
		}
	}
	canvas := ui.Column(ctx.Key("mobile-body")).Grow(1).MinWidth(0).MinHeight(0).Clip()
	if !d.snapshotOnly {
		kit.ApplyRotateEnterPose(ctx, canvas.TransformOrigin(0.5, 0.35), "rot-mob", ui.Pose{Y: 12, Scale: 0.965, Fade: 0.55}, kit.MotionRotate)
	}
	canvas.Children(func() {
		pageContent()
		if isMobileSubpage(d.page) {
			rail := ui.Box(ctx.Key("edge-back")).Absolute().Left(leftInset).Top(floatingTop).Bottom(0).Width(20).ZIndex(8).
				Draggable(nil, ui.DraggableOptions{
					Axis:   ui.Horizontal,
					Bounds: &ui.Rect{},
					Rigid:  true,
				})
			px, _, _ := rail.PointerPosition()
			if _, _, held := rail.Dragged(); held && px >= 68 {
				d.goPage(pageMore)
			}
		}
	})
	if p.LiquidGlass {
		d.renderTopScrollEdge(ctx, p, pages[d.page], true, floatingTop)
		d.mobileTopBar(ctx, p, msgs, mobileTitle, name, status, tone, topInset, rightInset, leftInset)
		if hasBottomBar {
			d.mobileBottomBar(ctx, p, msgs.Workspace(), pages, bottomInset, leftInset, rightInset)
		}
	}
}

func (d *dashboard) renderDesktopWorkspace(
	ctx *ui.Context,
	p kit.Palette,
	msgs locales.Messages,
	pages, topPages []workspacePage,
	position navigationPosition,
	landscape bool,
	mobileTitle, name, status string,
	tone ui.Color,
	floatingTop, bottomInset, leftInset float32,
	pageContent func(),
) {
	var headerExtra func()
	if position == navigationSide {
		headerExtra = pages[d.page].headerExtra
	}
	if !p.LiquidGlass {
		d.desktopHeader(ctx, p, msgs, mobileTitle, name, status, tone, topPages, headerExtra)
	}
	body := ui.Row(ctx.Key("desktop-body")).Grow(1).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
	if d.mobile && !d.snapshotOnly {
		kit.ApplyRotateEnterPose(ctx, body.TransformOrigin(0.5, 0.35), "rot-desk", ui.Pose{Y: 10, Scale: 0.965, Fade: 0.55}, kit.MotionRotate)
	}
	body.Children(func() {
		if position == navigationSide {
			sideWrap := ui.Box(ctx).FillHeight().ZIndex(2)
			if p.LiquidGlass || landscape {
				var sideLeft, sideBottom float32
				if landscape {
					sideLeft, sideBottom = leftInset, bottomInset
				}
				sideWrap.Padding(floatingTop, 0, sideBottom, sideLeft)
			}
			sideWrap.Children(func() {
				d.pageNavigation(ctx, p, msgs, pages[:pageMore], navigationSide, false)
			})
		}
		desktopContent := ui.Column(ctx.Key("desktop-content")).Grow(1).MinWidth(0).MinHeight(0).FillHeight()
		if !d.snapshotOnly && !kit.IsOrientationRotating(ctx) {
			desktopContent.Transition(kit.SideContentTransition)
		}
		desktopContent.Children(pageContent)
	})
	if p.LiquidGlass {
		d.renderTopScrollEdge(ctx, p, pages[d.page], false, floatingTop)
		d.desktopHeader(ctx, p, msgs, mobileTitle, name, status, tone, topPages, headerExtra)
	}
}

func (d *dashboard) statusPillButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, name, status string, tone ui.Color, compactLabel, iconsOnly bool) ui.Element {
	fullStatus := name
	if status != "" {
		fullStatus = name + " · " + status
	}
	statusLabel := fullStatus
	if compactLabel && status != "" {
		statusLabel = status
	}
	pillHeight := d.headerLayoutMetrics(ctx).pillHeight
	var pill ui.Element
	buildPill := func() {
		pill = ui.ButtonBase(ctx.Key("status-pill")).Height(pillHeight).Shrink(0).
			Radius(kit.RadiusFull).AlignItems(ui.Center).
			Label(fullStatus).
			OnClick(func() {
				d.deviceMenuOpen = !d.deviceMenuOpen
				if d.deviceMenuOpen {
					d.scanMeters(msgs, true)
				}
			})
		if iconsOnly {
			pill.Size(pillHeight, pillHeight).Justify(ui.Center)
		} else {
			pill.Padding(0, 12).Gap(6)
		}
		if iconsOnly || statusLabel != fullStatus {
			pill.Tooltip(fullStatus)
		}
		pillBg := p.Inset
		if pill.Pressed() {
			pillBg = p.Press
		} else if pill.Hovered() || pill.FocusVisible() {
			pillBg = p.SurfaceHi
		}
		if !d.mobile {
			kit.ControlMotion(pill)
		}
		if p.LiquidGlass {
			kit.GlassFloatingButton(ctx, pill, p, kit.RadiusFull, ui.Color{}, pillBg, p.LineSoft)
		} else {
			pill.Background(pill.AnimateColor("pill-bg", pillBg, kit.MotionFast)).Border(1, p.LineSoft)
		}
		pill.Children(func() {
			kit.StatusDot(ctx, tone, 7)
			if !iconsOnly {
				kit.AnimatedLabel(ctx, statusLabel).FontSize(12).FontWeight(600).SingleLine().
					MaxWidth(170).TextColor(p.Text)
			}
		})
	}
	if d.mobile && !d.snapshotOnly {
		wrap := ui.Box(ctx.Key(statusPillWrapKey{landscape: d.isLandscape(ctx)})).Shrink(0).TransformOrigin(0.5, 0.5)
		kit.ApplyRotateEnterPose(ctx, wrap, "rot-pill", ui.Pose{X: 16, Scale: 0.93, Fade: 0.65}, kit.MotionRotate)
		wrap.Children(buildPill)
	} else {
		buildPill()
	}
	return pill
}

func floatingGlassIconButton(ctx *ui.Context, p kit.Palette, key string, size float32, icon *ui.SVG, iconSize float32, iconColor ui.Color, label string, onClick func(ui.Element)) ui.Element {
	var btn ui.Element
	btn = ui.ButtonBase(ctx.Key(key)).Size(size, size).Shrink(0).Center().
		Label(label).Tooltip(label).OnClick(func() {
		if onClick != nil {
			onClick(btn)
		}
	})
	kit.ControlMotion(kit.GlassFloatingButton(ctx, btn, p, kit.RadiusFull, ui.Color{}, p.Surface, p.LineSoft)).Children(func() {
		pop := float32(0)
		if btn.Pressed() {
			pop = 1
		}
		kit.GlassIconPop(ctx, ui.Icon(ctx, icon).FontSize(iconSize).TextColor(iconColor), p, pop)
	})
	return btn
}

func (d *dashboard) headerThemeButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, size float32) {
	themeIcon, themeLabel := kit.IconMoon, msgs.GuiDark()
	if p.Dark {
		themeIcon, themeLabel = kit.IconSun, msgs.GuiLight()
	}
	if p.LiquidGlass {
		iconSize := float32(15)
		if d.mobile {
			iconSize = 16
		}
		btn := floatingGlassIconButton(ctx, p, "theme-toggle", size, themeIcon, iconSize, p.Muted, themeLabel, d.toggleLightDark)
		if !d.mobile && !p.Dark && p.CardShadow.A > 0 {
			btn.Shadow(0, 1, 4, 0, p.CardShadow)
		}
		return
	}
	if d.mobile {
		var themeBtn ui.Element
		themeBtn = kit.RoundIconActionButton(ctx.Key("theme-toggle"), p, themeIcon, themeLabel, size, false, func() {
			d.toggleLightDark(themeBtn)
		})
		return
	}
	var themeBtn ui.Element
	themeBtn = kit.IconActionButton(ctx.Key("theme-toggle"), p, themeIcon, themeLabel, windowButtonWidth, false, func() {
		d.toggleLightDark(themeBtn)
	})
}

func (d *dashboard) mobileTopBar(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pageTitle, name, status string, tone ui.Color, topInset, rightInset, leftInset float32) {
	hasBack := isMobileSubpage(d.page)
	leading := leftInset + 16
	gap := float32(10)
	if hasBack {
		leading = leftInset + 8
		gap = 6
	}
	trailing := rightInset + 14
	header := ui.Row(ctx.Key("workspace-header")).Height(mobileHeaderHeight+topInset).Shrink(0).
		AlignItems(ui.Center).
		Padding(topInset, trailing, 0, leading).Gap(gap)
	if p.LiquidGlass {
		header.Absolute().Top(0).Left(0).Right(0).ZIndex(9)
	}
	kit.GlassBarChrome(header, p, p.Surface, p.LineSoft)
	var statusPill ui.Element
	header.Children(func() {
		if hasBack {
			if p.LiquidGlass {
				floatingGlassIconButton(ctx, p, "more-back", 38, kit.IconArrowLeft, 16, p.Text, msgs.GuiBack(), func(ui.Element) {
					d.goPage(pageMore)
				})
			} else {
				kit.RoundIconActionButton(ctx.Key("more-back"), p, kit.IconArrowLeft, msgs.GuiBack(), 40, false, func() {
					d.goPage(pageMore)
				})
			}
		}
		title := kit.AnimatedLabel(ctx.Key(headerTitleKey{landscape: false}), pageTitle).FontSize(19).FontWeight(750).SingleLine().Shrink(1).MinWidth(0).
			Role(ui.RoleHeading).Level(1)
		if !d.snapshotOnly {
			title.Transition(kit.NavRotateMatchTransition(ctx, navHeaderTitleMatchKey{}, false))
		}
		ui.Spacer(ctx).MinWidth(0)
		statusPill = d.statusPillButton(ctx, p, msgs, name, status, tone, kit.NarrowWidth(ctx), false)
	})
	if !d.snapshotOnly {
		d.deviceSwitcherPopover(ctx, p, msgs, statusPill)
	}
}

func (d *dashboard) desktopHeader(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pageTitle, name, status string, tone ui.Color, topPages []workspacePage, headerExtra func()) {
	top := len(topPages) != 0
	topInset, rightInset, _, leftInset := ctx.SafeArea()
	metrics := d.headerLayoutMetrics(ctx)
	plan := d.computeDesktopHeaderPlan(ctx, topPages, pageTitle, name, status, metrics)

	header := ui.Row(ctx.Key("workspace-header")).Shrink(0).AlignItems(ui.Center).Gap(headerGap)
	if !d.mobile {
		header.Height(headerHeight).Padding(0, metrics.padEnd, 0, metrics.padStart).DragWindow()
	} else {
		header.Height(mobileHeaderHeight+topInset).
			Padding(topInset, rightInset+metrics.padEnd, 0, leftInset+metrics.padStart)
	}
	if p.LiquidGlass {
		header.Absolute().Top(0).Left(0).Right(0).ZIndex(9)
	}
	kit.GlassBarChrome(header, p, p.Surface, p.LineSoft)

	var statusPill ui.Element
	header.Children(func() {
		if d.mobile {
			title := kit.AnimatedLabel(ctx.Key(headerTitleKey{landscape: true}), pageTitle).FontSize(19).FontWeight(750).SingleLine().Shrink(0).MinWidth(0).
				Role(ui.RoleHeading).Level(1)
			if !d.snapshotOnly {
				title.Transition(kit.NavRotateMatchTransition(ctx, navHeaderTitleMatchKey{}, false))
			}
		} else {
			ui.Row(ctx).AlignItems(ui.Center).Gap(7).Shrink(0).Children(func() {
				ui.Image(ctx, kit.BrandLogo).Size(22, 22).Shrink(0).AccessHidden(true)
				ui.Text(ctx, "WattScope").FontSize(13).FontWeight(750).SingleLine().Shrink(0).
					Role(ui.RoleHeading).Level(1)
			})
		}

		if top {
			d.pageNavigation(ctx, p, msgs, topPages, navigationTop, plan.navIconsOnly)
		} else if headerExtra != nil {
			ui.Box(ctx).Shrink(1).MinWidth(0).Children(headerExtra)
		}

		ui.Spacer(ctx).MinWidth(0)

		statusPill = d.statusPillButton(ctx, p, msgs, name, status, tone, plan.pillCompact, plan.pillIconsOnly)

		ctrlRow := ui.Row(ctx.Key("header-controls")).Shrink(0).AlignItems(ui.Center).Gap(windowButtonGap)
		if d.mobile && !d.snapshotOnly {
			kit.ApplyRotateEnterPose(ctx, ctrlRow.TransformOrigin(0.5, 0.5), "rot-theme", ui.Pose{Scale: 0.72, Fade: 0.85}, kit.MotionRotate)
		}
		ctrlRow.Children(func() {
			d.headerThemeButton(ctx, p, msgs, metrics.themeBtnSize)
			if !d.mobile {
				d.windowButtons(ctx, p, msgs)
			}
		})
	})
	if !d.snapshotOnly {
		d.deviceSwitcherPopover(ctx, p, msgs, statusPill)
	}
}

func (d *dashboard) toggleNavigationPlacement() {
	if d.settings.Navigation == navigationSidebar {
		d.settings.Navigation = navigationHorizontal
	} else {
		d.settings.Navigation = navigationSidebar
	}
	d.saveSettings()
}

func (d *dashboard) pageNavigation(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pages []workspacePage, position navigationPosition, iconsOnly bool) {
	var itemBuf [pageMore]kit.NavTab
	items := itemBuf[:0]
	if len(pages) > len(itemBuf) {
		items = make([]kit.NavTab, 0, len(pages))
	}
	for index, page := range pages {
		idx := index
		items = append(items, kit.NavTab{
			Label:    page.label,
			Icon:     page.icon,
			OnSelect: func() { d.goPage(idx) },
		})
	}
	sidebar := position == navigationSide
	tabPad, tabGap := float32(navTabPadding), float32(navLabelGap)
	toggleTooltip := msgs.GuiSideNavigation()
	if sidebar {
		iconsOnly = d.sidebarWidthFor(ctx) < kit.DesktopSidebarWidth
		tabPad, tabGap = 12, 10
		toggleTooltip = msgs.GuiTopNavigation()
	}
	selected := d.page
	if d.page == pageConnectionLog {
		selected = pageDevice
	}
	before := selected
	kit.DesktopNav(ctx, p, msgs.Workspace(), &selected, items, kit.DesktopNavOptions{
		Sidebar:       sidebar,
		IconsOnly:     iconsOnly,
		Touch:         d.mobile,
		TabPad:        tabPad,
		TabGap:        tabGap,
		OnToggle:      d.toggleNavigationPlacement,
		ToggleLabel:   msgs.GuiNavigation(),
		ToggleTooltip: toggleTooltip,
	})
	if selected != before {
		d.goPage(selected)
	}
}

func (d *dashboard) mobileBottomBar(ctx *ui.Context, p kit.Palette, label string, pages []workspacePage, bottomInset, leftInset, rightInset float32) {
	destinations := [...]int{pageOverview, pagePD, pageRecords, pageMore}
	var items [len(destinations)]kit.NavTab
	selected := len(destinations) - 1
	for index, destination := range destinations {
		dest := destination
		page := pages[dest]
		items[index] = kit.NavTab{
			Label: page.label, Icon: page.icon, OnSelect: func() { d.goPage(dest) },
		}
		if d.page == dest {
			selected = index
		}
	}
	before := selected
	kit.BottomNavigationBar(ctx, p, "mobile-pages", label, &selected, items[:], kit.BottomNavigationBarOptions{
		Style:       d.bottomBarStyle(),
		BottomInset: bottomInset,
		LeftInset:   leftInset,
		RightInset:  rightInset,
	})
	if selected != before {
		d.goPage(destinations[selected])
	}
}

func (d *dashboard) morePage(ctx *ui.Context, p kit.Palette, msgs locales.Messages) {
	kit.NarrowColumn(ctx, func() {
		kit.Card(ctx, p).Padding(5).Gap(2).Children(func() {
			for index, entry := range []struct {
				page  int
				label string
				icon  *ui.SVG
			}{
				{pageDevice, msgs.GuiDeviceInfo(), kit.IconCpu},
				{pageConnectionLog, msgs.GuiConnectionLog(), kit.IconActivity},
				{pageSettings, msgs.GuiSettings(), kit.IconSliders},
			} {
				if index > 0 {
					ui.Box(ctx).FillWidth().Padding(0, 12).Children(func() {
						kit.Divider(ctx, p)
					})
				}
				row := ui.ButtonBase(ctx.Key(entry.page)).FillWidth().MinHeight(48).Padding(0, 12).Gap(10).
					Radius(kit.RadiusM).AlignItems(ui.Center).Label(entry.label).OnClick(func() { d.goPage(entry.page) })
				bg := ui.Transparent
				if row.Pressed() {
					bg = p.Press
				} else if row.Hovered() || row.FocusVisible() {
					bg = p.Hover
				}
				kit.ControlMotion(row).Children(func() {
					kit.GlassPillControl(ctx.Key("bg"), row, p, kit.RadiusM, ui.Color{}, false, bg, 0, ui.Transparent, 0.985)
					ui.Icon(ctx, entry.icon).FontSize(17).TextColor(p.Muted).Shrink(0)
					kit.AnimatedLabel(ctx, entry.label).FontSize(14).FontWeight(550).Grow(1)
					ui.Icon(ctx, kit.IconChevronR).FontSize(15).TextColor(p.Muted).Shrink(0)
				})
			}
		})
	})
}

type desktopHeaderMetrics struct {
	padStart     float32
	padEnd       float32
	pillHeight   float32
	themeBtnSize float32
	tabsMargin   float32
}

func (d *dashboard) headerLayoutMetrics(ctx *ui.Context) desktopHeaderMetrics {
	if !d.mobile {
		return desktopHeaderMetrics{
			padStart:     headerLeading,
			padEnd:       workspacePadding,
			pillHeight:   kit.DesktopTopNavHeight,
			themeBtnSize: kit.DesktopTopNavHeight,
			tabsMargin:   6,
		}
	}
	_, winH := ctx.Size()
	padStart := float32(headerLeading)
	if navigationPlacement(d.mobile, true, d.settings.Navigation) == navigationSide {
		padStart = kit.CompactSidebarSideMargin(winH)
	}
	return desktopHeaderMetrics{
		padStart:     padStart,
		padEnd:       14,
		pillHeight:   36,
		themeBtnSize: 36,
		tabsMargin:   4,
	}
}

type desktopHeaderPlan struct {
	navIconsOnly  bool
	pillCompact   bool
	pillIconsOnly bool
}

type desktopHeaderPlanKey struct {
	mobile        bool
	landscape     bool
	availableW    float32
	leftInset     float32
	rightInset    float32
	topPagesCount int
	locale        string
	pageTitle     string
	name          string
	status        string
	metrics       desktopHeaderMetrics
}

type desktopHeaderPlanCache struct {
	key  desktopHeaderPlanKey
	plan desktopHeaderPlan
}

func measureStatusPillWidth(ctx *ui.Context, text string, pillHeight float32) float32 {
	if text == "" {
		return pillHeight
	}
	w, _ := ctx.MeasureText(1<<20, ui.Span{Text: text, Size: 12, Weight: 600})
	return min(170, w) + 37
}

func measureTopTabsWidth(ctx *ui.Context, pages []workspacePage, tabPad, tabGap, ctrlH float32) float32 {
	if len(pages) == 0 {
		return 0
	}
	// Outer capsule horizontal padding (6), leading toggle (ctrlH), inter-item gaps.
	width := float32(6+ctrlH) + navTabGap*float32(len(pages))
	for _, page := range pages {
		labelW, _ := ctx.MeasureText(1<<20, ui.Span{Text: page.label, Size: 13, Weight: 550})
		width += navIconSize + tabGap + 2*tabPad + labelW
	}
	return width
}

func (d *dashboard) computeDesktopHeaderPlan(ctx *ui.Context, pages []workspacePage, pageTitle, name, status string, metrics desktopHeaderMetrics) desktopHeaderPlan {
	available, _ := ctx.Size()
	landscape := d.isLandscape(ctx)
	var leftInset, rightInset float32
	if d.mobile {
		_, rightInset, _, leftInset = ctx.SafeArea()
	}
	key := desktopHeaderPlanKey{
		mobile:        d.mobile,
		landscape:     landscape,
		availableW:    available,
		leftInset:     leftInset,
		rightInset:    rightInset,
		topPagesCount: len(pages),
		locale:        d.locale(),
		pageTitle:     pageTitle,
		name:          name,
		status:        status,
		metrics:       metrics,
	}
	if d.headerPlanCache.key == key {
		return d.headerPlanCache.plan
	}

	var brandW float32
	rightW := float32(windowButtonWidth)
	if d.mobile {
		titleW, _ := ctx.MeasureText(1<<20, ui.Span{Text: pageTitle, Size: 19, Weight: 750})
		brandW = titleW
		rightW = metrics.themeBtnSize
		available -= leftInset + rightInset
	} else {
		titleW, _ := ctx.MeasureText(1<<20, ui.Span{Text: "WattScope", Size: 13, Weight: 750})
		brandW = 22 + 7 + titleW
		rightW += windowButtonGap + windowButtonsWidth
	}
	fullText := name
	if status != "" {
		fullText = name + " · " + status
	}
	fullPillW := measureStatusPillWidth(ctx, fullText, metrics.pillHeight)
	shortPillW := fullPillW
	if status != "" {
		shortPillW = measureStatusPillWidth(ctx, status, metrics.pillHeight)
	}
	dotPillW := metrics.pillHeight

	gapCount := float32(3)
	if len(pages) == 0 {
		gapCount = 2
	}
	fixed := metrics.padStart + metrics.padEnd + gapCount*headerGap + brandW + rightW

	var plan desktopHeaderPlan
	if len(pages) == 0 {
		switch {
		case available >= fixed+fullPillW:
		case status != "" && available >= fixed+shortPillW:
			plan = desktopHeaderPlan{pillCompact: true}
		default:
			plan = desktopHeaderPlan{pillIconsOnly: true}
		}
		d.headerPlanCache = desktopHeaderPlanCache{key: key, plan: plan}
		return plan
	}

	topCtrlH := kit.DesktopNavTopControlHeight(d.mobile)
	tabsFullW := measureTopTabsWidth(ctx, pages, navTabPadding, navLabelGap, topCtrlH)
	tabsIconW := float32(6+topCtrlH) + float32(len(pages))*topCtrlH + navTabGap*float32(len(pages))

	switch {
	case available >= fixed+metrics.tabsMargin+tabsFullW+fullPillW:
		plan = desktopHeaderPlan{}
	case status != "" && available >= fixed+metrics.tabsMargin+tabsFullW+shortPillW:
		plan = desktopHeaderPlan{pillCompact: true}
	case available >= fixed+metrics.tabsMargin+tabsFullW+dotPillW:
		plan = desktopHeaderPlan{pillIconsOnly: true}
	case available >= fixed+metrics.tabsMargin+tabsIconW+fullPillW:
		plan = desktopHeaderPlan{navIconsOnly: true}
	case status != "" && available >= fixed+metrics.tabsMargin+tabsIconW+shortPillW:
		plan = desktopHeaderPlan{navIconsOnly: true, pillCompact: true}
	default:
		plan = desktopHeaderPlan{navIconsOnly: true, pillIconsOnly: true}
	}
	d.headerPlanCache = desktopHeaderPlanCache{key: key, plan: plan}
	return plan
}
