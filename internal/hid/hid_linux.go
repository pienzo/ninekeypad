package hid

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Tested on Ubuntu 26.04.1 with the 9-key keypad (September 2026). The approach (sysfs
// discovery, report descriptor usage, /dev/hidrawN read/write) follows vaydeer-studio-linux,
// which was tested on real hardware; see docs/protocol.md.

const sysClassHidraw = "/sys/class/hidraw"

// Enumerate lists the hidraw nodes of one USB device.
func Enumerate(vid, pid uint16) ([]Interface, error) {
	entries, err := filepath.Glob(filepath.Join(sysClassHidraw, "hidraw*"))
	if err != nil {
		return nil, err
	}
	var out []Interface
	for _, entry := range entries {
		dev, err := filepath.EvalSymlinks(filepath.Join(entry, "device"))
		if err != nil {
			continue
		}
		uevent, err := os.ReadFile(filepath.Join(dev, "uevent"))
		if err != nil {
			continue
		}
		v, p, ok := ParseHIDID(string(uevent))
		if !ok || v != vid || p != pid {
			continue
		}
		info := Interface{Path: filepath.Join("/dev", filepath.Base(entry)), VendorID: v, ProductID: p, Interface: -1}
		if desc, err := os.ReadFile(filepath.Join(dev, "report_descriptor")); err == nil {
			info.UsagePage, info.Usage = ParseUsage(desc)
		}
		info.Interface = ParseInterfaceNumber(dev)
		out = append(out, info)
	}
	return out, nil
}

type linuxDevice struct {
	f  *os.File
	fd int // kept for the polling fallback; f.Fd() would switch the fd to blocking
}

// Open opens a /dev/hidrawN node exclusively (flock) for reading and writing.
// Needs root, or a udev rule granting access.
func Open(path string) (Device, error) {
	return open(path, syscall.O_RDWR, true)
}

// OpenReader opens a /dev/hidrawN node read-only, shared with other programs.
func OpenReader(path string) (Device, error) {
	return open(path, syscall.O_RDONLY, false)
}

func open(path string, mode int, exclusive bool) (Device, error) {
	fd, err := syscall.Open(path, mode|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		if err == syscall.EACCES || err == syscall.EPERM {
			return nil, ErrPermission
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// Other programs that honour flock (vaydeer-studio-linux does) stay out while we talk.
	if exclusive {
		if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			syscall.Close(fd)
			if err == syscall.EWOULDBLOCK {
				return nil, ErrBusy
			}
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
	}
	// A non-blocking fd makes os.File pollable, so read deadlines work.
	d := &linuxDevice{f: os.NewFile(uintptr(fd), path), fd: fd}
	// Drop answers another program left queued on this node.
	for i := 0; i < 256; i++ {
		if _, err := d.Read(time.Millisecond); err != nil {
			break
		}
	}
	return d, nil
}

func (d *linuxDevice) Write(report []byte) error {
	// hidraw: first byte is the report ID (0 = unnumbered), the kernel strips it.
	n, err := d.f.Write(report)
	if err != nil {
		return err
	}
	if n != len(report) {
		return fmt.Errorf("partial write: %d of %d bytes", n, len(report))
	}
	return nil
}

func (d *linuxDevice) Read(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, 64)
	deadline := time.Now().Add(timeout)
	if err := d.f.SetReadDeadline(deadline); err != nil {
		if errors.Is(err, os.ErrClosed) {
			return nil, err // never read a closed fd number: it may belong to another file now
		}
		return d.readPolling(buf, deadline)
	}
	n, err := d.f.Read(buf)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, ErrTimeout
	}
	if err != nil {
		return nil, err
	}
	// hidraw returns unnumbered reports without a report ID byte.
	return buf[:n], nil
}

// readPolling is the fallback when the fd could not join Go's poller.
func (d *linuxDevice) readPolling(buf []byte, deadline time.Time) ([]byte, error) {
	for {
		n, err := syscall.Read(d.fd, buf)
		if err == nil {
			return buf[:n], nil
		}
		if err != syscall.EAGAIN {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (d *linuxDevice) Close() error { return d.f.Close() }
