package app

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/protocol/pd"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (d *dashboard) recordDir() (string, error) {
	return recording.Directory(d.dir)
}

func (d *dashboard) csvButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, state backend.Recording, touch bool) ui.Element {
	label, icon, variant := msgs.RecordCsv(), kit.IconRecord, kit.VariantSecondary
	if state.Active {
		label, icon, variant = msgs.ActionStop()+" CSV", kit.IconStop, kit.VariantDanger
	}
	return kit.ActionButton(ctx.Key("csv"), p, variant, icon, label, touch, func() {
		service := d.service
		dir, err := d.recordDir()
		d.runJob(func(ctx context.Context) (func(), string, error) {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			if state.Active {
				return nil, msgs.RecordingSaved(), service.StopRecording()
			}
			if err != nil {
				return nil, "", err
			}
			path := recording.NextPath(dir)
			return nil, msgs.RecordingTo(path), service.StartRecording(path)
		})
	}).Disabled(d.service == nil || d.busy)
}

func (d *dashboard) captureButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, state backend.CaptureRecording, touch bool) {
	label, icon, variant := msgs.PdRecordStart(), kit.IconRecord, kit.VariantSecondary
	if state.Active {
		label, icon, variant = msgs.PdRecordStop(), kit.IconStop, kit.VariantDanger
	}
	kit.ActionButton(ctx.Key("capture"), p, variant, icon, label, touch, func() {
		service := d.service
		dir, err := d.recordDir()
		d.runJob(func(ctx context.Context) (func(), string, error) {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			if state.Active {
				return nil, msgs.RecordingSaved(), service.StopCapture()
			}
			if err != nil {
				return nil, "", err
			}
			path := recording.NextCapturePath(dir)
			return nil, msgs.CaptureStarted(path), service.StartCapture(path)
		})
	}).Disabled(d.service == nil || d.busy)
}

func (d *dashboard) openCapture(msgs locales.Messages) {
	if !d.mobile {
		d.openCapturePicker(msgs)
		return
	}
	dir, _ := d.recordDir()
	window := d.window
	d.runJob(func(ctx context.Context) (func(), string, error) {
		paths, err := awaitDialog(ctx, func() ([]string, error) {
			return mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: window, Title: msgs.GuiOpenCapture(), DefaultPath: dir,
			})
		})
		if err != nil || len(paths) == 0 {
			return nil, "", err
		}
		apply, err := d.readCaptureApply(ctx, paths[0])
		return apply, "", err
	})
}

func (d *dashboard) exportPackets(msgs locales.Messages, visible []pd.Packet) {
	dir, err := d.recordDir()
	d.runJob(func(ctx context.Context) (func(), string, error) {
		if err != nil {
			return nil, "", err
		}
		path := recording.NextCapturePath(dir)
		if err := recording.ExportCaptureContext(ctx, path, visible); err != nil {
			return nil, "", err
		}
		if runtime.GOOS == "android" {
			target, err := awaitDialog(ctx, func() (string, error) {
				return mygo.Dialog.SaveCopy(path, filepath.Base(path))
			})
			if err != nil {
				return nil, "", err
			}
			if target != "" {
				path = target
			}
		}
		return nil, msgs.ExportedPackets(fmt.Sprint(len(visible)), path), nil
	})
}

func (d *dashboard) checkPD(msgs locales.Messages) {
	service := d.service
	sessionCtx := d.sessionCtx
	d.runJob(func(ctx context.Context) (func(), string, error) {
		if sessionCtx != nil {
			ctx = sessionCtx
		}
		enabled, err := service.CheckSavedPDHost(ctx)
		if ctx.Err() != nil {
			return nil, "", nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", msgs.PdCheckFailed(), err)
		}
		if enabled {
			return nil, msgs.GuiPdOnline() + " · " + msgs.GuiOn(), nil
		}
		return nil, msgs.GuiPdOnline() + " · " + msgs.GuiOff(), nil
	})
}

func (d *dashboard) recordsPage(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot) {
	touch := d.touchUI(ctx)
	var capture backend.CaptureRecording
	if d.service != nil {
		capture = d.service.PacketSnapshot(0, 0).Recording
	}
	packets := d.packetSnapshot()
	visible := d.packets.visiblePackets(msgs, packets.Packets)

	kit.NarrowColumn(ctx, func() {
		kit.Card(ctx, p).Gap(10).Children(func() {
			ui.Row(ctx).FillWidth().MinHeight(28).AlignItems(ui.Center).Gap(8).Children(func() {
				kit.SectionTitle(ctx, msgs.ActionRecord())
			})
			kit.Divider(ctx, p)
			d.recordRow(ctx, p, msgs, "CSV", snap.Recording.Path, snap.Recording.Active, touch, func() {
				if snap.Recording.Path != "" && !snap.Recording.Active {
					d.saveCopyButton(ctx, p, msgs, "CSV", snap.Recording.Path, touch)
				}
				d.csvButton(ctx, p, msgs, snap.Recording, touch)
			})
			kit.Divider(ctx, p)
			d.recordRow(ctx, p, msgs, msgs.PagePd(), capture.Path, capture.Active, touch, func() {
				kit.ActionButton(ctx.Key("open"), p, kit.VariantSecondary, kit.IconFolder, msgs.GuiOpenCapture(), touch, func() {
					d.openCapture(msgs)
				}).Disabled(d.busy)
				kit.ActionButton(ctx.Key("export"), p, kit.VariantSecondary, kit.IconDownload, msgs.ExportPackets(), touch, func() {
					d.exportPackets(msgs, visible)
				}).Disabled(d.busy || len(visible) == 0)
				if capture.Path != "" && !capture.Active {
					d.saveCopyButton(ctx, p, msgs, "pd", capture.Path, touch)
				}
				d.captureButton(ctx, p, msgs, capture, touch)
			})
		})

		ui.Row(ctx).FillWidth().Wrap().Gap(12).AlignItems(ui.Stretch).Children(func() {
			if snap.Statistics.Samples != 0 {
				d.sessionCard(ctx, p, msgs, snap, touch).Grow(1).Basis(300).MinWidth(260)
			}
			d.directoryCard(ctx, p, msgs, touch).Grow(1).Basis(300).MinWidth(260)
		})
	})
}

