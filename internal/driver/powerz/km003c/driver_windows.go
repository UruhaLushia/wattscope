package km003c

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/UruhaLushia/wattscope/internal/device"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const windowsKM003CInterface = `usb\vid_5fc9&pid_0063&mi_00\`

func (driver Driver) Discover(ctx context.Context) ([]device.Info, error) {
	devices, err := windows.SetupDiGetClassDevsEx(nil, "USB", 0, windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES, 0, "")
	if err != nil {
		return nil, fmt.Errorf("enumerate Windows USB devices: %w", err)
	}
	defer devices.Close()
	var found []device.Info
	for index := 0; ctx.Err() == nil; index++ {
		entry, err := devices.EnumDeviceInfo(index)
		if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("enumerate Windows USB devices: %w", err)
		}
		instance, err := devices.DeviceInstanceID(entry)
		if err != nil || !strings.HasPrefix(strings.ToLower(instance), windowsKM003CInterface) {
			continue
		}
		service, err := devices.DeviceRegistryProperty(entry, windows.SPDRP_SERVICE)
		if name, ok := service.(string); err != nil || !ok || !strings.EqualFold(name, "WinUSB") {
			continue
		}
		path := findWinUSBPath(devices, entry, instance)
		if path == "" {
			continue
		}
		found = append(found, device.Info{
			ID: driver.ID() + ":" + strings.ToLower(instance), Driver: driver.ID(),
			Path: path, Name: driver.Name(), Transport: "usb",
			Capabilities: []string{device.CapPDCapture},
		})
	}
	return found, ctx.Err()
}

func findWinUSBPath(devices windows.DevInfo, entry *windows.DevInfoData, instance string) string {
	handle, err := devices.OpenDevRegKey(entry, windows.DICS_FLAG_GLOBAL, 0, windows.DIREG_DEV, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	key := registry.Key(handle)
	defer key.Close()
	for _, name := range []string{"DeviceInterfaceGUIDs", "DeviceInterfaceGUID"} {
		values, _, err := key.GetStringsValue(name)
		if err != nil {
			if value, _, err := key.GetStringValue(name); err == nil {
				values = []string{value}
			}
		}
		for _, value := range values {
			guid, err := windows.GUIDFromString(strings.TrimSpace(value))
			if err != nil {
				continue
			}
			paths, err := windows.CM_Get_Device_Interface_List(instance, &guid, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
			if err == nil && len(paths) != 0 {
				return paths[0]
			}
		}
	}
	return ""
}

func (driver Driver) Open(ctx context.Context, selected device.Info) (device.Connection, error) {
	devices, err := driver.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, info := range devices {
		if info.ID == selected.ID && strings.EqualFold(info.Path, selected.Path) {
			return openWinUSB(info.Path)
		}
	}
	return nil, device.ErrNoDevice
}
