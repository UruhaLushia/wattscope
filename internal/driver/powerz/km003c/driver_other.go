//go:build !linux && !darwin && !windows

package km003c

import (
	"context"
	"fmt"
	"runtime"

	"github.com/UruhaLushia/wattscope/internal/device"
)

func (Driver) Discover(context.Context) ([]device.Info, error) {
	return nil, fmt.Errorf("native KM003C backend for %s is unavailable", runtime.GOOS)
}

func (Driver) Open(context.Context, device.Info) (device.Connection, error) {
	return nil, fmt.Errorf("native KM003C backend for %s is unavailable", runtime.GOOS)
}
