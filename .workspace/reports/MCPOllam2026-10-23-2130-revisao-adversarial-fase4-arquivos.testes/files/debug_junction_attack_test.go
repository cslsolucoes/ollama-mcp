package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttackDebugRootJunction(t *testing.T) {
	winOnly(t)
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "A")
	jr := filepath.Join(w.base, "jroot")
	if !mklink(t, "/J", jr, w.root) {
		t.Skip()
	}
	ev, err := filepath.EvalSymlinks(filepath.Join(jr, "a.txt"))
	t.Logf("EvalSymlinks(jroot\\a.txt) = %q err=%v", ev, err)
	ev2, err := filepath.EvalSymlinks(jr)
	t.Logf("EvalSymlinks(jroot) = %q err=%v", ev2, err)
	st, err := os.Lstat(jr)
	t.Logf("Lstat(jroot) mode=%v err=%v", st.Mode(), err)
	rt, err := os.OpenRoot(jr)
	t.Logf("OpenRoot(jroot) err=%v", err)
	if err == nil {
		_, err = rt.Stat("a.txt")
		t.Logf("rt.Stat(a.txt) err=%v", err)
		rt.Close()
	}
	// junction ABOVE the root: root = jparent\root where jparent -> base
	jp := filepath.Join(os.TempDir(), "advrev-jparent")
	os.Remove(jp)
	if mklink(t, "/J", jp, w.base) {
		defer os.Remove(jp)
		g, err := NewGuard([]string{filepath.Join(jp, "root")}, 100)
		t.Logf("NewGuard(jparent\\root) err=%v roots=%v", err, g.Roots())
		if err == nil {
			_, err = g.Read("a.txt")
			t.Logf("Read(a.txt) via junction-above-root err=%v", err)
			_, err = g.Read(`..\outside\x.txt`)
			t.Logf("Read(..\\outside) via junction-above-root err=%v", err)
		}
	}
}
