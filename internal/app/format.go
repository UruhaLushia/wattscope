package app

import (
	"fmt"
	"math"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

func displayedCurrentDirection(sample *device.Sample) device.CurrentDirection {
	if sample != nil && sample.CurrentDirectionStable {
		return sample.CurrentDirection
	}
	return ""
}

func displayedCurrentDirectionLabel(msgs locales.Messages, sample *device.Sample) string {
	switch displayedCurrentDirection(sample) {
	case device.CurrentPlugToReceptacle:
		return msgs.GuiPlugToReceptacle()
	case device.CurrentReceptacleToPlug:
		return msgs.GuiReceptacleToPlug()
	default:
		return ""
	}
}

func (d *dashboard) copyText(ctx *ui.Context, msgs locales.Messages, text string) {
	ctx.Services().WriteClipboard(text)
	d.notify(msgs.GuiCopied(), "success")
}

func clampOrder[T ~int | ~float32 | ~float64](v, lo, hi T) T {
	return max(lo, min(hi, v))
}

func (d *dashboard) touchUI() bool {
	return d.mobile
}

func formatDuration(seconds float64) string {
	return (time.Duration(math.Round(max(0, seconds))) * time.Second).String()
}

func formatSessionDuration(seconds float64) (string, string) {
	if seconds < 60 {
		return fmt.Sprintf("%.1f", max(0, seconds)), "s"
	}
	return formatDuration(seconds), ""
}

func middleEllipsis(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit || limit < 4 {
		return value
	}
	left := (limit - 1) / 2
	return string(runes[:left]) + "…" + string(runes[len(runes)-(limit-left-1):])
}
