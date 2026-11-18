//go:build !android || no_gui

package km003c

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
)

func (Driver) LinuxUSBMatches() []device.LinuxUSBMatch {
	return []device.LinuxUSBMatch{{VendorID: vendorID, ProductID: productID}}
}

func (driver Driver) Discover(ctx context.Context) ([]device.Info, error) {
	return driver.discover(ctx, "/sys/bus/usb/devices")
}

func (driver Driver) discover(ctx context.Context, root string) ([]device.Info, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var devices []device.Info
	vendor, product := fmt.Sprintf("%04x", vendorID), fmt.Sprintf("%04x", productID)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		location := entry.Name()
		dir := filepath.Join(root, location)
		if sysfsText(dir, "idVendor") != vendor || sysfsText(dir, "idProduct") != product {
			continue
		}
		bus, busErr := strconv.Atoi(sysfsText(dir, "busnum"))
		address, addressErr := strconv.Atoi(sysfsText(dir, "devnum"))
		if busErr != nil || addressErr != nil || bus <= 0 || address <= 0 {
			continue
		}
		serial := sysfsText(dir, "serial")
		info := device.Info{
			ID: driver.ID() + ":" + location + ":" + serial, Driver: driver.ID(),
			Name: driver.Name(), Serial: serial,
			Path: fmt.Sprintf("/dev/bus/usb/%03d/%03d", bus, address), Transport: "usb",
			Capabilities: []string{device.CapPDCapture},
		}
		if hwmon := findHwmon(dir, location); hwmon != "" {
			info.Path, info.Transport, info.Capabilities = hwmon, "hwmon", []string{device.CapPDDirectUSB}
		}
		devices = append(devices, info)
	}
	return devices, nil
}

func (driver Driver) Open(ctx context.Context, selected device.Info) (device.Connection, error) {
	devices, err := driver.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, info := range devices {
		if info.ID != selected.ID || info.Path != selected.Path || info.Transport != selected.Transport {
			continue
		}
		if info.Transport == "hwmon" {
			return &hwmonConnection{path: info.Path}, nil
		}
		return openUSB(info.Path)
	}
	return nil, device.ErrNoDevice
}

func findHwmon(dir, location string) string {
	root := filepath.Join(dir, location+":1.0", "hwmon")
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if sysfsText(path, "name") == "powerz" {
			return path
		}
	}
	return ""
}

func sysfsText(dir, name string) string {
	data, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimSpace(string(data))
}

type hwmonConnection struct {
	path     string
	nextPoll time.Time
}

func (conn *hwmonConnection) Read(ctx context.Context) (device.Sample, error) {
	if err := waitPoll(ctx, conn.nextPoll); err != nil {
		return device.Sample{}, err
	}
	conn.nextPoll = time.Now().Add(pollInterval)
	var values [7]float64
	for index, name := range [...]string{
		"in0_input", "curr1_input", "temp1_input", "in1_input", "in2_input", "in3_input", "in4_input",
	} {
		if err := ctx.Err(); err != nil {
			return device.Sample{}, err
		}
		data, err := os.ReadFile(filepath.Join(conn.path, name))
		if err != nil {
			return device.Sample{}, err
		}
		value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return device.Sample{}, fmt.Errorf("%w: %s: %v", device.ErrInvalidReport, name, err)
		}
		values[index] = float64(value) / 1000
	}
	sample := measurement(values[0], values[1])
	sample.Temperature, sample.CC1, sample.CC2, sample.DP, sample.DN = &values[2], &values[3], &values[4], &values[5], &values[6]
	return sample, nil
}

func (*hwmonConnection) Close() error { return nil }
