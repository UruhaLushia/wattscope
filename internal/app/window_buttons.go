package app

import (
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

const (
	windowButtonWidth  = 28
	windowButtonGap    = 4
	windowButtonsWidth = 3*windowButtonWidth + 2*windowButtonGap
)

func (d *dashboard) windowButtons(ctx *ui.Context, p kit.Palette, msgs locales.Messages) {
	window := d.window
	maximizeLabel, maximizeIcon := msgs.GuiMaximize(), kit.IconMaximize
	if window != nil && window.IsMaximized() {
		maximizeLabel, maximizeIcon = msgs.GuiRestoreWindow(), kit.IconRestore
	}
	ui.Row(ctx).Width(windowButtonsWidth).Shrink(0).Gap(windowButtonGap).AlignItems(ui.Center).Children(func() {
		windowControlButton(ctx.Key("minimize"), p, msgs.GuiMinimize(), kit.IconMinimize, false, func() {
			if window != nil {
				go window.Minimize()
			}
		})
		windowControlButton(ctx.Key("maximize"), p, maximizeLabel, maximizeIcon, false, func() {
			if window != nil {
				go window.ToggleMaximize()
			}
		}).MaximizeButton()
		windowControlButton(ctx.Key("close"), p, msgs.GuiCloseWindow(), kit.IconClose, true, func() {
			if window != nil {
				go window.Close()
			}
		})
	})
}

func windowControlButton(ctx *ui.Context, p kit.Palette, label string, icon *ui.SVG, danger bool, onClick func()) ui.Element {
	button := ui.ButtonBase(ctx).Label(label).Tooltip(label).Size(windowButtonWidth, windowButtonWidth).
		Shrink(0).Radius(kit.RadiusS).Center().OnClick(onClick)
	hovered := button.Hovered() || button.FocusVisible() || button.Pressed()
	bg, fg := ui.Transparent, p.Muted
	switch {
	case danger && hovered:
		bg, fg = p.Red, kit.AccentInk(p.Red)
	case button.Pressed():
		bg, fg = p.Press, p.Text
	case hovered:
		bg, fg = p.Hover, p.Text
	}
	kit.ControlMotion(button).Background(button.AnimateColor("win-bg", bg, kit.MotionFast))
	return button.Children(func() {
		ui.Icon(ctx, icon).FontSize(14).TextColor(button.AnimateColor("win-fg", fg, kit.MotionFast))
	})
}