func (d *dashboard) saveCopyButton(ctx *ui.Context, p kit.Palette, msgs locales.Messages, key, path string, touch bool) {
	kit.ActionButton(ctx.Key("save-"+key), p, kit.VariantSecondary, kit.IconDownload, msgs.GuiSaveCopy(), touch, func() {
		if !d.mobile {
			d.openSaveCopyPicker(msgs, path)
			return
		}
		d.runJob(func(ctx context.Context) (func(), string, error) {
			target, err := awaitDialog(ctx, func() (string, error) {
				return mygo.Dialog.SaveCopy(path, filepath.Base(path))
			})
			return nil, target, err
		})
	}).Disabled(d.busy)
}

func (d *dashboard) recordRow(ctx *ui.Context, p kit.Palette, msgs locales.Messages, title, path string, active bool, touch bool, controls func()) {
	tone := kit.TonePlain
	status := msgs.GuiIdle()
	switch {
	case active:
		status, tone = msgs.Recording(), kit.ToneDanger
	case path != "":
		status, tone = msgs.RecordingSaved(), kit.ToneSuccess
	}
	if d.mobile {
		ui.Column(ctx).FillWidth().Padding(6, 0).Gap(10).Children(func() {
			ui.Column(ctx).FillWidth().Gap(4).Children(func() {
				ui.Row(ctx).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(8).Children(func() {
					ui.Text(ctx, title).FontSize(14).FontWeight(700)
					kit.Badge(ctx, p, status, tone)
				})
				if path != "" {
					ui.Text(ctx, filepath.Base(path)).FillWidth().SingleLine().FontSize(12).TextColor(p.Muted).Tooltip(path)
				}
			})
			ui.Row(ctx).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(controls)
		})
		return
	}
	ui.Row(ctx).FillWidth().Wrap().Padding(4, 0).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(ctx).Grow(1).Basis(160).MinWidth(120).Gap(4).Children(func() {
			ui.Row(ctx).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(ctx, title).FontSize(13).FontWeight(700)
				kit.Badge(ctx, p, status, tone)
			})
			if path != "" {
				ui.Text(ctx, filepath.Base(path)).FillWidth().SingleLine().FontSize(11).TextColor(p.Muted).Tooltip(path)
			}
		})
		ui.Row(ctx).Shrink(0).MaxWidthPercent(100).Wrap().Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(controls)
	})
}

func (d *dashboard) sessionCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, snap backend.Snapshot, touch bool) ui.Element {
	stats := snap.Statistics
	return kit.Card(ctx, p).Gap(10).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(28).AlignItems(ui.Center).Gap(8).Children(func() {
			kit.SectionTitle(ctx, msgs.GuiSession())
			ui.Spacer(ctx)
			kit.ActionButton(ctx.Key("clear-session"), p, kit.VariantGhost, kit.IconTrash, msgs.ActionClear(), touch, func() {
				d.resetChartView()
				if d.service != nil {
					d.service.ResetStatistics()
				}
			}).Height(28).Padding(0, 8).Disabled(d.service == nil)
		})
		kit.Divider(ctx, p)
		ui.Row(ctx).FillWidth().Wrap().Gap(12).AlignItems(ui.Center).Children(func() {
			kit.MetricSummaryCell(ctx, p, msgs.Energy(), fmt.Sprintf("%.4f", stats.WattHours), "Wh")
			kit.VerticalDivider(ctx, p, 24)
			kit.MetricSummaryCell(ctx, p, msgs.ChargeShort(), fmt.Sprintf("%.3f", stats.AbsoluteAmpHours*1000), "mAh")
			kit.VerticalDivider(ctx, p, 24)
			durVal, durUnit := formatSessionDuration(stats.MeasuredSeconds)
			kit.MetricSummaryCell(ctx, p, msgs.GuiDuration(), durVal, durUnit)
		})
	})
}

func (d *dashboard) directoryCard(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) ui.Element {
	dir, err := d.recordDir()
	text := dir
	textColor := p.Muted
	if err != nil {
		text = err.Error()
		textColor = p.Red
	}
	return kit.Card(ctx, p).Gap(10).Children(func() {
		ui.Row(ctx).FillWidth().MinHeight(28).AlignItems(ui.Center).Gap(8).Children(func() {
			kit.SectionTitle(ctx, msgs.GuiDir())
			ui.Spacer(ctx)
			if !d.mobile && err == nil {
				kit.ActionButton(ctx.Key("open-dir"), p, kit.VariantGhost, kit.IconFolder, msgs.GuiOpenFolder(), touch, func() {
					_ = mygo.Shell.OpenPath(dir)
				}).Height(28).Padding(0, 8)
			}
		})
		kit.Divider(ctx, p)
		ui.Text(ctx, text).FillWidth().Font("monospace").FontSize(12).TextColor(textColor).Wrap().Selectable()
	})
}
