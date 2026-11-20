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

const (
	fullSidebarMinWidth    = 680
	navigationSidebarWidth = kit.DesktopSidebarWidth + kit.DesktopSidebarMargin
	compactSidebarWidth    = kit.DesktopCompactRailWidth + kit.DesktopSidebarMargin
	workspacePadding       = 16
	headerHeight           = 44
	mobileHeaderHeight     = 50
	bottomBarHeight        = kit.BottomNavHeight
	headerLeading          = 16
	headerGap              = 8
	topNavHeight           = 26
	navTabPadding          = 10
	navLabelGap            = 7
	navTabGap              = 2
	navIconSize            = 15
	keyboardAtLeast        = 120
)

type workspacePage struct {
	label   string
	icon    *ui.SVG
	content func()
	list    func(bottomPadding float32)
	overlay func()
}

func navigationPlacement(mobile bool, preference string) navigationPosition {
	if mobile {
		return navigationBottom
	}
	if preference == navigationSidebar {
		return navigationSide
	}
	return navigationTop
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

func (d *dashboard) workspace(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pages []workspacePage, position navigationPosition, name, status string, tone ui.Color) {
	topInset, rightInset, bottomInset, leftInset := ctx.SafeArea()
	keyboardOpen := d.mobile && bottomInset > keyboardAtLeast
	hasBottomBar := position == navigationBottom && !keyboardOpen

	if !d.snapshotOnly && d.mobile && isMobileSubpage(d.page) {
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

	horizontalAnim := position == navigationTop || position == navigationBottom

	root := ui.Column(ctx).Fill()
	if !d.mobile {
		kit.CanvasBackdrop(root, p).Padding(topInset, rightInset, 0, leftInset)
	}
	navStyle := d.bottomBarStyle()
	navReserve := kit.BottomNavReserveFor(bottomInset, hasBottomBar, navStyle)
	floatingTop := float32(0)
	if p.LiquidGlass {
		if d.mobile {
			floatingTop = mobileHeaderHeight + topInset
		} else {
			floatingTop = headerHeight
		}
	}

	root.Children(func() {
		mobileTitle := pages[d.page].label
		if d.page == pageDevice {
			mobileTitle = msgs.GuiDeviceInfo()
		}

		pageContent := func() {
			page := pages[d.page]
			contentFloatingTop := floatingTop
			if !d.mobile && (d.page == pageDevice || d.page == pageConnectionLog) {
				d.deviceSubnav(ctx, p, msgs, floatingTop)
				contentFloatingTop = 0
			}
			motionWrap := ui.Column(ctx.Key(d.page)).Fill().MinWidth(0).MinHeight(0)
			if !d.snapshotOnly {
				kit.NavigationPageMotion(motionWrap, horizontalAnim, d.pageDirection)
			}
			motionWrap.Children(func() {
				if page.list != nil {
					padY, padX, gap := float32(16), float32(20), float32(12)
					if d.mobile {
						padY, padX, gap = 10, 14, 10
					}
					padY += contentFloatingTop
					padBottom := padY
					listBottom := float32(0)
					inStart, inEnd := padX, padX
					if d.mobile {
						inStart, inEnd = max(padX, leftInset), max(padX, rightInset)
						if hasBottomBar {
							padBottom, listBottom = 0, navReserve+padY-contentFloatingTop
						}
					}
					ui.Column(ctx).Fill().MinWidth(0).MinHeight(0).
						Padding(padY, inEnd, padBottom, inStart).
						Gap(gap).Children(func() { page.list(listBottom) })
					return
				}

				padY, padX, colGap := float32(16), float32(20), float32(16)
				if d.mobile {
					padY, padX, colGap = 12, 14, 12
				}
				padBottom := padY + 6
				if hasBottomBar {
					padBottom = navReserve + 12
				}
				padY += contentFloatingTop
				inStart, inEnd := padX, padX
				if d.mobile {
					inStart, inEnd = max(padX, leftInset), max(padX, rightInset)
				}
				scroll := ui.Scroll(ctx.Key(d.page)).Scrollbars(false).Fill().MinWidth(0).MinHeight(0).TrackScroll(&d.scroll)
				if contentFloatingTop > 0 || navReserve > 0 {
					scroll.ScrollbarInsets(contentFloatingTop, 0, navReserve)
				}
				scroll.Children(func() {
					ui.Column(ctx).FillWidth().Shrink(0).
						Padding(padY, inEnd, padBottom, inStart).
						Gap(colGap).Children(func() {
						kit.FormColumn(ctx, func() {
							ui.Column(ctx).FillWidth().Gap(colGap).Children(page.content)
						})
					})
				})
			})
		}

		if d.mobile {
			bottomCanvasMargin := float32(0)
			if hasBottomBar {
				bottomCanvasMargin = -bottomInset
			}
			topCanvasMargin := float32(0)
			if p.LiquidGlass {
				topCanvasMargin = -topInset
			}
			if !p.LiquidGlass {
				d.mobileTopBar(ctx, p, msgs, mobileTitle, name, status, tone, topInset, rightInset, leftInset)
				if hasBottomBar {
					d.mobileBottomBar(ctx, p, msgs.Workspace(), pages, bottomInset, leftInset, rightInset)
				}
			}
			canvas := ui.Column(ctx.Key("mobile-body")).FillWidth().Grow(1).MinHeight(0).Clip().
				Margin(topCanvasMargin, -rightInset, bottomCanvasMargin, -leftInset)
			canvas.Children(func() {
				pageContent()
				if isMobileSubpage(d.page) {
					rail := ui.Box(ctx.Key("edge-back")).Absolute().Left(0).Top(floatingTop).Bottom(0).Width(20).ZIndex(8).
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
				kit.GlassScrollEdge(ctx, p, false, -topInset, mobileHeaderHeight+topInset+10, 8)
				if hasBottomBar {
					kit.GlassScrollEdge(ctx, p, true, -bottomInset, navReserve+10, 8)
				}
				d.mobileTopBar(ctx, p, msgs, mobileTitle, name, status, tone, topInset, rightInset, leftInset)
				if hasBottomBar {
					d.mobileBottomBar(ctx, p, msgs.Workspace(), pages, bottomInset, leftInset, rightInset)
				}
			}
		} else {
			if !p.LiquidGlass {
				d.desktopHeader(ctx, p, msgs, name, status, tone, topPages)
			}
			ui.Row(ctx.Key("desktop-body")).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				if position == navigationSide {
					if p.LiquidGlass {
						ui.Box(ctx).FillHeight().Padding(headerHeight, 0, 0, 0).Children(func() {
							d.pageNavigation(ctx, p, msgs, pages[:pageMore], navigationSide, false, navTabPadding, navLabelGap)
						})
					} else {
						d.pageNavigation(ctx, p, msgs, pages[:pageMore], navigationSide, false, navTabPadding, navLabelGap)
					}
				}
				desktopContent := kit.CanvasBackdrop(ui.Column(ctx.Key("desktop-content")).Grow(1).MinWidth(0).MinHeight(0).FillHeight(), p)
				if !d.snapshotOnly {
					desktopContent.Transition(kit.SideContentTransition)
				}
				desktopContent.Children(pageContent)
			})
			if p.LiquidGlass {
				kit.GlassScrollEdge(ctx, p, false, 0, headerHeight+10, 8)
				d.desktopHeader(ctx, p, msgs, name, status, tone, topPages)
			}
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

func (d *dashboard) statusPillButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, name, status string, tone ui.Color, compactLabel, iconsOnly bool) ui.Element {
	fullStatus := name
	if status != "" {
		fullStatus = name + " · " + status
	}
	statusLabel := fullStatus
	if compactLabel && status != "" {
		statusLabel = status
	}
	pillHeight := kit.DesktopTopNavHeight
	if d.mobile {
		pillHeight = 36
	}
	pill := ui.ButtonBase(ctx.Key("status-pill")).Height(pillHeight).Shrink(0).
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
	kit.ControlMotion(pill)
	if p.LiquidGlass {
		if !p.Dark && p.CardShadow.A > 0 {
			pill.Shadow(0, 2, 8, 0, p.CardShadow)
		}
		kit.GlassFloatingButton(pill, p, kit.RadiusFull, ui.Color{}, pillBg, p.LineSoft)
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
	return pill
}

func (d *dashboard) mobileTopBar(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pageTitle, name, status string, tone ui.Color, topInset, rightInset, leftInset float32) {
	hasBack := isMobileSubpage(d.page)
	leading := max(16, leftInset)
	gap := float32(10)
	if hasBack {
		leading = max(8, leftInset)
		gap = 6
	}
	trailing := max(14, rightInset)
	header := ui.Row(ctx.Key("workspace-header")).Height(mobileHeaderHeight+topInset).Shrink(0).
		AlignItems(ui.Center).
		Padding(topInset, trailing, 0, leading).Gap(gap)
	if p.LiquidGlass {
		header.Absolute().Top(-topInset).Left(-leftInset).Right(-rightInset).ZIndex(9)
	} else {
		header.FillWidth().Margin(-topInset, -rightInset, 0, -leftInset)
	}
	kit.GlassBarChrome(header, p, p.Surface, p.LineSoft)
	var statusPill ui.Element
	header.Children(func() {
		if hasBack {
			if p.LiquidGlass {
				back := ui.ButtonBase(ctx.Key("more-back")).Size(38, 38).Shrink(0).Center().
					Label(msgs.GuiBack()).Tooltip(msgs.GuiBack()).OnClick(func() {
					d.goPage(pageMore)
				})
				if !p.Dark && p.CardShadow.A > 0 {
					back.Shadow(0, 2, 8, 0, p.CardShadow)
				}
				kit.ControlMotion(kit.GlassFloatingButton(back, p, kit.RadiusFull, ui.Color{}, p.Surface, p.LineSoft)).Children(func() {
					pop := float32(0)
					if back.Pressed() {
						pop = 1
					}
					kit.GlassIconPop(ui.Icon(ctx, kit.IconArrowLeft).FontSize(16).TextColor(p.Text), p, pop)
				})
			} else {
				kit.IconActionButton(ctx.Key("more-back"), p, kit.IconArrowLeft, msgs.GuiBack(), 40, false, func() {
					d.goPage(pageMore)
				})
			}
		}
		kit.AnimatedLabel(ctx, pageTitle).FontSize(19).FontWeight(750).SingleLine().Grow(1).MinWidth(0).
			Role(ui.RoleHeading).Level(1)
		statusPill = d.statusPillButton(ctx, p, msgs, name, status, tone, kit.NarrowWidth(ctx), false)
	})
	if !d.snapshotOnly {
		d.deviceSwitcherPopover(ctx, p, msgs, statusPill)
	}
}

func (d *dashboard) desktopHeader(ctx *ui.Context, p kit.Palette, msgs locales.Messages, name, status string, tone ui.Color, topPages []workspacePage) {
	top := len(topPages) != 0
	plan := computeDesktopHeaderPlan(ctx, topPages, name, status)
	themeIcon, themeLabel := kit.IconMoon, msgs.GuiDark()
	if p.Dark {
		themeIcon, themeLabel = kit.IconSun, msgs.GuiLight()
	}

	header := ui.Row(ctx.Key("workspace-header")).Height(headerHeight).Shrink(0).AlignItems(ui.Center).
		Padding(0, workspacePadding, 0, headerLeading).Gap(headerGap).
		DragWindow()
	if p.LiquidGlass {
		header.Absolute().Top(0).Left(0).Right(0).ZIndex(9)
	} else {
		header.FillWidth()
	}
	kit.GlassBarChrome(header, p, p.Surface, p.LineSoft)

	var statusPill ui.Element
	header.Children(func() {
		ui.Row(ctx).AlignItems(ui.Center).Gap(7).Shrink(0).Children(func() {
			ui.Image(ctx, kit.BrandLogo).Size(22, 22).Shrink(0).AccessHidden(true)
			ui.Text(ctx, "WattScope").FontSize(13).FontWeight(750).SingleLine().Shrink(0).
				Role(ui.RoleHeading).Level(1)
		})

		if top {
			d.pageNavigation(ctx, p, msgs, topPages, navigationTop, plan.navIconsOnly, plan.tabPad, plan.tabGap)
		}

		ui.Spacer(ctx).MinWidth(0)

		statusPill = d.statusPillButton(ctx, p, msgs, name, status, tone, plan.pillCompact, plan.pillIconsOnly)

		ui.Row(ctx).Shrink(0).AlignItems(ui.Center).Gap(windowButtonGap).Children(func() {
			if p.LiquidGlass {
				var themeBtn ui.Element
				themeBtn = ui.ButtonBase(ctx.Key("theme-toggle")).Size(kit.DesktopTopNavHeight, kit.DesktopTopNavHeight).Shrink(0).Center().
					Label(themeLabel).Tooltip(themeLabel).OnClick(func() {
					d.toggleLightDark(themeBtn)
				})
				if !p.Dark && p.CardShadow.A > 0 {
					themeBtn.Shadow(0, 1, 4, 0, p.CardShadow)
				}
				kit.ControlMotion(kit.GlassFloatingButton(themeBtn, p, kit.RadiusFull, ui.Color{}, p.Surface, p.LineSoft)).Children(func() {
					pop := float32(0)
					if themeBtn.Pressed() {
						pop = 1
					}
					kit.GlassIconPop(ui.Icon(ctx, themeIcon).FontSize(15).TextColor(p.Muted), p, pop)
				})
			} else {
				var themeBtn ui.Element
				themeBtn = kit.IconActionButton(ctx.Key("theme-toggle"), p, themeIcon, themeLabel, windowButtonWidth, false, func() {
					d.toggleLightDark(themeBtn)
				})
			}
			d.windowButtons(ctx, p, msgs)
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

func (d *dashboard) pageNavigation(ctx *ui.Context, p kit.Palette, msgs locales.Messages, pages []workspacePage, position navigationPosition, iconsOnly bool, tabPad, tabGap float32) {
	items := make([]kit.NavTab, len(pages))
	for index, page := range pages {
		idx := index
		items[idx] = kit.NavTab{
			Label:    page.label,
			Icon:     page.icon,
			OnSelect: func() { d.goPage(idx) },
		}
	}
	sidebar := position == navigationSide
	toggleTooltip := msgs.GuiSideNavigation()
	if sidebar {
		width, _ := ctx.Size()
		iconsOnly = sidebarWidth(width) != navigationSidebarWidth
		tabPad, tabGap = 12, 10
		toggleTooltip = msgs.GuiTopNavigation()
	}
	selected := d.page
	if d.page == pageConnectionLog {
		selected = pageDevice
	}
	before := selected
	kit.DesktopNav(ctx, p, msgs.Workspace(), &selected, items, sidebar, iconsOnly, tabPad, tabGap, d.toggleNavigationPlacement, msgs.GuiNavigation(), toggleTooltip)
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
	kit.BottomNavigationBar(ctx, p, "mobile-pages", label, &selected, items[:], d.bottomBarStyle(), bottomInset, leftInset, rightInset)
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

type desktopHeaderPlan struct {
	navIconsOnly  bool
	pillCompact   bool
	pillIconsOnly bool
	tabPad        float32
	tabGap        float32
}

func measureStatusPillWidth(ctx *ui.Context, text string) float32 {
	if text == "" {
		return 28
	}
	w, _ := ctx.MeasureText(1<<20, ui.Span{Text: text, Size: 12, Weight: 600})
	return min(170, w) + 33
}

func measureTopTabsWidth(ctx *ui.Context, pages []workspacePage, tabPad, tabGap float32) float32 {
	if len(pages) == 0 {
		return 0
	}
	// Outer capsule horizontal padding (6), leading toggle (26), inter-item gaps.
	width := float32(6+26) + navTabGap*float32(len(pages))
	for _, page := range pages {
		labelW, _ := ctx.MeasureText(1<<20, ui.Span{Text: page.label, Size: 13, Weight: 550})
		width += navIconSize + tabGap + 2*tabPad + labelW
	}
	return width
}

func computeDesktopHeaderPlan(ctx *ui.Context, pages []workspacePage, name, status string) desktopHeaderPlan {
	available, _ := ctx.Size()
	titleW, _ := ctx.MeasureText(1<<20, ui.Span{Text: "WattScope", Size: 13, Weight: 750})
	fullText := name
	if status != "" {
		fullText = name + " · " + status
	}
	fullPillW := measureStatusPillWidth(ctx, fullText)
	shortPillW := fullPillW
	if status != "" {
		shortPillW = measureStatusPillWidth(ctx, status)
	}
	const dotPillW = float32(28)

	rightW := float32(windowButtonWidth + windowButtonGap + windowButtonsWidth)
	brandW := 22 + 7 + titleW
	fixed := float32(headerLeading+workspacePadding+4*headerGap) + brandW + rightW

	if len(pages) == 0 {
		if available >= fixed+fullPillW {
			return desktopHeaderPlan{}
		}
		if status != "" && available >= fixed+shortPillW {
			return desktopHeaderPlan{pillCompact: true}
		}
		return desktopHeaderPlan{pillIconsOnly: available < fixed+dotPillW}
	}

	tabsFullW := measureTopTabsWidth(ctx, pages, navTabPadding, navLabelGap)
	tabsIconW := float32(6+26) + float32(len(pages))*26 + navTabGap*float32(len(pages))

	switch {
	case available >= fixed+6+tabsFullW+fullPillW:
		return desktopHeaderPlan{tabPad: navTabPadding, tabGap: navLabelGap}
	case status != "" && available >= fixed+6+tabsFullW+shortPillW:
		return desktopHeaderPlan{pillCompact: true, tabPad: navTabPadding, tabGap: navLabelGap}
	case available >= fixed+6+tabsFullW+dotPillW:
		return desktopHeaderPlan{pillIconsOnly: true, tabPad: navTabPadding, tabGap: navLabelGap}
	case available >= fixed+tabsIconW+fullPillW:
		return desktopHeaderPlan{navIconsOnly: true, tabPad: navTabPadding, tabGap: navLabelGap}
	case status != "" && available >= fixed+tabsIconW+shortPillW:
		return desktopHeaderPlan{navIconsOnly: true, pillCompact: true, tabPad: navTabPadding, tabGap: navLabelGap}
	default:
		return desktopHeaderPlan{navIconsOnly: true, pillIconsOnly: true, tabPad: navTabPadding, tabGap: navLabelGap}
	}
}
