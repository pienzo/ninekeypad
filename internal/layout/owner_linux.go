package layout

import (
	"os"
	"strconv"
)

// fixOwner gives a file or folder the program made under "sudo" to the user who ran sudo,
// so the backups stay theirs and not root's. It is only called for things the program
// created itself. Errors are ignored: FAT32/exFAT sticks have no owners at all.
func fixOwner(path string) {
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err1 != nil || err2 != nil || os.Geteuid() != 0 {
		return
	}
	os.Lchown(path, uid, gid)
}

// syncFolder flushes a rename to the disk, so a file is really there after a power cut.
func syncFolder(folder string) {
	if d, err := os.Open(folder); err == nil {
		d.Sync()
		d.Close()
	}
}
