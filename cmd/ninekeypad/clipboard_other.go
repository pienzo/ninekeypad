//go:build !windows && !linux

package main

import "errors"

func copyToClipboard(string) error { return errors.New("not supported on this system") }
