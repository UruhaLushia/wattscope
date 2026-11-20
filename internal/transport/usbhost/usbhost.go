package usbhost

import (
	"context"

	"github.com/egoist/mygo/native"
)

var (
	ErrPermission         = native.NewError("permission")
	ErrPermissionRequired = native.NewError("permissionRequired")
	ErrDisconnected       = native.NewError("disconnected")
)

type ShizukuAccess int

const (
	ShizukuUnavailable ShizukuAccess = iota
	ShizukuRequiresRoot
	ShizukuNotAuthorized
	ShizukuAuthorized
)

//mygo:native
type Host interface {
	Devices(vendor int, product int) ([]string, error)
	ShizukuStatus() (ShizukuAccess, error)
	AuthorizeShizuku(ctx context.Context) (ShizukuAccess, error)
	Open(ctx context.Context, path string, interfaceNumber int, input int, output int, prompt bool) (int, error)
	Read(handle int, size int, timeoutMS int) ([]byte, error)
	Write(handle int, data []byte, timeoutMS int) (int, error)
	Close(handle int) error
	UpdateNotification(text, language string) error
	EnsureForeground() error
	StopForeground() error
}
