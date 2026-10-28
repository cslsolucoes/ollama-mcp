//go:build windows

package ocr

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// emitNoise writes to os.Stdout, to the Win32 standard output handle and to
// descriptor 1 and the stdout FILE of msvcrt.dll.
func emitNoise() {
	fmt.Println("NOISE os.Stdout")
	if h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		var n uint32
		msg := []byte("NOISE win32\n")
		_ = windows.WriteFile(h, msg, &n, nil)
	}
	crt := windows.NewLazySystemDLL("msvcrt.dll")
	msg := []byte("NOISE crt write\n")
	crt.NewProc("_write").Call(1, uintptr(unsafe.Pointer(&msg[0])), uintptr(len(msg)))
	s := []byte("NOISE crt puts\x00")
	crt.NewProc("puts").Call(uintptr(unsafe.Pointer(&s[0])))
	crt.NewProc("fflush").Call(0)
	_ = os.Stdout.Sync()
}

// commitMemory commits and touches n bytes with VirtualAlloc (never freed:
// the worker process is short-lived).
func commitMemory(n uintptr) error {
	addr, err := windows.VirtualAlloc(0, n, windows.MEM_RESERVE|windows.MEM_COMMIT, windows.PAGE_READWRITE)
	if err != nil {
		return fmt.Errorf("commit recusado: %w", err)
	}
	base := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	for i := uintptr(0); i < n; i += 4096 {
		*(*byte)(unsafe.Add(base, i)) = 1
	}
	return nil
}
