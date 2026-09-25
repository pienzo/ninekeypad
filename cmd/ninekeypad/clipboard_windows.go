package main

import (
	"errors"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory    = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// copyToClipboard puts text on the Windows clipboard directly, without a helper program.
func copyToClipboard(text string) error {
	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	opened := false
	for i := 0; i < 10 && !opened; i++ { // another program may hold the clipboard for a moment
		r, _, _ := procOpenClipboard.Call(0)
		if opened = r != 0; !opened {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !opened {
		return errors.New("the clipboard is busy")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	size := uintptr(len(u16) * 2)
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return errors.New("out of memory")
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return errors.New("clipboard memory could not be locked")
	}
	procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&u16[0])), size)
	procGlobalUnlock.Call(h)
	if r, _, _ := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return errors.New("the clipboard did not take the text")
	}
	return nil // the clipboard owns h now
}
