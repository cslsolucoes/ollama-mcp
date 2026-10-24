//go:build windows

package files

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// 8.3 names on a volume with 8dot3 enabled (C:). D: (TEMP) has it disabled.
func TestAttackDenylistShortNamesOnC(t *testing.T) {
	winOnly(t)
	base, err := os.MkdirTemp(`C:\Users\Public`, "advrev")
	if err != nil {
		t.Skipf("C:\\Users\\Public not writable: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	w := world{base: base, root: filepath.Join(base, "root"), outside: filepath.Join(base, "outside"), sibling: filepath.Join(base, "root-other")}
	for _, d := range []string{w.root, w.outside} {
		os.MkdirAll(d, 0o755)
	}
	write(t, filepath.Join(w.outside, "x.txt"), outsideContent)
	targets := []string{
		".env.production.local",
		"credentials.json",
		"mysecretfile.txt",
		"id_ed25519_github",
		filepath.Join(".git", "config"),
		filepath.Join("secretsdir", "plain.txt"),
		filepath.Join("credentialstore", "plain.txt"),
		"settings.local.json",
		"certificate.pem",
	}
	for _, n := range targets {
		write(t, filepath.Join(w.root, n), secretContent)
	}
	g := w.guard(t, 1<<20)
	short := func(p string) string {
		p16, _ := syscall.UTF16PtrFromString(p)
		buf := make([]uint16, 1024)
		n, err := syscall.GetShortPathName(p16, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			return ""
		}
		return syscall.UTF16ToString(buf[:n])
	}
	any83 := false
	for _, n := range targets {
		long := filepath.Join(w.root, n)
		s := short(long)
		if s == "" || strings.EqualFold(s, long) {
			t.Logf("VEC|short/%s|NAO-TESTAVEL|sem nome 8.3", n)
			continue
		}
		any83 = true
		rel := strings.TrimPrefix(s, short(w.root)+`\`)
		t.Logf("short name of %s = %s", n, rel)
		attempt(t, w, g, "short/abs/"+n, s)
		attempt(t, w, g, "short/rel/"+n, rel)
	}
	if !any83 {
		t.Log("no 8.3 names generated")
	}
}

