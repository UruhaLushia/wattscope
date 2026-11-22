package device

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/UruhaLushia/wattscope/internal/diagnostics"
)

// Registry is immutable after construction. Drivers are wired explicitly by
// the application; adding a meter requires no changes to backend or frontends.
type Registry struct{ drivers []Driver }

func NewRegistry(drivers ...Driver) (*Registry, error) {
	seen := make(map[string]bool)
	for _, d := range drivers {
		if d == nil || d.ID() == "" {
			return nil, errors.New("driver ID is required")
		}
		if seen[d.ID()] {
			return nil, fmt.Errorf("duplicate driver %q", d.ID())
		}
		seen[d.ID()] = true
	}
	return &Registry{drivers: append([]Driver(nil), drivers...)}, nil
}

func (r *Registry) Drivers() []Driver { return append([]Driver(nil), r.drivers...) }

func (r *Registry) Discover(ctx context.Context, driver string) ([]Info, error) {
	logger := diagnostics.Logger(ctx)
	logger.InfoContext(ctx, diagnostics.ScanStarted, "driver_filter", driver)
	devices, err := r.discover(ctx, driver)
	if err != nil {
		logger.WarnContext(ctx, diagnostics.ScanFailed, "error", err)
		return devices, err
	}
	logger.InfoContext(ctx, diagnostics.ScanCompleted, "found", len(devices))
	for _, meter := range devices {
		logger.InfoContext(ctx, diagnostics.MeterFound, "driver", meter.Driver, "transport", meter.Transport, "path", meter.Path)
	}
	return devices, nil
}

func (r *Registry) discover(ctx context.Context, driver string) ([]Info, error) {
	logger := diagnostics.Logger(ctx)
	devices := make([]Info, 0)
	matched := driver == ""
	for _, d := range r.drivers {
		if driver != "" && d.ID() != driver {
			continue
		}
		matched = true
		started := time.Now()
		found, err := d.Discover(ctx)
		logger.DebugContext(ctx, diagnostics.DriverScanFinished, "driver", d.ID(), "found", len(found),
			"elapsed", time.Since(started).Round(time.Millisecond), "error", err)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.ID(), err)
		}
		devices = append(devices, found...)
	}
	if !matched {
		return nil, fmt.Errorf("unknown driver %q (run drivers)", driver)
	}
	return devices, nil
}

// Opener selects exactly one device and prefers its previous ID on reconnect.
// selector may be the ID or initial path; Linux hidraw renumbering is supported.
func (r *Registry) Opener(driver, selector string) OpenFunc {
	var bound Info
	var peers map[string]bool
	return func(ctx context.Context) (Info, Connection, error) {
		logger := diagnostics.Logger(ctx)
		devices, err := r.discover(ctx, driver)
		if err != nil {
			return Info{}, nil, err
		}
		matches := make([]Info, 0, len(devices))
		for _, d := range devices {
			if bound.ID != "" {
				if d.ID != bound.ID {
					continue
				}
			} else if selector != "" && !matchesSelector(d, selector) {
				continue
			}
			matches = append(matches, d)
		}
		if len(matches) == 0 && bound.PathDependentID {
			for _, d := range devices {
				if d.Driver == bound.Driver && !peers[d.ID] {
					matches = append(matches, d)
				}
			}
		}
		if len(matches) == 0 {
			return Info{}, nil, fmt.Errorf("%w; connect its data/HID port and run devices", ErrNoDevice)
		}
		if len(matches) != 1 {
			return Info{}, nil, fmt.Errorf("%w (%d); select one with --device", ErrMultipleDevices, len(matches))
		}
		d := matches[0]
		for _, implementation := range r.drivers {
			if implementation.ID() != d.Driver {
				continue
			}
			logger.DebugContext(ctx, diagnostics.OpeningMeter, "driver", d.Driver, "transport", d.Transport, "path", d.Path)
			connection, err := implementation.Open(ctx, d)
			if err == nil {
				bound = d
				if d.PathDependentID {
					peers = make(map[string]bool)
					for _, candidate := range devices {
						if candidate.Driver == d.Driver && candidate.ID != d.ID {
							peers[candidate.ID] = true
						}
					}
				}
			}
			return d, connection, err
		}
		return d, nil, fmt.Errorf("driver %q is not registered", d.Driver)
	}
}

func matchesSelector(d Info, selector string) bool {
	if d.ID == selector || d.Path == selector {
		return true
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(d.Path, selector)
	}
	// Accept stable device symlinks as well as the current kernel-assigned node.
	resolved, err := filepath.EvalSymlinks(selector)
	if err != nil {
		return false
	}
	path, err := filepath.EvalSymlinks(d.Path)
	return err == nil && resolved == path
}
