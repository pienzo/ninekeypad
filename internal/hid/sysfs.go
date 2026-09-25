package hid

import (
	"regexp"
	"strconv"
)

// Parsers for Linux sysfs text. They live outside hid_linux.go so the tests run on Windows too.

var (
	hidIDRe = regexp.MustCompile(`HID_ID=\w+:([0-9A-Fa-f]{8}):([0-9A-Fa-f]{8})`)
	ifaceRe = regexp.MustCompile(`:1\.(\d+)(?:/|$)`)
)

// ParseHIDID reads vendor and product from a sysfs HID uevent ("HID_ID=0003:00000483:00005752").
func ParseHIDID(uevent string) (vid, pid uint16, ok bool) {
	m := hidIDRe.FindStringSubmatch(uevent)
	if m == nil {
		return 0, 0, false
	}
	v, err1 := strconv.ParseUint(m[1], 16, 32)
	p, err2 := strconv.ParseUint(m[2], 16, 32)
	if err1 != nil || err2 != nil || v > 0xFFFF || p > 0xFFFF {
		return 0, 0, false
	}
	return uint16(v), uint16(p), true
}

// ParseInterfaceNumber takes the USB interface number from a resolved sysfs device path
// (".../1-2:1.0/0003:0483:5752.0004" -> 0), or -1.
func ParseInterfaceNumber(sysfsPath string) int {
	m := ifaceRe.FindStringSubmatch(sysfsPath)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return n
}
