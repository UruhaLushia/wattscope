package app

import (
	"slices"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/preferences"
	"github.com/egoist/mygo/ui"
)

const (
	settingsKey          = "gui"
	settingsAuto         = "auto"
	navigationHorizontal = "horizontal"
	navigationSidebar    = "sidebar"
	bottomBarCapsule     = "capsule"
	bottomBarStandard    = "standard"
	islandLeftVoltage    = "voltage"
	islandLeftCurrent    = "current"
	islandLeftEnergy     = "energy"
	islandLeftCharge     = "charge"
	islandLeftOff        = "off"
)

type guiSettings struct {
	Theme          mygo.ThemeSource `json:"theme"`
	Accent         string           `json:"accent"`
	Language       string           `json:"language"`
	Navigation     string           `json:"navigation"`
	BottomBarStyle string           `json:"bottom_bar_style"`
	SmoothChart    bool             `json:"smooth_chart"`
	LiquidGlass    bool             `json:"liquid_glass"`
	IslandEnabled  bool             `json:"island_enabled"`
	IslandLeft     string           `json:"island_left,omitempty"`
}

type islandDisplay struct {
	enabled bool
	left    string
}

func (d *dashboard) syncIslandDisplay() {
	d.islandDisplay.Store(&islandDisplay{enabled: d.settings.IslandEnabled, left: d.settings.IslandLeft})
}

type themeChange struct {
	source mygo.ThemeSource
	origin kit.ThemeRevealOrigin
	toggle bool
}

func (d *dashboard) loadSettings(language string) {
	d.settings.SmoothChart = true
	d.settings.IslandEnabled = true
	preferences.Get(settingsKey, &d.settings)
	switch d.settings.Theme {
	case mygo.ThemeLight, mygo.ThemeDark:
	default:
		d.settings.Theme = mygo.ThemeSystem
	}
	if !slices.Contains(kit.AccentPresets, d.settings.Accent) {
		d.settings.Accent = kit.DefaultAccentHex
	}
	if !slices.Contains(locales.Catalog.Languages(), d.settings.Language) {
		d.settings.Language = settingsAuto
	}
	switch d.settings.Navigation {
	case navigationHorizontal, navigationSidebar:
	default:
		d.settings.Navigation = navigationSidebar
		if d.mobile {
			d.settings.Navigation = navigationHorizontal
		}
	}
	switch d.settings.BottomBarStyle {
	case bottomBarCapsule, bottomBarStandard:
	default:
		d.settings.BottomBarStyle = bottomBarCapsule
	}
	switch d.settings.IslandLeft {
	case islandLeftVoltage, islandLeftCurrent, islandLeftEnergy, islandLeftCharge, islandLeftOff:
	default:
		d.settings.IslandLeft = islandLeftVoltage
	}
	d.syncIslandDisplay()
	d.language = d.settings.Language
	if language = strings.TrimSpace(language); language != "" && !strings.EqualFold(language, settingsAuto) {
		d.language = locales.Resolve(language)
	}
	if d.mobile {
		d.notifyLanguage.Store(d.locale())
	}
}

func (d *dashboard) bottomBarStyle() kit.BottomNavStyle {
	if d.settings.BottomBarStyle == bottomBarStandard {
		return kit.BottomNavStandard
	}
	return kit.BottomNavCapsule
}

func (d *dashboard) saveSettings() {
	if err := preferences.Set(settingsKey, d.settings); err != nil {
		msgs := locales.In(d.locale())
		d.notify(msgs.GuiSettingsSaveFailed(err.Error()), "", "error")
	}
}

func (d *dashboard) setTheme(value string) {
	d.themeChanges = append(d.themeChanges, themeChange{
		source: mygo.ThemeSource(value), origin: kit.TakeThemeRevealOrigin(),
	})
}

func (d *dashboard) toggleLightDark(trigger ui.Element) {
	d.themeChanges = append(d.themeChanges, themeChange{
		toggle: true, origin: kit.CaptureThemeRevealOrigin(trigger),
	})
}

func (d *dashboard) applyThemeChanges(ctx *ui.Context) {
	original := d.settings.Theme
	for _, change := range d.themeChanges {
		fromDark := d.themeIsDark(ctx)
		source := change.source
		if change.toggle {
			source = mygo.ThemeDark
			if fromDark {
				source = mygo.ThemeLight
			}
		}
		if source == d.settings.Theme {
			continue
		}
		d.settings.Theme = source
		if d.window != nil {
			mygo.Theme.SetSource(source)
			if source == mygo.ThemeSystem {
				theme, _ := kit.ResolveTheme(mygo.Theme.IsDark(), d.settings.Accent)
				ctx.SetTheme(theme)
			}
		}
		if dark := d.themeIsDark(ctx); dark != fromDark {
			d.themeReveal.Start(ctx, dark, change.origin)
		}
	}
	d.themeChanges = d.themeChanges[:0]
	if d.settings.Theme != original {
		d.saveSettings()
	}
}

type settingOption struct {
	value string
	label string
	icon  *ui.SVG
}

func themeOptions(msgs locales.Messages) []settingOption {
	return []settingOption{
		{string(mygo.ThemeSystem), msgs.GuiSystem(), kit.IconSliders},
		{string(mygo.ThemeLight), msgs.GuiLight(), kit.IconSun},
		{string(mygo.ThemeDark), msgs.GuiDark(), kit.IconMoon},
	}
}

