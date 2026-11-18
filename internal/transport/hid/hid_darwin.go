package hid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

const macReportQueue = 2048

func Enumerate(vendorID, productID uint16) ([]Device, error) {
	a, err := macNative()
	if err != nil {
		return nil, err
	}
	// A fresh manager owns each enumeration, including its run loop scheduling.
	// Replugging never depends on events left on another goroutine's OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	manager := a.IOHIDManagerCreate(0, 0)
	if manager == 0 {
		return nil, errors.New("create macOS HID manager")
	}
	defer a.CFRelease(manager)
	match := a.CFDictionaryCreateMutable(0, 2, a.dictKeys, a.dictValues)
	if match == 0 {
		return nil, errors.New("create HID matching dictionary")
	}
	defer a.CFRelease(match)
	for _, property := range []struct {
		key   string
		value uint16
	}{{"VendorID", vendorID}, {"ProductID", productID}} {
		value := int64(property.value)
		number := a.CFNumberCreate(0, macSInt64, &value)
		if number == 0 {
			return nil, errors.New("create HID matching value")
		}
		a.CFDictionarySetValue(match, a.keys[property.key], number)
		a.CFRelease(number) // The dictionary retains its keys and values.
	}
	a.IOHIDManagerSetDeviceMatching(manager, match)
	loop := a.CFRunLoopGetCurrent()
	a.IOHIDManagerScheduleWithRunLoop(manager, loop, a.mode)
	defer a.IOHIDManagerUnscheduleFromRunLoop(manager, loop, a.mode)
	a.CFRunLoopRunInMode(a.mode, 0.001, false)
	set := a.IOHIDManagerCopyDevices(manager)
	devices := make([]Device, 0)
	if set == 0 {
		return devices, nil
	}
	defer a.CFRelease(set)
	count := a.CFSetGetCount(set)
	if count < 0 || count > 65536 {
		return nil, errors.New("invalid HID device count")
	}
	if count == 0 {
		return devices, nil
	}
	refs := make([]uintptr, count)
	a.CFSetGetValues(set, &refs[0])
	seen := make(map[string]bool)
	for _, ref := range refs {
		d, err := a.deviceInfo(ref)
		if err != nil || d.VendorID != vendorID || d.ProductID != productID || seen[d.Path] {
			continue
		}
		seen[d.Path] = true
		devices = append(devices, d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	return devices, nil
}

var (
	macReaders    sync.Map
	macNextReader atomic.Uint64
)

type hidReader struct {
	reportSize         int
	reports            chan []byte
	stop, failed, done chan struct{}
	stopOnce, failOnce sync.Once
	failure, closeErr  error // Published by failed/done respectively.
}

func Open(d Device, reportSize int) (Reader, error) {
	if reportSize < 1 || reportSize > maxReportSize {
		return nil, errors.New("invalid HID input report size")
	}
	if !strings.HasPrefix(d.Path, "DevSrvsID:") {
		return nil, errors.New("invalid HID device path")
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(d.Path, "DevSrvsID:"), 10, 64)
	if err != nil || id == 0 {
		return nil, errors.New("invalid HID registry ID")
	}
	a, err := macNative()
	if err != nil {
		return nil, err
	}
	r := &hidReader{reportSize: reportSize, reports: make(chan []byte, macReportQueue),
		stop: make(chan struct{}), failed: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go r.receive(a, d, id, ready)
	if err := <-ready; err != nil {
		<-r.done
		return nil, fmt.Errorf("open macOS HID %s: %w", d.Path, err)
	}
	return r, nil
}

func (r *hidReader) receive(a *macAPI, selected Device, id uint64, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(r.done)
	match := a.IORegistryEntryIDMatching(id)
	if match == 0 {
		ready <- errors.New("create HID registry match")
		return
	}
	// IOServiceGetMatchingService consumes the matching dictionary.
	service := a.IOServiceGetMatchingService(0, match)
	if service == 0 {
		ready <- errors.New("selected HID registry entry is unavailable")
		return
	}
	device := a.IOHIDDeviceCreate(0, service)
	a.IOObjectRelease(service)
	if device == 0 {
		ready <- errors.New("create macOS HID device")
		return
	}
	var pin runtime.Pinner
	var buffer []byte
	var token, loop uintptr
	opened, scheduled := false, false
	defer func() {
		// Callbacks and cleanup share this OS thread. The native object is
		// stopped, closed and released before its buffer is unpinned.
		if scheduled {
			a.IOHIDDeviceUnscheduleFromRunLoop(device, loop, a.mode)
			a.IOHIDDeviceRegisterInputReportCallback(device, nil, 0, 0, token)
			a.IOHIDDeviceRegisterRemovalCallback(device, 0, token)
		}
		if opened {
			status := a.IOHIDDeviceClose(device, 0)
			if code := uint32(status); code != macNoDevice && code != macNotOpen {
				r.closeErr = macStatus("close HID device", status)
			}
		}
		a.CFRelease(device)
		if token != 0 {
			macReaders.Delete(token)
		}
		pin.Unpin()
		runtime.KeepAlive(buffer)
	}()
	actual, err := a.deviceInfo(device)
	if err != nil {
		ready <- err
		return
	}
	if actual.Path != selected.Path || actual.VendorID != selected.VendorID || actual.ProductID != selected.ProductID || actual.Serial != selected.Serial || (selected.Location != "" && actual.Location != selected.Location) {
		ready <- errors.New("opened HID does not match selected VID/PID/serial/location")
		return
	}
	inputSize, ok := a.propertyNumber(device, "MaxInputReportSize")
	if !ok || inputSize < 1 || inputSize > maxReportSize {
		ready <- errors.New("invalid native HID input report size")
		return
	}
	// Zero options means shared access. Never seize the device or send reports.
	if err := macStatus("open HID device", a.IOHIDDeviceOpen(device, 0)); err != nil {
		ready <- err
		return
	}
	opened = true
	buffer = make([]byte, max(r.reportSize, int(inputSize)))
	pin.Pin(&buffer[0])
	token = uintptr(macNextReader.Add(1))
	macReaders.Store(token, r)
	loop = a.CFRunLoopGetCurrent()
	a.IOHIDDeviceRegisterInputReportCallback(device, &buffer[0], len(buffer), a.reportCallback, token)
	a.IOHIDDeviceRegisterRemovalCallback(device, a.removalCallback, token)
	a.IOHIDDeviceScheduleWithRunLoop(device, loop, a.mode)
	scheduled = true
	ready <- nil
	for {
		select {
		case <-r.stop:
			return
		case <-r.failed:
			return
		default:
			result := a.CFRunLoopRunInMode(a.mode, 0.1, true)
			if result != macRunLoopTimedOut && result != macRunLoopHandledSource {
				r.fail(fmt.Errorf("macOS HID event loop stopped: %d", result))
				return
			}
		}
	}
}

// Copy the report before IOKit reuses its pinned buffer.
func macInputReport(token uintptr, result int32, _ uintptr, reportType uint32, _ uint32, report unsafe.Pointer, length int) {
	value, ok := macReaders.Load(token)
	if !ok {
		return
	}
	r := value.(*hidReader)
	if result != 0 {
		r.fail(macStatus("receive HID report", result))
		return
	}
	if reportType != 0 { // kIOHIDReportTypeInput
		return
	}
	if report == nil || length < 1 || length > r.reportSize {
		r.fail(fmt.Errorf("invalid HID input report length: %d", length))
		return
	}
	packet := append([]byte(nil), unsafe.Slice((*byte)(report), length)...)
	select {
	case r.reports <- packet:
	default:
		r.fail(fmt.Errorf("macOS HID input queue overflow; capture interrupted"))
	}
}

func macDeviceRemoved(token uintptr, _ int32, _ uintptr) {
	if value, ok := macReaders.Load(token); ok {
		value.(*hidReader).fail(fmt.Errorf("macOS HID device disconnected"))
	}
}

func (r *hidReader) fail(err error) {
	r.failOnce.Do(func() { r.failure = err; close(r.failed) })
}

func (r *hidReader) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-r.stop:
		return nil, os.ErrClosed
	default:
	}
	// Deliver already received packets before a disconnect/overflow error.
	select {
	case report := <-r.reports:
		return report, nil
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.stop:
		return nil, os.ErrClosed
	case report := <-r.reports:
		return report, nil
	case <-r.failed:
	case <-r.done:
	}
	// A terminal signal can arrive together with the final buffered report.
	select {
	case report := <-r.reports:
		return report, nil
	default:
	}
	select {
	case <-r.failed:
		return nil, r.failure
	default:
		return nil, os.ErrClosed
	}
}

func (r *hidReader) Close() error {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
	return r.closeErr
}
