//go:build !android

package app

import (
	"context"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

func (*dashboard) watchMeterNotification(context.Context, *backend.Service) func() {
	return func() {}
}

func ensureMeterForeground() {}

func (*dashboard) islandSettings(*ui.Context, kit.Palette, locales.Messages, bool) {}
