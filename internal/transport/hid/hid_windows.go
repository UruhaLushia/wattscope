package hid

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	hidDLL           = windows.NewLazySystemDLL("hid.dll")
	hidGetAttributes = hidDLL.NewProc("HidD_GetAttributes")
	hidGetSerial     = hidDLL.NewProc("HidD_GetSerialNumberString")
	hidClass         = windows.GUID{Data1: 0x4d1e55b2, Data2: 0xf16f, Data3: 0x11cf, Data4: [8]byte{0x88, 0xcb, 0, 0x11, 0x11, 0, 0, 0x30}}
)

func Enumerate(vendorID, productID uint16) ([]Device, error) {
	paths, err := windows.CM_Get_Device_Interface_List("", &hidClass, windows.CM_GET_DEVICE_INTERFACE_LIST_PRESENT)
	if err != nil {
		return nil, fmt.Errorf("enumerate HID: %w", err)
	}
	devices := make([]Device, 0)
	for _, path := range paths {
		if !strings.Contains(strings.ToLower(path), fmt.Sprintf("vid_%04x&pid_%04x", vendorID, productID)) {
			continue
		}
		d := Device{Path: path, Name: fmt.Sprintf("HID %04x:%04x", vendorID, productID), Location: path, VendorID: vendorID, ProductID: productID}
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			var serial [256]uint16
			if ok, _, _ := hidGetSerial.Call(uintptr(h), uintptr(unsafe.Pointer(&serial[0])), uintptr(unsafe.Sizeof(serial))); ok != 0 {
				d.Serial = windows.UTF16ToString(serial[:])
			}
			windows.CloseHandle(h)
		}
		devices = append(devices, d)
	}
	return devices, nil
}

type hidReader struct {
	handle     windows.Handle
	reportSize int
}

func Open(d Device, reportSize int) (Reader, error) {
	if reportSize < 1 || reportSize > maxReportSize {
		return nil, errors.New("invalid HID input report size")
	}
	p, err := windows.UTF16PtrFromString(d.Path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open HID: %w", err)
	}
	attrs := struct {
		Size                     uint32
		Vendor, Product, Version uint16
	}{}
	attrs.Size = uint32(unsafe.Sizeof(attrs))
	ok, _, _ := hidGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs)))
	if ok == 0 || attrs.Vendor != d.VendorID || attrs.Product != d.ProductID {
		windows.CloseHandle(h)
		return nil, errors.New("opened HID does not match selected VID/PID")
	}
	return &hidReader{handle: h, reportSize: reportSize}, nil
}

func (r *hidReader) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	buf := make([]byte, r.reportSize)
	var n uint32
	err = windows.ReadFile(r.handle, buf, &n, &ov)
	if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
		return nil, err
	}
	if errors.Is(err, windows.ERROR_IO_PENDING) {
		defer func() { runtime.KeepAlive(buf); runtime.KeepAlive(&ov) }()
		for {
			if err := ctx.Err(); err != nil {
				windows.CancelIoEx(r.handle, &ov)
				// Drain cancellation before releasing memory used by asynchronous I/O.
				windows.GetOverlappedResult(r.handle, &ov, &n, true)
				return nil, err
			}
			status, err := windows.WaitForSingleObject(event, 100)
			if err != nil {
				windows.CancelIoEx(r.handle, &ov)
				windows.GetOverlappedResult(r.handle, &ov, &n, true)
				return nil, err
			}
			if status == uint32(windows.WAIT_TIMEOUT) {
				continue
			}
			if err := windows.GetOverlappedResult(r.handle, &ov, &n, false); err != nil {
				return nil, err
			}
			break
		}
	}
	if n == 0 {
		return nil, errors.New("HID closed")
	}
	return buf[:n], nil
}

func (r *hidReader) Close() error { return windows.CloseHandle(r.handle) }
