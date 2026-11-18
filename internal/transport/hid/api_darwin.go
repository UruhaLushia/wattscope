package hid

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"

	"github.com/ebitengine/purego"
)

const (
	macUTF8                        = 0x08000100
	macSInt64                      = 4
	macRunLoopTimedOut             = 3
	macRunLoopHandledSource        = 4
	macNoDevice             uint32 = 0xe00002c0
	macNotPrivileged        uint32 = 0xe00002c1
	macNotOpen              uint32 = 0xe00002cd
	macNotPermitted         uint32 = 0xe00002e2
)

// CFIndex is signed and pointer-sized; IOReturn and Mach ports are 32-bit on
// both supported Darwin architectures. CF references are opaque handles.
type macFunctions struct {
	CFRelease                              func(uintptr)
	CFGetTypeID                            func(uintptr) uintptr
	CFStringGetTypeID                      func() uintptr
	CFStringCreateWithCString              func(uintptr, string, uint32) uintptr
	CFStringGetLength                      func(uintptr) int
	CFStringGetMaximumSizeForEncoding      func(int, uint32) int
	CFStringGetCString                     func(uintptr, *byte, int, uint32) bool
	CFNumberGetTypeID                      func() uintptr
	CFNumberCreate                         func(uintptr, int, *int64) uintptr
	CFNumberGetValue                       func(uintptr, int, *int64) bool
	CFDictionaryCreateMutable              func(uintptr, int, uintptr, uintptr) uintptr
	CFDictionarySetValue                   func(uintptr, uintptr, uintptr)
	CFSetGetCount                          func(uintptr) int
	CFSetGetValues                         func(uintptr, *uintptr)
	CFRunLoopGetCurrent                    func() uintptr
	CFRunLoopRunInMode                     func(uintptr, float64, bool) int32
	IOHIDManagerCreate                     func(uintptr, uint32) uintptr
	IOHIDManagerSetDeviceMatching          func(uintptr, uintptr)
	IOHIDManagerCopyDevices                func(uintptr) uintptr
	IOHIDManagerScheduleWithRunLoop        func(uintptr, uintptr, uintptr)
	IOHIDManagerUnscheduleFromRunLoop      func(uintptr, uintptr, uintptr)
	IOHIDDeviceCreate                      func(uintptr, uint32) uintptr
	IOHIDDeviceGetService                  func(uintptr) uint32
	IOHIDDeviceGetProperty                 func(uintptr, uintptr) uintptr
	IOHIDDeviceOpen                        func(uintptr, uint32) int32
	IOHIDDeviceClose                       func(uintptr, uint32) int32
	IOHIDDeviceRegisterInputReportCallback func(uintptr, *byte, int, uintptr, uintptr)
	IOHIDDeviceRegisterRemovalCallback     func(uintptr, uintptr, uintptr)
	IOHIDDeviceScheduleWithRunLoop         func(uintptr, uintptr, uintptr)
	IOHIDDeviceUnscheduleFromRunLoop       func(uintptr, uintptr, uintptr)
	IORegistryEntryGetRegistryEntryID      func(uint32, *uint64) int32
	IORegistryEntryGetPath                 func(uint32, string, *byte) int32
	IORegistryEntryIDMatching              func(uint64) uintptr
	IOServiceGetMatchingService            func(uint32, uintptr) uint32
	IOObjectRelease                        func(uint32) int32
}

type macAPI struct {
	macFunctions
	keys                            map[string]uintptr
	mode, dictKeys, dictValues      uintptr
	reportCallback, removalCallback uintptr
}

var macNative = sync.OnceValues(loadMacAPI)

