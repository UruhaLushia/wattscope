package app

import (
	"fmt"

	"github.com/UruhaLushia/wattscope/internal/backend"
	"github.com/UruhaLushia/wattscope/internal/transport/usbhost"
)

func updateMeterNotification(snapshot backend.Snapshot, language string) {
	if snapshot.State != backend.Streaming || snapshot.Latest == nil {
		_ = usbhost.UpdateNotification("", language)
		return
	}
	sample := snapshot.Latest
	_ = usbhost.UpdateNotification(fmt.Sprintf("%.2f V · %.2f A · %.2f W", sample.Voltage, sample.Current, sample.Power), language)
}

func ensureMeterForeground() { _ = usbhost.EnsureForeground() }
func stopMeterForeground()   { _ = usbhost.StopForeground() }
