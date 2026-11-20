//go:build !android

package app

import "github.com/UruhaLushia/wattscope/internal/backend"

func updateMeterNotification(backend.Snapshot, string) {}
func ensureMeterForeground()                           {}
func stopMeterForeground()                             {}
