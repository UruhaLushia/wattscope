package km003c

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/ebitengine/purego"
)

type macFunctions struct {
	CFRelease                         func(uintptr)
	CFStringCreateWithCString         func(uintptr, string, uint32) uintptr
	CFStringGetCString                func(uintptr, *byte, int, uint32) bool
	CFGetTypeID                       func(uintptr) uintptr
	CFStringGetTypeID                 func() uintptr
	CFNumberGetTypeID                 func() uintptr
	CFNumberGetValue                  func(uintptr, int, *int64) bool
	CFUUIDCreateFromString            func(uintptr, uintptr) uintptr
	IOServiceMatching                 func(string) uintptr
	IOServiceGetMatchingServices      func(uint32, uintptr, *uint32) int32
	IOServiceGetMatchingService       func(uint32, uintptr) uint32
	IOIteratorNext                    func(uint32) uint32
	IOObjectRelease                   func(uint32) int32
	IORegistryEntryIDMatching         func(uint64) uintptr
	IORegistryEntryGetRegistryEntryID func(uint32, *uint64) int32
	IORegistryEntryGetParentEntry     func(uint32, string, *uint32) int32
	IORegistryEntryCreateCFProperty   func(uint32, uintptr, uintptr, uint32) uintptr
	IOCreatePlugInInterfaceForService func(uint32, uintptr, uintptr, *unsafe.Pointer, *int32) int32
}

type macUUID struct{ low, high uint64 }

type macAPI struct {
	macFunctions
	userClientType, plugInType uintptr
}

var macNative = sync.OnceValues(loadMacAPI)

const macUTF8 = 0x08000100

func loadMacAPI() (*macAPI, error) {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load CoreFoundation: %w", err)
	}
	ioKit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		purego.Dlclose(cf)
		return nil, fmt.Errorf("load IOKit: %w", err)
	}
	api := &macAPI{}
	loaded := false
	defer func() {
		if !loaded {
			if api.userClientType != 0 {
				api.CFRelease(api.userClientType)
			}
			if api.plugInType != 0 {
				api.CFRelease(api.plugInType)
			}
			purego.Dlclose(ioKit)
			purego.Dlclose(cf)
		}
	}()
	bindings := reflect.ValueOf(&api.macFunctions).Elem()
	for index := range bindings.NumField() {
		name := bindings.Type().Field(index).Name
		library := cf
		if strings.HasPrefix(name, "IO") {
			library = ioKit
		}
		symbol, err := purego.Dlsym(library, name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", name, err)
		}
		purego.RegisterFunc(bindings.Field(index).Addr().Interface(), symbol)
	}
	api.userClientType = api.uuid("2D9786C6-9EF3-11D4-AD51-000A27052861")
	api.plugInType = api.uuid("C244E858-109C-11D4-91D4-0050E4C6426F")
	if api.userClientType == 0 || api.plugInType == 0 {
		return nil, fmt.Errorf("create IOKit USB interface UUIDs")
	}
	loaded = true
	return api, nil
}

func usbInterfaceID() macUUID {
	id := [16]byte{0x6a, 0xe4, 0x4d, 0x3f, 0xeb, 0x45, 0x48, 0x7f, 0x8e, 0x8e, 0xb9, 0x3b, 0x99, 0xf8, 0xea, 0x9e}
	return macUUID{low: binary.LittleEndian.Uint64(id[:8]), high: binary.LittleEndian.Uint64(id[8:])}
}

func (api *macAPI) uuid(value string) uintptr {
	text := api.CFStringCreateWithCString(0, value, macUTF8)
	if text == 0 {
		return 0
	}
	defer api.CFRelease(text)
	return api.CFUUIDCreateFromString(0, text)
}

func (api *macAPI) property(entry uint32, name string) uintptr {
	key := api.CFStringCreateWithCString(0, name, macUTF8)
	if key == 0 {
		return 0
	}
	defer api.CFRelease(key)
	return api.IORegistryEntryCreateCFProperty(entry, key, 0, 0)
}

