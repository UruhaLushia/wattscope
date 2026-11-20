package hid

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func Enumerate(vendorID, productID uint16) ([]Device, error) {
	paths, err := filepath.Glob("/sys/class/hidraw/hidraw*")
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0)
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(path, "device/uevent"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("enumerate HID: %w", err)
		}
		props := make(map[string]string)
		for line := range strings.SplitSeq(string(data), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				props[k] = v
			}
		}
		var bus, vid, pid uint32
		if _, err := fmt.Sscanf(props["HID_ID"], "%x:%x:%x", &bus, &vid, &pid); err != nil {
			continue
		}
		if bus != 3 || vid != uint32(vendorID) || pid != uint32(productID) {
			continue
		}
		devices = append(devices, Device{
			Path: filepath.Join("/dev", filepath.Base(path)), Name: props["HID_NAME"],
			Serial: props["HID_UNIQ"], Location: props["HID_PHYS"],
			VendorID: vendorID, ProductID: productID,
		})
	}
	return devices, nil
}

type hidReader struct{ fd, reportSize int }

func Open(d Device, reportSize int) (Reader, error) {
	return open(d, reportSize, unix.O_RDONLY)
}

func open(d Device, reportSize, access int) (*hidReader, error) {
	if reportSize < 1 || reportSize > maxReportSize {
		return nil, errors.New("invalid HID input report size")
	}
	fd, err := unix.Open(d.Path, access|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) {
			return nil, fmt.Errorf("open %s: device access not granted to the current user: %w", d.Path, err)
		}
		return nil, fmt.Errorf("open %s: %w", d.Path, err)
	}
	info, err := unix.IoctlHIDGetRawInfo(fd)
	if err != nil || uint16(info.Vendor) != d.VendorID || uint16(info.Product) != d.ProductID {
		unix.Close(fd)
		return nil, errors.New("opened HID does not match selected VID/PID")
	}
	return &hidReader{fd: fd, reportSize: reportSize}, nil
}

func Request(ctx context.Context, d Device, report []byte, accept func([]byte) bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := open(d, len(report), unix.O_RDWR)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	size, err := unix.Write(reader.fd, report)
	if err != nil {
		return nil, fmt.Errorf("send HID request: %w", err)
	}
	if size != len(report) {
		return nil, errors.New("short HID request")
	}
	for {
		reply, err := reader.Read(ctx)
		if err != nil {
			return nil, err
		}
		if accept(reply) {
			return reply, nil
		}
	}
}

func (r *hidReader) Read(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		wait := 100
		if deadline, ok := ctx.Deadline(); ok {
			remaining := max(int(time.Until(deadline).Milliseconds()), 1)
			if remaining < wait {
				wait = remaining
			}
		}
		fds := []unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}
		_, err := unix.Poll(fds, wait)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return nil, errors.New("HID disconnected")
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}
		buf := make([]byte, r.reportSize)
		n, err := unix.Read(r.fd, buf)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read HID: %w", err)
		}
		if n == 0 {
			return nil, errors.New("HID closed")
		}
		return buf[:n], nil
	}
}

func (r *hidReader) Close() error { return unix.Close(r.fd) }
