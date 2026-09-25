package hid

import "testing"

func TestParseHIDID(t *testing.T) {
	uevent := "DRIVER=hid-generic\nHID_ID=0003:00000483:00005752\nHID_NAME=Vaydeer 9-key Smart Keypad\nHID_PHYS=usb-0000:00:14.0-1/input0\n"
	v, p, ok := ParseHIDID(uevent)
	if !ok || v != 0x0483 || p != 0x5752 {
		t.Fatalf("got %04x:%04x ok=%v", v, p, ok)
	}
	if _, _, ok := ParseHIDID("DRIVER=hid-generic\n"); ok {
		t.Fatal("parsed a uevent without HID_ID")
	}
}

func TestParseInterfaceNumber(t *testing.T) {
	cases := map[string]int{
		"/sys/devices/pci0000:00/0000:00:14.0/usb1/1-1/1-1:1.0/0003:0483:5752.0001":         0,
		"/sys/devices/pci0000:00/0000:00:14.0/usb1/1-1/1-1:1.2/0003:0483:5752.0003":         2,
		"/sys/devices/pci0000:00/0000:00:14.0/usb1/1-1/1-1.4/1-1.4:1.3/0003:0483:5752.0007": 3,
		"/sys/devices/virtual/misc/uhid/0003:0483:5752.0009":                                -1,
	}
	for path, want := range cases {
		if got := ParseInterfaceNumber(path); got != want {
			t.Errorf("%s: got %d, want %d", path, got, want)
		}
	}
}

func TestParseUsage(t *testing.T) {
	cases := []struct {
		name        string
		desc        []byte
		page, usage uint16
	}{
		// Vendor command collection: Usage Page (0xFF00), Usage (0x01), Collection (Application) ...
		{"vendor command", []byte{0x06, 0x00, 0xFF, 0x09, 0x01, 0xA1, 0x01, 0x15, 0x00, 0x26, 0xFF, 0x00}, 0xFF00, 0x01},
		{"vendor events", []byte{0x06, 0x00, 0xFF, 0x09, 0x02, 0xA1, 0x01}, 0xFF00, 0x02},
		// Keyboard: Usage Page (Generic Desktop), Usage (Keyboard)
		{"keyboard", []byte{0x05, 0x01, 0x09, 0x06, 0xA1, 0x01}, 0x0001, 0x06},
		// Leading long item is skipped
		{"long item first", []byte{0xFE, 0x02, 0x10, 0xAA, 0xBB, 0x06, 0x00, 0xFF, 0x09, 0x01}, 0xFF00, 0x01},
		{"truncated", []byte{0x06, 0x00}, 0, 0},
	}
	for _, c := range cases {
		page, usage := ParseUsage(c.desc)
		if page != c.page || usage != c.usage {
			t.Errorf("%s: got %04x/%02x, want %04x/%02x", c.name, page, usage, c.page, c.usage)
		}
	}
}
