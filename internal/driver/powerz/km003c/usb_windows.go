package km003c

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/UruhaLushia/wattscope/internal/device"
	"golang.org/x/sys/windows"
)

var (
	winUSB           = windows.NewLazySystemDLL("winusb.dll")
	winInitialize    = winUSB.NewProc("WinUsb_Initialize")
	winFree          = winUSB.NewProc("WinUsb_Free")
	winGetDescriptor = winUSB.NewProc("WinUsb_GetDescriptor")
	winQuerySettings = winUSB.NewProc("WinUsb_QueryInterfaceSettings")
	winSetPipePolicy = winUSB.NewProc("WinUsb_SetPipePolicy")
	winReadPipe      = winUSB.NewProc("WinUsb_ReadPipe")
	winWritePipe     = winUSB.NewProc("WinUsb_WritePipe")
)

type winTransport struct {
	file  windows.Handle
	iface uintptr
}

func openWinUSB(path string) (*usbConnection, error) {
	if err := winUSB.Load(); err != nil {
		return nil, fmt.Errorf("load WinUSB: %w", err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	file, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open KM003C WinUSB device: %w", err)
	}
	conn := &winTransport{file: file}
	ok, _, err := winInitialize.Call(uintptr(file), uintptr(unsafe.Pointer(&conn.iface)))
	if ok == 0 {
		windows.CloseHandle(file)
		return nil, fmt.Errorf("initialize KM003C WinUSB interface: %w", err)
	}
	if err := conn.checkDevice(); err != nil {
		conn.Close()
		return nil, err
	}
	return &usbConnection{transport: conn}, nil
}

func (conn *winTransport) checkDevice() error {
	var descriptor [18]byte
	var length uint32
	ok, _, err := winGetDescriptor.Call(conn.iface, 1, 0, 0, uintptr(unsafe.Pointer(&descriptor[0])),
		uintptr(len(descriptor)), uintptr(unsafe.Pointer(&length)))
	if ok == 0 {
		return fmt.Errorf("read KM003C USB descriptor: %w", err)
	}
	if length != uint32(len(descriptor)) || !matchesUSBDescriptor(descriptor[:]) {
		return device.ErrNoDevice
	}
	var iface [9]byte
	ok, _, err = winQuerySettings.Call(conn.iface, 0, uintptr(unsafe.Pointer(&iface[0])))
	if ok == 0 {
		return fmt.Errorf("query KM003C WinUSB interface: %w", err)
	}
	if iface[2] != 0 || iface[4] < 2 || iface[5] != 0xff {
		return fmt.Errorf("KM003C vendor USB interface 0 is unavailable")
	}
	return nil
}

func (conn *winTransport) Close() error {
	ok, _, err := winFree.Call(conn.iface)
	closeErr := windows.CloseHandle(conn.file)
	if ok == 0 {
		return errors.Join(fmt.Errorf("release KM003C WinUSB interface: %w", err), closeErr)
	}
	return closeErr
}

func (conn *winTransport) bulk(ctx context.Context, endpoint byte, data []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline, _ := ctx.Deadline()
	timeout := uint32(max(1, time.Until(deadline).Milliseconds()))
	const pipeTransferTimeout = 3
	ok, _, err := winSetPipePolicy.Call(conn.iface, uintptr(endpoint), pipeTransferTimeout,
		unsafe.Sizeof(timeout), uintptr(unsafe.Pointer(&timeout)))
	if ok == 0 {
		return 0, fmt.Errorf("set KM003C WinUSB pipe timeout: %w", err)
	}
	proc := winReadPipe
	if endpoint == endpointOut {
		proc = winWritePipe
	}
	var length uint32
	ok, _, err = proc.Call(conn.iface, uintptr(endpoint), uintptr(unsafe.Pointer(unsafe.SliceData(data))),
		uintptr(len(data)), uintptr(unsafe.Pointer(&length)), 0)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ok == 0 {
		switch {
		case errors.Is(err, windows.ERROR_SEM_TIMEOUT):
			return 0, context.DeadlineExceeded
		case errors.Is(err, windows.ERROR_DEVICE_NOT_CONNECTED):
			return 0, device.ErrNoDevice
		default:
			return 0, err
		}
	}
	if length > uint32(len(data)) {
		return 0, fmt.Errorf("%w: invalid KM003C USB transfer length %d", device.ErrInvalidReport, length)
	}
	return int(length), nil
}
