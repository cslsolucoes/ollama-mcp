//go:build windows

package files

import (
	"os"
	"syscall"
)

// linkCount returns the number of hard links of the open file.
func linkCount(f *os.File) (uint32, error) {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return 0, err
	}
	return d.NumberOfLinks, nil
}
