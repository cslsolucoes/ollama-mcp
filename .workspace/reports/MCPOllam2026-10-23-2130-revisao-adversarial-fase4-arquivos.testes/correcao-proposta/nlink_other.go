//go:build !windows

package files

import (
	"errors"
	"os"
	"syscall"
)

func linkCount(f *os.File) (uint32, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("no stat_t")
	}
	return uint32(s.Nlink), nil
}
