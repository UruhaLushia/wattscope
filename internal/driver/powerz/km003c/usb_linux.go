//go:build !android || no_gui

package km003c

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/UruhaLushia/wattscope/internal/device"
	"golang.org/x/sys/unix"
)

type bulkTransfer struct {
	endpoint uint32
	length   uint32
	timeout  uint32
	data     unsafe.Pointer
}

type usbFile struct{ *os.File }

func openUSB(path string) (*usbConnection, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var descriptor [18]byte
	_, err = io.ReadFull(file, descriptor[:])
	if err == nil && !matchesUSBDescriptor(descriptor[:]) {
		err = device.ErrNoDevice
	}
	if err == nil {
		err = unix.IoctlSetPointerInt(int(file.Fd()), usbIOCTL(15, 4, false), 0)
	}
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("open KM003C USB interface 0: %w", err)
	}
	return &usbConnection{transport: usbFile{file}}, nil
}

func (file usbFile) bulk(ctx context.Context, endpoint byte, data []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline, _ := ctx.Deadline()
	transfer := bulkTransfer{
		endpoint: uint32(endpoint), length: uint32(len(data)),
		timeout: uint32(max(1, time.Until(deadline).Milliseconds())),
		data:    unsafe.Pointer(unsafe.SliceData(data)),
	}
	count, _, errno := unix.Syscall(unix.SYS_IOCTL, file.Fd(),
		uintptr(usbIOCTL(2, unsafe.Sizeof(transfer), true)), uintptr(unsafe.Pointer(&transfer)))
	runtime.KeepAlive(file.File)
	runtime.KeepAlive(data)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if errors.Is(errno, unix.ETIMEDOUT) {
		return 0, context.DeadlineExceeded
	}
	if errno != 0 {
		return 0, errno
	}
	return int(count), nil
}

func usbIOCTL(command, size uintptr, inOut bool) uint {
	direction, shift := uintptr(2), 30
	switch runtime.GOARCH {
	case "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le":
		shift = 29
		if inOut {
			direction = 6
		}
	default:
		if inOut {
			direction = 3
		}
	}
	return uint(direction<<shift | size<<16 | 'U'<<8 | command)
}
