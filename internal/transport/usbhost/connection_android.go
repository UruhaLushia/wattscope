package usbhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/UruhaLushia/wattscope/internal/device"
	"github.com/UruhaLushia/wattscope/internal/diagnostics"
)

var usbManager = sync.OnceValues(NewHost)

type noPermissionPrompt struct{}

func WithoutPermissionPrompt(ctx context.Context) context.Context {
	return context.WithValue(ctx, noPermissionPrompt{}, true)
}

func GetShizukuAccess(allowShell bool) (ShizukuAccess, error) {
	host, err := usbManager()
	if err != nil {
		return ShizukuUnavailable, err
	}
	return host.ShizukuStatus(allowShell)
}

func RequestShizukuAccess(ctx context.Context, allowShell bool) (ShizukuAccess, error) {
	host, err := usbManager()
	if err != nil {
		return ShizukuUnavailable, err
	}
	return host.AuthorizeShizuku(ctx, allowShell)
}

func Enumerate(ctx context.Context, vendor, product int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, err := usbManager()
	if err != nil {
		return nil, err
	}
	return host.Devices(vendor, product)
}

type Connection struct {
	host   Host
	handle int
}

func Connect(ctx context.Context, path string, iface, input, output int) (*Connection, error) {
	host, err := usbManager()
	if err != nil {
		return nil, err
	}
	logger := diagnostics.Logger(ctx).With("path", path, "interface", iface)
	logger.InfoContext(ctx, diagnostics.AndroidOpening)
	quiet, _ := ctx.Value(noPermissionPrompt{}).(bool)
	handle, err := host.Open(ctx, path, iface, input, output, !quiet)
	if err != nil {
		return nil, usbError(err)
	}
	logger.InfoContext(ctx, diagnostics.AndroidAccessGranted)
	return &Connection{host: host, handle: handle}, nil
}

func (conn *Connection) Read(ctx context.Context, size int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := conn.host.Read(conn.handle, size, timeout(ctx))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, usbError(err)
	}
	if len(data) == 0 {
		return nil, context.DeadlineExceeded
	}
	return data, nil
}

func (conn *Connection) Write(ctx context.Context, data []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	count, err := conn.host.Write(conn.handle, data, timeout(ctx))
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err != nil {
		return 0, usbError(err)
	}
	if count < 0 {
		return 0, context.DeadlineExceeded
	}
	return count, nil
}

func (conn *Connection) Close() error { return conn.host.Close(conn.handle) }

func timeout(ctx context.Context) int {
	if deadline, ok := ctx.Deadline(); ok {
		return int(max(1, time.Until(deadline).Milliseconds()))
	}
	return 500
}

func usbError(err error) error {
	switch {
	case errors.Is(err, ErrPermission):
		return fmt.Errorf("%w: %w", device.ErrAccessDeclined, err)
	case errors.Is(err, ErrPermissionRequired):
		return fmt.Errorf("%w: %w", os.ErrPermission, err)
	case errors.Is(err, ErrDisconnected):
		return fmt.Errorf("%w: %w", device.ErrNoDevice, err)
	default:
		return err
	}
}
