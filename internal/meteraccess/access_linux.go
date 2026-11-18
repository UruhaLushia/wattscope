//go:build linux && !android

package meteraccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/session"
)

const Command = "--authorize-meters"

func Authorize(ctx context.Context) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	output, err := exec.CommandContext(ctx, "pkexec", "--disable-internal-agent", executable, Command).CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 126 {
		return false, nil
	}
	if detail := strings.TrimSpace(string(output)); detail != "" {
		return false, errors.New(detail)
	}
	return false, err
}

func Apply(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return errors.New("meter authorization requires root")
	}
	rules, matches := meterRules(session.Drivers())
	if len(matches) == 0 {
		return nil
	}
	udevadm, err := findUdevadm()
	if err != nil {
		return err
	}
	if err := writeRules("/etc/udev/rules.d/70-wattscope.rules", rules); err != nil {
		return err
	}
	if err := runUdevadm(ctx, udevadm, "control", "--reload-rules"); err != nil {
		return err
	}
	return refreshDevices(ctx, udevadm, "/sys/bus/usb/devices", matches)
}

func meterRules(drivers []device.Driver) (string, []device.LinuxUSBMatch) {
	var rules strings.Builder
	var matches []device.LinuxUSBMatch
	for _, driver := range drivers {
		provider, ok := driver.(interface{ LinuxUSBMatches() []device.LinuxUSBMatch })
		if !ok {
			continue
		}
		for _, match := range provider.LinuxUSBMatches() {
			if slices.Contains(matches, match) {
				continue
			}
			matches = append(matches, match)
			rule := `SUBSYSTEM=="usb", ENV{DEVTYPE}=="usb_device", ATTR{idVendor}=="%04x", ATTR{idProduct}=="%04x", TAG+="uaccess"`
			if match.HIDRaw {
				rule = `SUBSYSTEM=="hidraw", ATTRS{idVendor}=="%04x", ATTRS{idProduct}=="%04x", TAG+="uaccess"`
			}
			fmt.Fprintf(&rules, rule+"\n", match.VendorID, match.ProductID)
		}
	}
	return rules.String(), matches
}

func writeRules(path, rules string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".70-wattscope.*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err := temporary.WriteString(rules); err != nil {
		return err
	}
	if err := temporary.Chmod(0644); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func findUdevadm() (string, error) {
	for _, directory := range []string{"/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		if path, err := exec.LookPath(filepath.Join(directory, "udevadm")); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("udevadm: %w", exec.ErrNotFound)
}

func refreshDevices(ctx context.Context, udevadm, usbDirectory string, matches []device.LinuxUSBMatch) error {
	entries, err := os.ReadDir(usbDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(usbDirectory, entry.Name())
		vendor, vendorErr := os.ReadFile(filepath.Join(path, "idVendor"))
		product, productErr := os.ReadFile(filepath.Join(path, "idProduct"))
		if vendorErr != nil || productErr != nil {
			continue
		}
		vendorID := strings.TrimSpace(string(vendor))
		productID := strings.TrimSpace(string(product))
		for _, match := range matches {
			if vendorID != fmt.Sprintf("%04x", match.VendorID) || productID != fmt.Sprintf("%04x", match.ProductID) {
				continue
			}
			args := []string{"trigger", "--settle", path}
			if match.HIDRaw {
				args = []string{"trigger", "--settle", "--subsystem-match=hidraw", "--parent-match=" + path}
			}
			if err := runUdevadm(ctx, udevadm, args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func runUdevadm(ctx context.Context, path string, args ...string) error {
	command := exec.CommandContext(ctx, path, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("udevadm %s: %w", args[0], err)
	}
	return nil
}