func (api *macAPI) number(entry uint32, name string) (int64, bool) {
	value := api.property(entry, name)
	if value == 0 {
		return 0, false
	}
	defer api.CFRelease(value)
	if api.CFGetTypeID(value) != api.CFNumberGetTypeID() {
		return 0, false
	}
	var number int64
	ok := api.CFNumberGetValue(value, 4, &number)
	return number, ok
}

func (api *macAPI) serial(entry uint32) string {
	for _, name := range []string{"kUSBSerialNumberString", "USB Serial Number"} {
		value := api.property(entry, name)
		if value == 0 {
			continue
		}
		var buffer [1024]byte
		ok := api.CFGetTypeID(value) == api.CFStringGetTypeID() &&
			api.CFStringGetCString(value, &buffer[0], len(buffer), macUTF8)
		api.CFRelease(value)
		if ok {
			if end := bytes.IndexByte(buffer[:], 0); end >= 0 {
				return string(buffer[:end])
			}
		}
	}
	return ""
}

const macUSBPathPrefix = "USBInterfaceID:"

func (api *macAPI) info(driver Driver, iface uint32) (device.Info, bool) {
	number, ok := api.number(iface, "bInterfaceNumber")
	if !ok || number != 0 {
		return device.Info{}, false
	}
	var parent uint32
	if api.IORegistryEntryGetParentEntry(iface, "IOService", &parent) != 0 || parent == 0 {
		return device.Info{}, false
	}
	defer api.IOObjectRelease(parent)
	vendor, _ := api.number(parent, "idVendor")
	product, _ := api.number(parent, "idProduct")
	if vendor != vendorID || product != productID {
		return device.Info{}, false
	}
	var registryID uint64
	location, ok := api.number(parent, "locationID")
	if !ok || api.IORegistryEntryGetRegistryEntryID(iface, &registryID) != 0 || registryID == 0 {
		return device.Info{}, false
	}
	serial := api.serial(parent)
	return device.Info{
		ID: fmt.Sprintf("%s:%08x:%s", driver.ID(), uint32(location), serial), Driver: driver.ID(),
		Name: driver.Name(), Serial: serial, Transport: "usb", Capabilities: []string{device.CapPDCapture},
		Path: macUSBPathPrefix + strconv.FormatUint(registryID, 10),
	}, true
}

func (driver Driver) Discover(ctx context.Context) ([]device.Info, error) {
	api, err := macNative()
	if err != nil {
		return nil, err
	}
	matching := api.IOServiceMatching("IOUSBHostInterface")
	if matching == 0 {
		return nil, fmt.Errorf("create IOKit USB matching dictionary")
	}
	var iterator uint32
	if err := macUSBStatus("discover USB interfaces", api.IOServiceGetMatchingServices(0, matching, &iterator)); err != nil {
		return nil, err
	}
	defer api.IOObjectRelease(iterator)
	var devices []device.Info
	for ctx.Err() == nil {
		iface := api.IOIteratorNext(iterator)
		if iface == 0 {
			break
		}
		info, ok := api.info(driver, iface)
		api.IOObjectRelease(iface)
		if ok {
			devices = append(devices, info)
		}
	}
	return devices, ctx.Err()
}

func (driver Driver) Open(ctx context.Context, selected device.Info) (device.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := strings.CutPrefix(selected.Path, macUSBPathPrefix)
	registryID, err := strconv.ParseUint(value, 10, 64)
	if !ok || err != nil || registryID == 0 {
		return nil, device.ErrNoDevice
	}
	api, err := macNative()
	if err != nil {
		return nil, err
	}
	matching := api.IORegistryEntryIDMatching(registryID)
	if matching == 0 {
		return nil, device.ErrNoDevice
	}
	iface := api.IOServiceGetMatchingService(0, matching)
	if iface == 0 {
		return nil, device.ErrNoDevice
	}
	defer api.IOObjectRelease(iface)
	info, ok := api.info(driver, iface)
	if !ok || info.ID != selected.ID {
		return nil, device.ErrNoDevice
	}
	transport, err := api.openUSB(iface)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		transport.Close()
		return nil, err
	}
	return &usbConnection{transport: transport}, nil
}
