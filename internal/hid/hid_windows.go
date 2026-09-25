package hid

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	modHid      = syscall.NewLazyDLL("hid.dll")
	modCfgmgr32 = syscall.NewLazyDLL("cfgmgr32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreateEventW        = modKernel32.NewProc("CreateEventW")
	procGetOverlappedResult = modKernel32.NewProc("GetOverlappedResult")

	procHidDGetHidGuid         = modHid.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes      = modHid.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData   = modHid.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData  = modHid.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps            = modHid.NewProc("HidP_GetCaps")
	procHidDFlushQueue         = modHid.NewProc("HidD_FlushQueue")
	procCMGetInterfaceListSize = modCfgmgr32.NewProc("CM_Get_Device_Interface_List_SizeW")
	procCMGetInterfaceList     = modCfgmgr32.NewProc("CM_Get_Device_Interface_ListW")
)

const (
	hidpStatusSuccess     = 0x00110000
	fileFlagOverlapped    = 0x40000000
	errorIOPending        = syscall.Errno(997)
	errorOperationAbort   = syscall.Errno(995)
	errorSharingViolation = syscall.Errno(32)
	waitTimeout           = 0x102
)

type hiddAttributes struct {
	Size      uint32
	VendorID  uint16
	ProductID uint16
	Version   uint16
}

type hidpCaps struct {
	Usage, UsagePage                                                   uint16
	InputReportByteLength, OutputReportByteLength, FeatureReportLength uint16
	Reserved                                                           [17]uint16
	Counts                                                             [10]uint16
}

var miRe = regexp.MustCompile(`(?i)&mi_([0-9a-f]{2})`)

// Enumerate lists the present HID collections of one USB device.
func Enumerate(vid, pid uint16) ([]Interface, error) {
	var guid syscall.GUID
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&guid)))

	var size uint32
	if r, _, _ := procCMGetInterfaceListSize.Call(uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&guid)), 0, 0); r != 0 {
		return nil, fmt.Errorf("CM_Get_Device_Interface_List_Size failed: %d", r)
	}
	buf := make([]uint16, size)
	if size > 0 {
		if r, _, _ := procCMGetInterfaceList.Call(uintptr(unsafe.Pointer(&guid)), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(size), 0); r != 0 {
			return nil, fmt.Errorf("CM_Get_Device_Interface_List failed: %d", r)
		}
	}

	want := fmt.Sprintf("vid_%04x&pid_%04x", vid, pid)
	var out []Interface
	for _, path := range splitMultiSz(buf) {
		if !strings.Contains(strings.ToLower(path), want) {
			continue
		}
		info, err := describe(path)
		if err != nil {
			continue // collection vanished or refused a plain query: not ours to use
		}
		if info.VendorID == vid && info.ProductID == pid {
			out = append(out, info)
		}
	}
	return out, nil
}

func splitMultiSz(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c == 0 {
			if i == start {
				break
			}
			out = append(out, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	return out
}

func describe(path string) (Interface, error) {
	// Access 0 = query only: works even on keyboards/mice that Windows holds exclusively.
	h, err := createFile(path, 0, shareAll, 0)
	if err != nil {
		return Interface{}, err
	}
	defer syscall.CloseHandle(h)

	info := Interface{Path: path, Interface: -1}
	attrs := hiddAttributes{Size: uint32(unsafe.Sizeof(hiddAttributes{}))}
	if r, _, _ := procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attrs))); r == 0 {
		return info, errors.New("HidD_GetAttributes failed")
	}
	info.VendorID, info.ProductID = attrs.VendorID, attrs.ProductID

	var pd uintptr
	if r, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&pd))); r != 0 {
		var caps hidpCaps
		if r, _, _ := procHidPGetCaps.Call(pd, uintptr(unsafe.Pointer(&caps))); r == hidpStatusSuccess {
			info.UsagePage, info.Usage = caps.UsagePage, caps.Usage
		}
		procHidDFreePreparsedData.Call(pd)
	}
	if m := miRe.FindStringSubmatch(path); m != nil {
		n, _ := strconv.ParseUint(m[1], 16, 8)
		info.Interface = int(n)
	}
	return info, nil
}

const shareAll = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE

func createFile(path string, access, share, flags uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(p, access, share, nil, syscall.OPEN_EXISTING, flags, 0)
	switch err {
	case nil:
		return h, nil
	case errorSharingViolation:
		return 0, ErrBusy
	case syscall.ERROR_ACCESS_DENIED:
		if share == 0 {
			return 0, ErrBusy // another handle is open, e.g. the Vaydeer app
		}
		return 0, ErrPermission
	}
	return 0, err
}

type winDevice struct {
	h     syscall.Handle
	event syscall.Handle
}

// Open opens a HID collection exclusively for reading and writing reports.
func Open(path string) (Device, error) {
	return open(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0)
}

// OpenReader opens a HID collection read-only, shared with other programs.
func OpenReader(path string) (Device, error) {
	return open(path, syscall.GENERIC_READ, shareAll)
}

func open(path string, access, share uint32) (Device, error) {
	h, err := createFile(path, access, share, fileFlagOverlapped)
	if err != nil {
		return nil, err
	}
	r, _, err := procCreateEventW.Call(0, 1, 0, 0) // manual reset, not signaled
	if r == 0 {
		syscall.CloseHandle(h)
		return nil, err
	}
	ev := syscall.Handle(r)
	procHidDFlushQueue.Call(uintptr(h)) // drop reports queued before we arrived
	return &winDevice{h: h, event: ev}, nil
}

// io runs one overlapped read or write and waits up to timeout (0 = forever).
func (d *winDevice) io(write bool, buf []byte, timeout time.Duration) (int, error) {
	ov := new(syscall.Overlapped)
	ov.HEvent = d.event
	var done uint32
	var err error
	if write {
		err = syscall.WriteFile(d.h, buf, &done, ov)
	} else {
		err = syscall.ReadFile(d.h, buf, &done, ov)
	}
	if err != nil && err != errorIOPending {
		return 0, err
	}
	ms := uint32(syscall.INFINITE)
	if timeout > 0 {
		ms = uint32(timeout / time.Millisecond)
	}
	if ev, _ := syscall.WaitForSingleObject(d.event, ms); ev == waitTimeout {
		syscall.CancelIoEx(d.h, ov)
		// Wait for the cancel to land: the kernel must be done with buf and ov before we return.
		// If the I/O finished just before the cancel, keep its data instead of dropping it.
		if err := overlappedResult(d.h, ov, &done); err == nil && done > 0 {
			return int(done), nil
		}
		return 0, ErrTimeout
	}
	if err := overlappedResult(d.h, ov, &done); err != nil {
		if err == errorOperationAbort {
			return 0, ErrTimeout
		}
		return 0, err
	}
	return int(done), nil
}

func overlappedResult(h syscall.Handle, ov *syscall.Overlapped, done *uint32) error {
	r, _, err := procGetOverlappedResult.Call(uintptr(h), uintptr(unsafe.Pointer(ov)), uintptr(unsafe.Pointer(done)), 1)
	if r == 0 {
		return err
	}
	return nil
}

func (d *winDevice) Write(report []byte) error {
	_, err := d.io(true, report, 2*time.Second)
	return err
}

func (d *winDevice) Read(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, 65)
	n, err := d.io(false, buf, timeout)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrTimeout
	}
	return buf[1:n], nil // Windows always hands back the report ID first
}

func (d *winDevice) Close() error {
	syscall.CloseHandle(d.event)
	return syscall.CloseHandle(d.h)
}
