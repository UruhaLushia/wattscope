//go:build !android && !linux

package app

import (
	"context"

	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

type meterAccessState struct{}

func (d *dashboard) automaticMeterContext() context.Context { return d.ctx }

func (*dashboard) refreshMeterAccess() {}

func (*dashboard) meterAccessSettings(*ui.Context, kit.Palette, locales.Messages, bool) {}
