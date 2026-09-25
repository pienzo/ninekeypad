// Package hid talks to USB HID interfaces with nothing but the operating system:
// hid.dll/cfgmgr32.dll on Windows, /sys and /dev/hidraw on Linux. No cgo, no libraries.
package hid

import (
	"errors"
	"time"
)

// Interface is one HID collection of a USB device.
type Interface struct {
	Path      string // what Open takes
	VendorID  uint16
	ProductID uint16
	Interface int // USB interface number (MI_xx), -1 when unknown
	UsagePage uint16
	Usage     uint16
}

// Device is an open HID interface.
type Device interface {
	// Write sends one output report. report[0] is the report ID (0 for this keypad).
	Write(report []byte) error
	// Read returns one input report WITHOUT the report ID byte, or ErrTimeout.
	Read(timeout time.Duration) ([]byte, error)
	Close() error
}

var (
	ErrTimeout    = errors.New("no answer from the keypad in time")
	ErrPermission = errors.New("no permission to open the keypad")
	ErrBusy       = errors.New("the keypad is in use by another program")
)

// Two ways to open an interface:
//   Open        read + write, EXCLUSIVE: no other program can talk to it meanwhile, so no
//               other program's answers or commands can get mixed into ours (ErrBusy if taken).
//   OpenReader  read only, shared: for listening to key events next to other programs.
