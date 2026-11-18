//go:build linux && !android

package app

import (
	"context"
	"errors"

	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/meteraccess"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

type meterAccessState struct{}

func (d *dashboard) automaticMeterContext() context.Context { return d.ctx }

func (*dashboard) refreshMeterAccess() {}

func (d *dashboard) meterAccessSettings(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	kit.FormGroup(ctx, p, msgs.GuiMeterAccess(), nil, func() {
		kit.IconSettingRow(ctx, p, kit.IconShield, msgs.GuiMeterRules(), msgs.GuiMeterRulesHint(), func() {
			kit.ActionButton(ctx.Key("authorize-udev"), p, kit.VariantSecondary, nil, msgs.GuiMeterRulesAuthorize(), touch, func() {
				d.runJob(func(ctx context.Context) (func(), string, error) {
					authorized, err := meteraccess.Authorize(ctx)
					if err != nil {
						return nil, "", errors.New(msgs.GuiMeterRulesFailed(err.Error()))
					}
					if !authorized {
						return nil, "", nil
					}
					return nil, msgs.GuiMeterRulesInstalled(), nil
				})
			}).Disabled(d.busy)
		})
	})
}
