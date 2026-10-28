//go:build windows

package ocr

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errStdinConsole = errors.New("ocr: o modo --ocr-worker é interno e não aceita terminal na entrada")

// WorkerStdio prepares the standard streams of a worker process and returns
// the frame channel: in is the original stdin, out the original stdout. The
// process standard output is then pointed at NUL three times over, so that
// nothing printed by native code can reach the frames: the Win32 standard
// handle (SetStdHandle), the C runtime descriptor 1 of msvcrt.dll (the CRT
// libtesseract64.dll imports) and os.Stdout. Standard error is left alone
// (the pool keeps a small buffer of it for diagnosis only). A console on
// stdin is refused: the mode is internal and only the pool starts it.
func WorkerStdio() (in io.Reader, out io.Writer, err error) {
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil {
		return nil, nil, errStdinConsole
	}
	frames := os.Stdout
	nul, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("ocr: não foi possível abrir NUL: %w", err)
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(nul.Fd())); err != nil {
		return nil, nil, fmt.Errorf("ocr: não foi possível desviar a saída padrão: %w", err)
	}
	if err := redirectCRTStdout(); err != nil {
		return nil, nil, err
	}
	os.Stdout = nul
	return os.Stdin, frames, nil
}

// redirectCRTStdout points descriptor 1 of the system msvcrt.dll at NUL. The
// CRT may have been initialized before SetStdHandle (other system DLLs load
// it), in which case its descriptor 1 still holds the pipe.
func redirectCRTStdout() error {
	crt, err := windows.LoadLibraryEx("msvcrt.dll", 0, windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return fmt.Errorf("ocr: msvcrt.dll indisponível: %w", err)
	}
	// Kept loaded for the life of the process (the DLL imports it anyway).
	open, err1 := windows.GetProcAddress(crt, "_open")
	dup2, err2 := windows.GetProcAddress(crt, "_dup2")
	closeFn, err3 := windows.GetProcAddress(crt, "_close")
	if err1 != nil || err2 != nil || err3 != nil {
		return errors.New("ocr: msvcrt.dll sem _open/_dup2/_close")
	}
	name := cBytes("NUL")
	const oWronly = 1
	fd, _, _ := syscall.SyscallN(open, uintptr(unsafe.Pointer(&name[0])), oWronly)
	if int32(fd) < 0 {
		return errors.New("ocr: _open(NUL) falhou")
	}
	r, _, _ := syscall.SyscallN(dup2, fd, 1)
	syscall.SyscallN(closeFn, fd)
	if int32(r) != 0 {
		return errors.New("ocr: _dup2(NUL, 1) falhou")
	}
	return nil
}
