//go:build !linux && !windows && !darwin

package hid

import (
	"fmt"
	"runtime"
)

func Enumerate(uint16, uint16) ([]Device, error) {
	return nil, fmt.Errorf("native HID backend for %s is not implemented yet; --demo is available", runtime.GOOS)
}
func Open(Device, int) (Reader, error) {
	return nil, fmt.Errorf("native HID backend for %s is not implemented yet", runtime.GOOS)
}
