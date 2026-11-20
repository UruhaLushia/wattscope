package app

import (
	"context"

	"github.com/UruhaLushia/wattscope/internal/diagnostics"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/transport/usbhost"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

type meterAccessState struct {
	shizukuAccess usbhost.ShizukuAccess
}

func (d *dashboard) automaticMeterContext() context.Context {
	return usbhost.WithoutPermissionPrompt(d.ctx)
}

func (d *dashboard) refreshMeterAccess() {
	d.shizukuAccess, _ = usbhost.GetShizukuAccess(supportsMeterIsland())
}

func shizukuStatusLabel(msgs locales.Messages, status usbhost.ShizukuAccess) string {
	switch status {
	case usbhost.ShizukuAuthorized, usbhost.ShizukuShellAuthorized:
		return msgs.GuiShizukuAuthorized()
	case usbhost.ShizukuNotAuthorized:
		return msgs.GuiShizukuNotAuthorized()
	case usbhost.ShizukuRequiresRoot:
		return msgs.GuiShizukuRequiresRoot()
	default:
		return msgs.GuiShizukuUnavailable()
	}
}

func (d *dashboard) meterAccessSettings(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	canUseIsland := supportsMeterIsland()
	hint := msgs.GuiShizukuUsbHint()
	if canUseIsland {
		hint = msgs.GuiShizukuIslandHint()
	}
	access := d.shizukuAccess
	kit.FormGroup(ctx, p, msgs.GuiMeterAccess(), nil, func() {
		kit.IconSettingRow(ctx, p, kit.IconShield, "Shizuku", shizukuStatusLabel(msgs, access), func() {
			if access != usbhost.ShizukuNotAuthorized {
				return
			}
			kit.ActionButton(ctx.Key("authorize-shizuku"), p, kit.VariantSecondary, nil, msgs.GuiShizukuAuthorize(), touch, func() {
				d.runJob(func(ctx context.Context) (func(), string, error) {
					access, err := usbhost.RequestShizukuAccess(ctx, canUseIsland)
					if err != nil {
						diagnostics.Logger(ctx).WarnContext(ctx, "Shizuku authorization failed", "error", err)
					}
					return func() {
						d.shizukuAccess = access
						if access == usbhost.ShizukuShellAuthorized {
							return
						}
						if access != usbhost.ShizukuAuthorized {
							message := shizukuStatusLabel(msgs, access)
							if access == usbhost.ShizukuNotAuthorized {
								message = msgs.GuiShizukuAllowInManager()
							}
							d.notify(message, "", "error")
							return
						}
						if d.done != nil {
							select {
							case <-d.done:
							default:
								return
							}
						}
						d.connectWith(d.automaticMeterContext(), d.source)
					}, "", ctx.Err()
				})
			}).Disabled(d.busy)
		})
		ui.Text(ctx, hint).Wrap().FillWidth().Padding(0, 0, 12, 0).FontSize(11).TextColor(p.Muted)
	})
}