func segmentedChoice(ctx *ui.Context, p kit.Palette, current string, options []settingOption, touch bool, choose func(string)) {
	index := 0
	labels := make([]string, len(options))
	for i, opt := range options {
		labels[i] = opt.label
		if opt.value == current {
			index = i
		}
	}
	selected := index
	kit.Segmented(ctx, p, &selected, labels, touch, func() {
		if selected >= 0 && selected < len(options) {
			choose(options[selected].value)
		}
	})
}

func languageOptions(msgs locales.Messages) []settingOption {
	options := []settingOption{{settingsAuto, msgs.GuiSystem(), nil}}
	for _, tag := range locales.Catalog.Languages() {
		options = append(options, settingOption{value: tag, label: locales.LanguageName(tag)})
	}
	return options
}

func optionLabel(options []settingOption, value string) string {
	for _, opt := range options {
		if opt.value == value {
			return opt.label
		}
	}
	if len(options) > 0 {
		return options[0].label
	}
	return value
}

func optionMenuItems(options []settingOption, current string, apply func(string)) []kit.MenuItem {
	items := make([]kit.MenuItem, len(options))
	for i, opt := range options {
		chosen := opt
		items[i] = kit.MenuItem{
			Value:    chosen.value,
			Label:    chosen.label,
			Icon:     chosen.icon,
			Radio:    true,
			Active:   chosen.value == current,
			OnSelect: func() { apply(chosen.value) },
		}
	}
	return items
}

func (d *dashboard) settingsPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages) {
	touch := d.touchUI(ctx)
	themes := themeOptions(msgs)
	languages := languageOptions(msgs)
	themeIcon := kit.IconSun
	if p.Dark {
		themeIcon = kit.IconMoon
	}

	kit.NarrowColumn(ctx, func() {
		kit.FormGroup(ctx, p, msgs.GuiAppearance(), nil, func() {
			kit.OptionRow(ctx.Key("opt-theme"), p, d.menuOpen("theme"), themeIcon, msgs.GuiTheme(),
				optionLabel(themes, string(d.settings.Theme)), touch,
				optionMenuItems(themes, string(d.settings.Theme), d.setTheme))

			kit.Divider(ctx, p)
			kit.IconSettingRow(ctx, p, kit.IconPalette, msgs.GuiAccentColor(), "", func() {
				ui.Row(ctx).Gap(10).Padding(4, 2).AlignItems(ui.Center).Children(func() {
					for _, hex := range kit.AccentPresets {
						preset := hex
						kit.ColorSwatch(ctx.Key("accent-"+preset), p, ui.Hex(preset), d.settings.Accent == preset, func() {
							d.settings.Accent = preset
							d.saveSettings()
						})
					}
				})
			})
			if !d.mobile {
				kit.Divider(ctx, p)
				navIcon := kit.IconSidebar
				if d.settings.Navigation == navigationHorizontal {
					navIcon = kit.IconTopBar
				}
				kit.IconSettingRow(ctx, p, navIcon, msgs.GuiNavigation(), "", func() {
					segmentedChoice(ctx.Key("navigation"), p, d.settings.Navigation, []settingOption{
						{value: navigationHorizontal, label: msgs.GuiTopNavigation()},
						{value: navigationSidebar, label: msgs.GuiSideNavigation()},
					}, touch, func(value string) {
						d.settings.Navigation = value
						d.saveSettings()
					})
				})
			} else {
				kit.Divider(ctx, p)
				curStyle := bottomBarCapsule
				if d.settings.BottomBarStyle == bottomBarStandard {
					curStyle = bottomBarStandard
				}
				kit.IconSettingRow(ctx, p, kit.IconBottomBar, msgs.GuiBottomBarStyle(), "", func() {
					segmentedChoice(ctx.Key("bottom-bar-style"), p, curStyle, []settingOption{
						{value: bottomBarCapsule, label: msgs.GuiBottomBarCapsule()},
						{value: bottomBarStandard, label: msgs.GuiBottomBarStandard()},
					}, touch, func(value string) {
						d.settings.BottomBarStyle = value
						d.saveSettings()
					})
				})
			}
			kit.Divider(ctx, p)
			kit.IconSettingRow(ctx, p, kit.IconLayers, msgs.GuiLiquidGlass(), "", func() {
				ui.Switch(ctx.Key("liquid-glass"), &d.settings.LiquidGlass).
					Label(msgs.GuiLiquidGlass()).OnChange(d.saveSettings)
			})
		})

		kit.FormGroup(ctx, p, msgs.GuiGeneral(), nil, func() {
			kit.OptionRow(ctx.Key("opt-language"), p, d.menuOpen("language"), kit.IconGlobe, msgs.GuiLanguage(),
				optionLabel(languages, d.language), touch,
				optionMenuItems(languages, d.language, func(value string) {
					d.language, d.settings.Language = value, value
					d.saveSettings()
				}))

			kit.Divider(ctx, p)
			kit.IconSettingRow(ctx, p, kit.IconActivity, msgs.GuiSmoothChart(), "", func() {
				ui.Switch(ctx.Key("smooth-chart"), &d.settings.SmoothChart).
					Label(msgs.GuiSmoothChart()).OnChange(d.saveSettings)
			})
		})
		d.islandSettings(ctx, p, msgs, touch)
		d.meterAccessSettings(ctx, p, msgs, touch)
	})
}
