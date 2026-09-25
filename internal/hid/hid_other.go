//go:build !windows && !linux

package hid

import "errors"

var errUnsupported = errors.New("this system is not supported: only Windows and Linux")

func Enumerate(vid, pid uint16) ([]Interface, error) { return nil, errUnsupported }
func Open(path string) (Device, error)               { return nil, errUnsupported }
func OpenReader(path string) (Device, error)         { return nil, errUnsupported }
