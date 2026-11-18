package km003c

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/ebitengine/purego"
)

type macPlugInTable struct {
	_, queryInterface, _, release uintptr
}

var errMacUSBNotOpen = errors.New("USB interface is not open")

type macUSBTable struct {
	_, _, _, release uintptr
	_                [4]uintptr
	open, close      uintptr
	_                [9]uintptr
	numEndpoints     uintptr
	_                [6]uintptr
	pipeProperties   uintptr
	_                [12]uintptr
	read, write      uintptr
}

type macUSB struct {
	iface           unsafe.Pointer
	inPipe, outPipe uint8
	release         func(unsafe.Pointer) uint32
	close           func(unsafe.Pointer) int32
	read            func(unsafe.Pointer, uint8, *byte, *uint32, uint32, uint32) int32
	write           func(unsafe.Pointer, uint8, *byte, uint32, uint32, uint32) int32
}

func (api *macAPI) openUSB(service uint32) (*macUSB, error) {
	var plugin unsafe.Pointer
	var score int32
	status := api.IOCreatePlugInInterfaceForService(service, api.userClientType, api.plugInType, &plugin, &score)
	if err := macUSBStatus("create KM003C interface plug-in", status); err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, fmt.Errorf("missing KM003C interface plug-in")
	}
	var queryInterface func(unsafe.Pointer, macUUID, *unsafe.Pointer) int32
	var releasePlugin func(unsafe.Pointer) uint32
	plugInTable := *(**macPlugInTable)(plugin)
	purego.RegisterFunc(&releasePlugin, plugInTable.release)
	purego.RegisterFunc(&queryInterface, plugInTable.queryInterface)
	conn := &macUSB{}
	status = queryInterface(plugin, usbInterfaceID(), &conn.iface)
	releasePlugin(plugin)
	if status != 0 || conn.iface == nil {
		return nil, fmt.Errorf("query KM003C USB interface: HRESULT 0x%08x", uint32(status))
	}
	table := *(**macUSBTable)(conn.iface)
	var open func(unsafe.Pointer) int32
	purego.RegisterFunc(&open, table.open)
	purego.RegisterFunc(&conn.release, table.release)
	purego.RegisterFunc(&conn.close, table.close)
	purego.RegisterFunc(&conn.read, table.read)
	purego.RegisterFunc(&conn.write, table.write)
	if err := macUSBStatus("open KM003C USB interface 0", open(conn.iface)); err != nil {
		conn.release(conn.iface)
		return nil, err
	}
	if err := conn.findPipes(table); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (conn *macUSB) findPipes(table *macUSBTable) error {
	var getCount func(unsafe.Pointer, *uint8) int32
	var properties func(unsafe.Pointer, uint8, *uint8, *uint8, *uint8, *uint16, *uint8) int32
	purego.RegisterFunc(&getCount, table.numEndpoints)
	purego.RegisterFunc(&properties, table.pipeProperties)
	var count uint8
	if err := macUSBStatus("read KM003C endpoint count", getCount(conn.iface, &count)); err != nil {
		return err
	}
	for index := 1; index <= int(count); index++ {
		pipe := uint8(index)
		var direction, number, transferType, interval uint8
		var packetSize uint16
		status := properties(conn.iface, pipe, &direction, &number, &transferType, &packetSize, &interval)
		if err := macUSBStatus("read KM003C pipe properties", status); err != nil {
			return err
		}
		if transferType != 2 {
			continue
		}
		switch number | direction<<7 {
		case endpointIn:
			conn.inPipe = pipe
		case endpointOut:
			conn.outPipe = pipe
		}
	}
	if conn.inPipe == 0 || conn.outPipe == 0 {
		return fmt.Errorf("KM003C bulk endpoints 0x01 and 0x81 are unavailable")
	}
	return nil
}

func (conn *macUSB) Close() error {
	err := macUSBStatus("close KM003C USB interface", conn.close(conn.iface))
	conn.release(conn.iface)
	if errors.Is(err, device.ErrNoDevice) || errors.Is(err, errMacUSBNotOpen) {
		return nil
	}
	return err
}

func (conn *macUSB) bulk(ctx context.Context, endpoint byte, data []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline, _ := ctx.Deadline()
	timeout := uint32(max(1, time.Until(deadline).Milliseconds()))
	length := uint32(len(data))
	var status int32
	switch endpoint {
	case endpointIn:
		status = conn.read(conn.iface, conn.inPipe, unsafe.SliceData(data), &length, timeout, timeout)
	case endpointOut:
		status = conn.write(conn.iface, conn.outPipe, unsafe.SliceData(data), length, timeout, timeout)
	default:
		return 0, fmt.Errorf("unknown KM003C endpoint 0x%02x", endpoint)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := macUSBStatus("transfer KM003C USB data", status); err != nil {
		return 0, err
	}
	if length > uint32(len(data)) {
		return 0, fmt.Errorf("%w: invalid KM003C USB transfer length %d", device.ErrInvalidReport, length)
	}
	return int(length), nil
}

func macUSBStatus(action string, status int32) error {
	code := uint32(status)
	switch code {
	case 0:
		return nil
	case 0xe00002c0:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, device.ErrNoDevice, code)
	case 0xe00002cd:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, errMacUSBNotOpen, code)
	case 0xe00002c1, 0xe00002e2:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, os.ErrPermission, code)
	case 0xe00002c5:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, syscall.EBUSY, code)
	case 0xe00002d6, 0xe0004051:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, context.DeadlineExceeded, code)
	case 0xe00002eb:
		return fmt.Errorf("%s: %w (IOReturn 0x%08x)", action, errBulkAborted, code)
	default:
		return fmt.Errorf("%s: IOReturn 0x%08x", action, code)
	}
}