func loadMacAPI() (*macAPI, error) {
	// These are system framework install names, not device or application paths.
	// No SDK, headers, compiler, or third-party native library is needed.
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load CoreFoundation: %w", err)
	}
	io, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		purego.Dlclose(cf)
		return nil, fmt.Errorf("load IOKit: %w", err)
	}
	a := &macAPI{keys: make(map[string]uintptr)}
	loaded := false
	defer func() {
		if !loaded {
			for _, key := range a.keys {
				a.CFRelease(key)
			}
			if a.mode != 0 {
				a.CFRelease(a.mode)
			}
			purego.Dlclose(io)
			purego.Dlclose(cf)
		}
	}()
	functions := reflect.ValueOf(&a.macFunctions).Elem()
	for i := range functions.NumField() {
		name := functions.Type().Field(i).Name
		library := cf
		if name[0] == 'I' {
			library = io
		}
		symbol, err := purego.Dlsym(library, name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", name, err)
		}
		purego.RegisterFunc(functions.Field(i).Addr().Interface(), symbol)
	}
	a.dictKeys, err = purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
	if err != nil {
		return nil, err
	}
	a.dictValues, err = purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"VendorID", "ProductID", "Product", "SerialNumber", "Transport", "MaxInputReportSize"} {
		key := a.CFStringCreateWithCString(0, name, macUTF8)
		if key == 0 {
			return nil, fmt.Errorf("create HID property key %s", name)
		}
		a.keys[name] = key
	}
	a.mode = a.CFStringCreateWithCString(0, "kCFRunLoopDefaultMode", macUTF8)
	if a.mode == 0 {
		return nil, fmt.Errorf("create HID run loop mode")
	}
	// Only two callbacks are allocated for the entire process. Contexts are
	// numeric tokens; native code never retains a Go reader pointer.
	a.reportCallback = purego.NewCallback(macInputReport)
	a.removalCallback = purego.NewCallback(macDeviceRemoved)
	loaded = true // Frameworks and immutable CF keys live for the process lifetime.
	return a, nil
}

func (a *macAPI) propertyNumber(device uintptr, key string) (int64, bool) {
	ref := a.IOHIDDeviceGetProperty(device, a.keys[key])
	if ref == 0 || a.CFGetTypeID(ref) != a.CFNumberGetTypeID() {
		return 0, false
	}
	var value int64
	ok := a.CFNumberGetValue(ref, macSInt64, &value)
	return value, ok
}

func (a *macAPI) propertyString(device uintptr, key string) string {
	ref := a.IOHIDDeviceGetProperty(device, a.keys[key])
	if ref == 0 || a.CFGetTypeID(ref) != a.CFStringGetTypeID() {
		return ""
	}
	length := a.CFStringGetLength(ref)
	if length < 0 || length > 4096 {
		return ""
	}
	size := a.CFStringGetMaximumSizeForEncoding(length, macUTF8)
	if size < 0 || size > 16384 {
		return ""
	}
	buffer := make([]byte, size+1)
	if !a.CFStringGetCString(ref, &buffer[0], len(buffer), macUTF8) {
		return ""
	}
	return macCString(buffer)
}

func macCString(buffer []byte) string {
	if end := bytes.IndexByte(buffer, 0); end >= 0 {
		return string(buffer[:end])
	}
	return "" // A missing terminator indicates an invalid native result.
}

func (a *macAPI) deviceInfo(ref uintptr) (Device, error) {
	vid, vok := a.propertyNumber(ref, "VendorID")
	pid, pok := a.propertyNumber(ref, "ProductID")
	if !vok || !pok || vid < 0 || vid > 65535 || pid < 0 || pid > 65535 || a.propertyString(ref, "Transport") != "USB" {
		return Device{}, errors.New("not a matching USB HID device")
	}
	// IOHIDDevice owns the service reference; do not release this borrowed port.
	service := a.IOHIDDeviceGetService(ref)
	var id uint64
	if service == 0 {
		return Device{}, errors.New("HID service is unavailable")
	}
	if err := macStatus("query HID registry ID", a.IORegistryEntryGetRegistryEntryID(service, &id)); err != nil {
		return Device{}, err
	}
	var location [512]byte // io_string_t, the public IORegistryEntryGetPath ABI.
	if err := macStatus("query HID location", a.IORegistryEntryGetPath(service, "IOService", &location[0])); err != nil {
		return Device{}, err
	}
	path := macCString(location[:])
	if id == 0 || path == "" {
		return Device{}, errors.New("HID registry identity is unavailable")
	}
	return Device{Path: "DevSrvsID:" + strconv.FormatUint(id, 10), Location: path,
		Name: a.propertyString(ref, "Product"), Serial: a.propertyString(ref, "SerialNumber"),
		VendorID: uint16(vid), ProductID: uint16(pid)}, nil
}

func macStatus(action string, status int32) error {
	if status == 0 {
		return nil
	}
	code := uint32(status)
	switch code {
	case macNotPrivileged, macNotPermitted:
		return fmt.Errorf("%s: %w (IOReturn 0x%08X)", action, os.ErrPermission, code)
	case macNoDevice, macNotOpen:
		return fmt.Errorf("%s: %w (IOReturn 0x%08X)", action, os.ErrNotExist, code)
	default:
		return fmt.Errorf("%s: IOReturn 0x%08X", action, code)
	}
}
