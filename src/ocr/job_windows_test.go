//go:build windows

package ocr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procIsProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func inJob(t *testing.T, pid int, job windows.Handle) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	var in int32
	r, _, e := procIsProcessInJob.Call(uintptr(h), uintptr(job), uintptr(unsafe.Pointer(&in)))
	if r == 0 {
		t.Fatal(e)
	}
	return in != 0
}

func TestJobWorkerIsInsideJob(t *testing.T) {
	p, log := newTestPool(t, "fake", nil)
	if _, err := p.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := log.list()[0]
	if !inJob(t, c.cmd.Process.Pid, p.job.h) {
		t.Fatal("worker is not in the pool's Job Object")
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(p.job.h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		t.Fatal(err)
	}
	want := uint32(windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION)
	if info.BasicLimitInformation.LimitFlags&want != want || info.ProcessMemoryLimit != uintptr(DefaultMaxMem) {
		t.Fatalf("job limits flags=%#x mem=%d", info.BasicLimitInformation.LimitFlags, info.ProcessMemoryLimit)
	}
}

func TestJobKillOnClose(t *testing.T) {
	p, log := newTestPool(t, "fake", nil)
	if _, err := p.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := log.list()[0]
	// Only the job handle is closed (as when the server process dies): the
	// worker, idle and blocked on its stdin, must be ended by the job.
	p.job.close()
	waitExited(t, c)
}

func TestJobMemoryCeiling(t *testing.T) {
	if raceEnabled {
		t.Skip("the race runtime commits its own shadow memory; ceiling measured without -race")
	}
	p, log := newTestPool(t, "alloc", func(c *Config) { c.MaxMem = 128 << 20 })
	_, err := p.Recognize(context.Background(), testImage(4, 4), 0)
	var pe *PageError
	if !errors.As(err, &pe) || !errors.Is(err, ErrEngine) || !strings.Contains(err.Error(), "commit recusado") {
		t.Fatalf("512 MiB under a 128 MiB ceiling: err = %v", err)
	}
	if n := len(log.list()); n != 1 {
		t.Fatalf("%d workers, want 1 (an engine error keeps the worker)", n)
	}
	// Control: the same commit fits under the default 1 GiB.
	p2, _ := newTestPool(t, "alloc", nil)
	if _, err := p2.Recognize(context.Background(), testImage(4, 4), 0); err != nil {
		t.Fatalf("512 MiB under 1 GiB: %v", err)
	}
}

// TestJobGoHeapExhaustion documents what happens when the Go heap of a
// worker hits the ceiling: the runtime reports "out of memory" (VirtualAlloc
// errno 1455) and then hangs instead of exiting, so the page timeout is what
// ends the worker. The page fails; the server is not affected.
func TestJobGoHeapExhaustion(t *testing.T) {
	if raceEnabled {
		t.Skip("the race runtime commits its own shadow memory")
	}
	p, log := newTestPool(t, "go-alloc", func(c *Config) { c.MaxMem = 128 << 20; c.PageTimeout = 8 * time.Second })
	_, err := p.Recognize(context.Background(), testImage(4, 4), 0)
	if err == nil {
		t.Fatal("512 MiB of Go heap under a 128 MiB ceiling succeeded")
	}
	kids := log.list()
	if len(kids) == 0 {
		t.Fatal("no worker started")
	}
	for _, c := range kids {
		waitExited(t, c)
	}
	if !strings.Contains(kids[0].stderr.String(), "1455") {
		t.Fatalf("worker stderr does not show the refused commit (err = %v)", err)
	}
	t.Logf("Go heap past the ceiling: %v", err)
}
