//go:build windows

package files

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// 8.3 short names: the request name looks harmless, the resolved name is not.
// D: has 8dot3 disabled on the dev machine; run with TMP on C: to exercise it.
func TestDenylistWindowsShortName(t *testing.T) {
	w := newWorld(t)
	long := filepath.Join(w.root, ".env.production.local")
	write(t, long, "SENSITIVE")
	p16, err := syscall.UTF16PtrFromString(long)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 512)
	n, err := syscall.GetShortPathName(p16, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("no 8.3 name available: %v", err)
	}
	short := syscall.UTF16ToString(buf[:n])
	if strings.EqualFold(short, long) {
		t.Skip("8.3 name generation is disabled on this volume")
	}
	t.Logf("short name: %s", filepath.Base(short))
	g := w.guard(t, 100)
	_, err = g.Read(short)
	wantReason(t, err, ReasonDenylisted, "short name")
}
