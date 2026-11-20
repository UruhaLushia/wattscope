package app

import (
	"context"
	"sync"
	"time"

	"github.com/UruhaLushia/wattscope/internal/android/monitor"
	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/kit"
	"github.com/UruhaLushia/wattscope/internal/recording"
	"github.com/UruhaLushia/wattscope/locales"
	"github.com/egoist/mygo/ui"
)

var meterMonitor = sync.OnceValues(monitor.NewMonitor)

var supportsMeterIsland = sync.OnceValue(func() bool {
	instance, err := meterMonitor()
	if err != nil {
		return false
	}
	supported, _ := instance.SupportsIsland()
	return supported
})

func (d *dashboard) islandSettings(ctx *ui.Context, p kit.Palette, msgs locales.Messages, touch bool) {
	if !supportsMeterIsland() {
		return
	}
	options := []settingOption{
		{value: islandLeftVoltage, label: msgs.Voltage()},
		{value: islandLeftCurrent, label: msgs.Current()},
		{value: islandLeftEnergy, label: msgs.Energy()},
		{value: islandLeftCharge, label: msgs.ChargeShort()},
		{value: islandLeftOff, label: msgs.GuiOff()},
	}
	kit.FormGroup(ctx, p, "", nil, func() {
		kit.IconSettingRow(ctx, p, kit.IconLayers, msgs.GuiSuperIsland(), "", func() {
			ui.Switch(ctx.Key("island-enabled"), &d.settings.IslandEnabled).
				Label(msgs.GuiSuperIsland()).OnChange(func() {
				*d.menuOpen("island-left") = false
				d.syncIslandDisplay()
				d.saveSettings()
			})
		})
		if !d.settings.IslandEnabled {
			return
		}
		kit.Divider(ctx, p)
		current := d.settings.IslandLeft
		kit.OptionRow(ctx.Key("opt-island-left"), p, d.menuOpen("island-left"), kit.IconActivity,
			msgs.GuiIslandLeft(), optionLabel(options, current), touch,
			optionMenuItems(options, current, func(value string) {
				d.settings.IslandLeft = value
				d.syncIslandDisplay()
				d.saveSettings()
			}))
	})
}

func (d *dashboard) watchMeterNotification(ctx context.Context, service *backend.Service) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		updates, unsubscribe := service.Subscribe()
		defer unsubscribe()
	waitForMeter:
		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
				status := service.ConnectionStatus()
				if status.Connections > 0 && status.State != backend.Stopped {
					break waitForMeter
				}
			}
		}
		unsubscribe()

		instance, err := meterMonitor()
		if err != nil {
			return
		}
		defer instance.Stop()

		actionCtx, actionCancel := context.WithCancel(ctx)
		toggles := make(chan struct{})
		actionsDone := make(chan struct{})
		go func() {
			defer close(actionsDone)
			for instance.AwaitRecordingToggle(actionCtx) == nil {
				select {
				case toggles <- struct{}{}:
				case <-actionCtx.Done():
					return
				}
			}
		}()
		defer func() {
			actionCancel()
			<-actionsDone
		}()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for ctx.Err() == nil {
			snapshot := service.Snapshot(0)
			if snapshot.State == backend.Stopped {
				return
			}
			locale, _ := d.notifyLanguage.Load().(string)
			updateMeterNotification(instance, snapshot, locale, d.islandDisplay.Load())
			select {
			case <-ctx.Done():
				return
			case <-toggles:
				if ctx.Err() != nil {
					return
				}
				current := service.Snapshot(0)
				if current.Recording.Active {
					_ = service.StopRecording()
				} else if current.State == backend.Streaming {
					if dir, err := d.recordDir(); err == nil {
						_ = service.StartRecording(recording.NextPath(dir))
					}
				}
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func updateMeterNotification(instance monitor.Monitor, snapshot backend.Snapshot, language string, display *islandDisplay) {
	sample := snapshot.Latest
	streaming := snapshot.State == backend.Streaming && sample != nil
	var power, voltage, current float64
	if streaming {
		power, voltage, current = sample.Power, sample.Voltage, sample.Current
	}
	if display == nil {
		display = &islandDisplay{enabled: true, left: islandLeftVoltage}
	}
	_ = instance.Update(
		power,
		voltage,
		current,
		snapshot.Statistics.WattHours,
		snapshot.Statistics.AmpHours*1000,
		snapshot.Statistics.MeasuredSeconds,
		language,
		display.left,
		streaming,
		snapshot.Recording.Active,
		display.enabled,
	)
}

func ensureMeterForeground() {
	if instance, err := meterMonitor(); err == nil {
		_ = instance.Ensure()
	}
}
