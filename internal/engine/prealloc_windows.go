//go:build windows

package engine

import (
	"os"

	"golang.org/x/sys/windows"
)

// setSparse marks a new file as sparse (NTFS) so that extending it with
// Truncate does not zero-fill the whole file synchronously: writes far from
// the start (the other segments) can begin immediately. Fails silently on
// file systems without sparse support (FAT32, exFAT).
func setSparse(f *os.File) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ioErr error
	err = rc.Control(func(fd uintptr) {
		var ret uint32
		ioErr = windows.DeviceIoControl(windows.Handle(fd), windows.FSCTL_SET_SPARSE, nil, 0, nil, 0, &ret, nil)
	})
	if err != nil {
		return err
	}
	return ioErr
}
