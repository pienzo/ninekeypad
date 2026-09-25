//go:build !linux

package layout

func fixOwner(string) {}

// syncFolder: Windows flushes the rename with the file; a folder cannot be opened for sync.
func syncFolder(string) {}
