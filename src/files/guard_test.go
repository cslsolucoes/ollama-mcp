package files

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// world is a sandbox: <base>/root is the allowed directory, <base>/outside
// holds a file the guard must never read, <base>/root-other shares the prefix
// of the root name. The directory is made by hand, not t.TempDir, because
// t.TempDir embeds the test name in the path and a name such as
// TestSecret... would trip the denylist on the part above the root.
type world struct {
	base, root, outside, sibling string
}

const outsideContent = "OUTSIDE-CONTENT-MUST-NOT-LEAK"

func newWorld(t *testing.T) world {
	t.Helper()
	base, err := os.MkdirTemp("", "gd")
	if err != nil {
		t.Fatal(err)
	}
	base, _ = filepath.EvalSymlinks(base)
	t.Cleanup(func() { os.RemoveAll(base) })
	w := world{base: base, root: filepath.Join(base, "root"), outside: filepath.Join(base, "outside"), sibling: filepath.Join(base, "root-other")}
	for _, d := range []string{w.root, w.outside, w.sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(w.outside, "x.txt"), outsideContent)
	write(t, filepath.Join(w.sibling, "f.txt"), outsideContent)
	return w
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w world) guard(t *testing.T, max int64, roots ...string) *Guard {
	t.Helper()
	if len(roots) == 0 {
		roots = []string{w.root}
	}
	g, err := NewGuard(roots, max)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func wantReason(t *testing.T, err error, want Reason, ctx string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("%s: want refusal %d, got %v", ctx, want, err)
	}
	if r.Reason != want {
		t.Fatalf("%s: reason = %d (%v), want %d", ctx, r.Reason, err, want)
	}
}

// noLeak checks the user-facing message carries neither a path nor content.
func noLeak(t *testing.T, w world, err error) {
	t.Helper()
	m := err.Error()
	if strings.Contains(m, w.base) || strings.Contains(m, outsideContent) || strings.Contains(m, filepath.Base(w.base)) {
		t.Fatalf("message leaks a path or content: %q", m)
	}
}

func TestZeroGuardRefusesEverything(t *testing.T) {
	var g *Guard
	if g.Enabled() {
		t.Fatal("nil guard enabled")
	}
	if _, err := g.Read("x"); err == nil {
		t.Fatal("nil guard read a file")
	}
	g2 := &Guard{}
	if _, err := g2.Read("x"); err == nil || g2.Enabled() {
		t.Fatal("zero guard must refuse")
	}
}

// TEST-009: root that does not exist in the flag.
func TestNewGuardValidatesRoots(t *testing.T) {
	w := newWorld(t)
	if _, err := NewGuard([]string{filepath.Join(w.base, "nope")}, 10); err == nil || !strings.Contains(err.Error(), "--allow-root") {
		t.Fatalf("missing root: %v", err)
	}
	write(t, filepath.Join(w.root, "f.txt"), "x")
	if _, err := NewGuard([]string{filepath.Join(w.root, "f.txt")}, 10); err == nil || !strings.Contains(err.Error(), "não é um diretório") {
		t.Fatalf("file as root: %v", err)
	}
	if _, err := NewGuard([]string{w.root}, 0); err == nil {
		t.Fatal("max bytes 0 must be an error, never 'unlimited'")
	}
	if _, err := NewGuard([]string{`\\host\share`}, 10); err == nil {
		t.Fatal("UNC root accepted")
	}
	g, err := NewGuard([]string{"", "  "}, 10)
	if err != nil || g.Enabled() {
		t.Fatalf("blank roots must give a disabled guard: %v", err)
	}
}

func TestSplitRoots(t *testing.T) {
	got := SplitRoots(` a , ,b,, c `)
	if strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("%q", got)
	}
	if len(SplitRoots("")) != 0 {
		t.Fatal("empty value")
	}
}

func TestReadOKAbsoluteRelativeAndNormalisation(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "dir", "a.txt"), "linha1\r\nlinha2\r\n")
	if err := os.WriteFile(filepath.Join(w.root, "bom.txt"), append([]byte{0xEF, 0xBB, 0xBF}, "olá\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	g := w.guard(t, 1<<20)
	f, err := g.Read(filepath.Join(w.root, "dir", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Text != "linha1\nlinha2\n" || f.Name != "a.txt" || f.Bytes != 16 {
		t.Fatalf("%+v", f)
	}
	f, err = g.Read(filepath.Join("dir", "a.txt")) // relative, one root (FILE-002)
	if err != nil || f.Text != "linha1\nlinha2\n" {
		t.Fatalf("relative: %v %+v", err, f)
	}
	f, err = g.Read("bom.txt")
	if err != nil || f.Text != "olá\n" { // BOM dropped (FILE-017)
		t.Fatalf("bom: %v %q", err, f.Text)
	}
}

func TestRelativeNeedsExactlyOneRoot(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "x")
	g := w.guard(t, 100, w.root, w.outside)
	_, err := g.Read("a.txt")
	wantReason(t, err, ReasonBadRequest, "relative with two roots")
	if _, err := g.Read(filepath.Join(w.root, "a.txt")); err != nil {
		t.Fatalf("absolute with two roots: %v", err)
	}
	if _, err := g.Read(filepath.Join(w.outside, "x.txt")); err != nil {
		t.Fatalf("second root: %v", err)
	}
}

// TEST-009 / FILE-003: traversal, absolute outside, both separators, case.
func TestTraversalAndOutside(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "ok.txt"), "x")
	g := w.guard(t, 1<<20)
	sep := string(filepath.Separator)
	other := "/"
	if sep == "/" {
		other = `\`
	}
	cases := map[string]string{
		"dotdot relative":     filepath.Join("..", "outside", "x.txt"),
		"dotdot slash":        "../outside/x.txt",
		"dotdot backslash":    `..\outside\x.txt`,
		"dotdot deep":         "a/../../outside/x.txt",
		"absolute outside":    filepath.Join(w.outside, "x.txt"),
		"absolute via dotdot": w.root + sep + ".." + sep + "outside" + sep + "x.txt",
		"sibling prefix":      filepath.Join(w.sibling, "f.txt"),
		"nonexistent":         "nao-existe.txt",
		"root parent":         w.base,
	}
	if runtime.GOOS == "windows" {
		cases["absolute forward slashes"] = strings.ReplaceAll(filepath.Join(w.outside, "x.txt"), `\`, "/")
		cases["upper case outside"] = strings.ToUpper(filepath.Join(w.outside, "x.txt"))
	}
	_ = other
	var first string
	for name, p := range cases {
		_, err := g.Read(p)
		if err == nil {
			t.Fatalf("%s: read %q", name, p)
		}
		wantReason(t, err, ReasonNotAllowed, name)
		noLeak(t, w, err)
		if first == "" {
			first = err.Error()
		}
		// FILE-014: outside the roots and inexistent share one message.
		if err.Error() != first {
			t.Fatalf("%s: message differs from the others: %q vs %q", name, err.Error(), first)
		}
	}
}

// FILE-003: segment comparison; case-insensitive on Windows.
func TestWithin(t *testing.T) {
	sep := string(filepath.Separator)
	vol := ""
	if runtime.GOOS == "windows" {
		vol = `C:`
	}
	root := vol + sep + "dados"
	cases := []struct {
		p    string
		ok   bool
		rel  string
		name string
	}{
		{vol + sep + "dados" + sep + "a.txt", true, "a.txt", "inside"},
		{vol + sep + "dados", true, ".", "root itself"},
		{vol + sep + "dados-outro" + sep + "a.txt", false, "", "sibling with same prefix"},
		{vol + sep + "dadosX", false, "", "string prefix"},
		{vol + sep + "outro", false, "", "elsewhere"},
		{vol + sep + "dados" + sep + "..a", true, "..a", "name starting with two dots"},
		{vol + sep, false, "", "parent"},
	}
	for _, c := range cases {
		rel, ok := within(root, c.p)
		if ok != c.ok || (ok && rel != c.rel) {
			t.Errorf("%s: within(%q,%q) = %q,%v want %q,%v", c.name, root, c.p, rel, ok, c.rel, c.ok)
		}
	}
	if runtime.GOOS == "windows" {
		if _, ok := within(`C:\Dados`, `c:\DADOS\a.txt`); !ok {
			t.Error("windows containment must ignore case")
		}
		if _, ok := within(`C:\dados`, `D:\dados\a.txt`); ok {
			t.Error("another volume is not inside")
		}
	}
}

func TestRootWithSamePrefixName(t *testing.T) {
	w := newWorld(t)
	// root is <base>/root; <base>/root-other must be unreachable even by name.
	g := w.guard(t, 100)
	_, err := g.Read(filepath.Join(w.sibling, "f.txt"))
	wantReason(t, err, ReasonNotAllowed, "sibling")
}

// P-10: every denylist item, by requested name.
func TestDenylist(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 1<<20)
	names := []string{
		".env", ".env.local", ".env.production", ".ENV", "prod.env", "server.pem", "SERVER.PEM", "tls.key", "cert.pfx", "cert.p12",
		"id_rsa", "id_rsa.pub", "id_ed25519", "id_ed25519.pub", "id_ecdsa", ".mcp.json", "settings.local.json",
		"credentials.json", "my-credential.txt", "AWS_Credentials", "secret.txt", "client_secret.json", "TopSecret.md",
		filepath.Join(".git", "config"), filepath.Join(".git", "objects", "ab", "cdef"), filepath.Join("src", ".git", "HEAD"),
		filepath.Join("secrets", "readme.txt"), filepath.Join("venv", ".env", "lib", "x.py"),
	}
	for _, n := range names {
		write(t, filepath.Join(w.root, n), "SENSITIVE")
		_, err := g.Read(n)
		wantReason(t, err, ReasonDenylisted, n)
		if strings.Contains(err.Error(), "SENSITIVE") {
			t.Fatalf("%s: content in the message", n)
		}
		// The absolute form is denied the same way.
		_, err = g.Read(filepath.Join(w.root, n))
		wantReason(t, err, ReasonDenylisted, "abs "+n)
	}
	// A denied name is denied even when the file does not exist: the answer
	// never tells whether it exists.
	_, err := g.Read(".env.nonexistent")
	wantReason(t, err, ReasonDenylisted, "inexistent denied name")
	// Look-alikes stay readable.
	for _, n := range []string{"environment.txt", "keyboard.txt", "monkey.txt", "gitignore.txt", "notes.txt", "envelope.txt", "pem.txt"} {
		write(t, filepath.Join(w.root, n), "ok")
		if _, err := g.Read(n); err != nil {
			t.Errorf("%s must be readable: %v", n, err)
		}
	}
}

// A root whose own name is sensitive must still serve its other files.
func TestRootNameDoesNotTripDenylist(t *testing.T) {
	w := newWorld(t)
	root := filepath.Join(w.base, "my-secrets-project")
	write(t, filepath.Join(root, "a.txt"), "x")
	g := w.guard(t, 100, root)
	if _, err := g.Read("a.txt"); err != nil {
		t.Fatalf("file under a root called secrets: %v", err)
	}
}

func TestDenylistUnit(t *testing.T) {
	if Denied(nil) || Denied([]string{"", "a", "b.txt"}) {
		t.Fatal("false positive")
	}
	if !Denied([]string{"a", ".git", "b"}) || !Denied([]string{"x", "Id_RSA"}) {
		t.Fatal("false negative")
	}
	// Windows drops trailing dots and spaces: ".env." opens ".env".
	if !Denied([]string{".env."}) || !Denied([]string{".env  "}) || !Denied([]string{"secret.txt. "}) {
		t.Fatal("trailing dot/space bypass")
	}
}

func TestDenylistWindowsTrailingDotBypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("trailing dots are dropped only on Windows")
	}
	w := newWorld(t)
	write(t, filepath.Join(w.root, ".env"), "SENSITIVE")
	g := w.guard(t, 100)
	for _, n := range []string{".env.", ".env ", ".ENV"} {
		_, err := g.Read(n)
		wantReason(t, err, ReasonDenylisted, "request "+n)
	}
}

// TestDenylistWindowsShortName lives in guard_windows_test.go (it needs the
// Windows-only syscalls, so this file stays buildable on Linux).

// A hard link is a second name for the same file: neither the denylist nor the
// containment can see the other names, so any file with more than one link is
// refused (SEC-C-01 of the adversarial review of 2026-10-23).
func TestHardlinksAreRefused(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, ".env"), "SECRET-ENV-MUST-NOT-LEAK")
	write(t, filepath.Join(w.root, ".git", "config"), "SECRET-GITCFG-MUST-NOT-LEAK")
	write(t, filepath.Join(w.root, "plain.txt"), "plain")
	g := w.guard(t, 1<<20)
	made := 0
	for name, target := range map[string]string{
		"hl-env.txt":     filepath.Join(w.root, ".env"),
		"hl-gitcfg.txt":  filepath.Join(w.root, ".git", "config"),
		"hl-outside.txt": filepath.Join(w.outside, "x.txt"),
		"hl-plain.txt":   filepath.Join(w.root, "plain.txt"),
	} {
		if err := os.Link(target, filepath.Join(w.root, name)); err != nil {
			t.Logf("hard link %s not created: %v", name, err)
			continue
		}
		made++
		f, err := g.Read(name)
		if err == nil {
			t.Fatalf("%s: hard link was read (%d bytes)", name, len(f.Text))
		}
		wantReason(t, err, ReasonNotAllowed, name)
		noLeak(t, w, err)
	}
	if made == 0 {
		t.Skip("hard links are not available on this volume")
	}
	// The linked original is refused too while it has two names...
	if _, err := g.Read("plain.txt"); err == nil {
		t.Fatal("plain.txt has 2 links and must be refused")
	}
	// ...and readable again once it is back to a single name.
	if err := os.Remove(filepath.Join(w.root, "hl-plain.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Read("plain.txt"); err != nil {
		t.Fatalf("plain.txt with 1 link: %v", err)
	}
}

func TestSymlinks(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "real", "in.txt"), "INSIDE")
	write(t, filepath.Join(w.root, ".env"), "SENSITIVE")
	if err := os.Symlink(w.outside, filepath.Join(w.root, "to-outside")); err != nil {
		t.Skipf("cannot create symlinks here (%v): containment is covered by TestWithin and the junction tests", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.Symlink(filepath.Join(w.outside, "x.txt"), filepath.Join(w.root, "file-to-outside.txt")))
	must(os.Symlink(filepath.Join(w.root, "real"), filepath.Join(w.root, "to-real")))
	must(os.Symlink(filepath.Join(w.root, ".env"), filepath.Join(w.root, "innocent.txt")))
	g := w.guard(t, 1<<20)

	_, err := g.Read(filepath.Join("to-outside", "x.txt"))
	wantReason(t, err, ReasonNotAllowed, "dir symlink to outside")
	noLeak(t, w, err)
	_, err = g.Read("file-to-outside.txt")
	wantReason(t, err, ReasonNotAllowed, "file symlink to outside")
	noLeak(t, w, err)

	f, err := g.Read(filepath.Join("to-real", "in.txt")) // stays inside: allowed
	if err != nil || f.Text != "INSIDE" {
		t.Fatalf("symlink inside the root: %v", err)
	}
	// resolved-name denylist: innocent.txt -> .env
	_, err = g.Read("innocent.txt")
	wantReason(t, err, ReasonDenylisted, "symlink to a denied file")
}

// A root that is itself reached through a symlink resolves once; files below
// it work and the resolved root is what is compared.
func TestRootThroughSymlink(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "A")
	link := filepath.Join(w.base, "rootlink")
	if err := os.Symlink(w.root, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	g := w.guard(t, 100, link)
	if f, err := g.Read(filepath.Join(link, "a.txt")); err != nil || f.Text != "A" {
		t.Fatalf("via link: %v", err)
	}
	if f, err := g.Read(filepath.Join(w.root, "a.txt")); err != nil || f.Text != "A" {
		t.Fatalf("via real path: %v", err)
	}
}

func mklinkJunction(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows feature")
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("mklink /J failed: %v %s", err, out)
	}
}

// FILE-004: a junction that leaves the root is refused. Go's EvalSymlinks does
// not follow junctions on Windows; the guard fails closed on them, which is
// what the test pins (Lstat marks them ModeIrregular).
func TestJunctionToOutsideIsRefused(t *testing.T) {
	w := newWorld(t)
	mklinkJunction(t, filepath.Join(w.root, "jout"), w.outside)
	g := w.guard(t, 1<<20)
	_, err := g.Read(filepath.Join("jout", "x.txt"))
	wantReason(t, err, ReasonNotAllowed, "junction to outside")
	noLeak(t, w, err)
	_, err = g.Read(filepath.Join(w.root, "jout", "x.txt"))
	wantReason(t, err, ReasonNotAllowed, "junction to outside, absolute")
}

// A junction that points back inside the root is also refused: crossing a
// junction is never allowed (documented limitation, fail closed).
func TestJunctionInsideIsAlsoRefused(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "real", "in.txt"), "INSIDE")
	mklinkJunction(t, filepath.Join(w.root, "jin"), filepath.Join(w.root, "real"))
	g := w.guard(t, 1<<20)
	_, err := g.Read(filepath.Join("jin", "in.txt"))
	wantReason(t, err, ReasonNotAllowed, "junction inside")
	if f, err := g.Read(filepath.Join("real", "in.txt")); err != nil || f.Text != "INSIDE" {
		t.Fatalf("the real path must still work: %v", err)
	}
}

func TestWindowsSyntaxTable(t *testing.T) {
	refused := map[string]string{
		`\\host\share\f.txt`:         "UNC",
		`\\?\C:\data\f.txt`:          `\\?\`,
		`\\?\UNC\host\share\f.txt`:   `\\?\UNC`,
		`\\.\pipe\x`:                 `\\.\`,
		`\\.\C:\data\f.txt`:          `\\.\ disk`,
		`//host/share/f.txt`:         "UNC forward",
		`C:\data\f.txt:stream`:       "ADS",
		`C:\data\f.txt::$DATA`:       "ADS $DATA",
		`C:\data\f.txt:stream:$DATA`: "ADS typed",
		`f.txt:stream`:               "ADS relative",
		`C:\data:x\f.txt`:            "ADS on a directory",
		`1:\data\f.txt`:              "bad drive",
		`C:f.txt`:                    "drive relative",
		`C:`:                         "bare drive",
		`CON`:                        "CON",
		`con`:                        "con lower",
		`C:\data\NUL`:                "NUL",
		`C:\data\nul.txt`:            "NUL with extension",
		`C:\data\COM1`:               "COM1",
		`C:\data\COM1.txt`:           "COM1.txt",
		`C:\data\com9.log`:           "COM9",
		`C:\data\LPT1`:               "LPT1",
		`C:\data\lpt9.txt`:           "LPT9",
		`C:\data\AUX.md`:             "AUX",
		`C:\data\PRN.`:               "PRN with dot",
		`C:\data\sub\con\f.txt`:      "device as directory",
		`C:\data\CONIN$`:             "CONIN$",
		`C:\data\f*.txt`:             "wildcard",
		`C:\data\f?.txt`:             "question mark",
		`C:\data\f<.txt`:             "angle",
		"C:\\data\\f\x00.txt":        "NUL byte",
	}
	for p, name := range refused {
		rf := checkSyntax(p, true)
		if rf == nil || rf.Reason != ReasonSpecialPath {
			t.Errorf("%s: %q was not refused (%v)", name, p, rf)
		}
	}
	accepted := []string{
		`C:\data\f.txt`, `C:\data\console.txt`, `C:\data\com10.txt`, `C:\data\comma.txt`, `C:\data\aux2.txt`,
		`C:\data\prn-notes.txt`, `C:/data/f.txt`, `data\f.txt`, `\data\f.txt`, `c:\Data\Sub Dir\f.txt`, `C:\data\lpt.txt`,
	}
	for _, p := range accepted {
		if rf := checkSyntax(p, true); rf != nil {
			t.Errorf("%q wrongly refused: %v", p, rf)
		}
	}
	// On other systems only the NUL byte and the \\ prefix are refused.
	if checkSyntax(`/data/a:b/con`, false) != nil {
		t.Error("':' and device names are valid file names off Windows")
	}
	if checkSyntax(`\\host\x`, false) == nil {
		t.Error(`\\ prefix must be refused everywhere`)
	}
}

// The same cases through Read, on the real OS.
func TestWindowsSpecialPathsThroughRead(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	w := newWorld(t)
	write(t, filepath.Join(w.root, "f.txt"), "x")
	g := w.guard(t, 100)
	for _, p := range []string{
		`\\localhost\C$\Windows\win.ini`, `\\?\` + filepath.Join(w.root, "f.txt"), `\\.\` + filepath.Join(w.root, "f.txt"),
		filepath.Join(w.root, "f.txt") + ":stream", filepath.Join(w.root, "f.txt") + "::$DATA",
		filepath.Join(w.root, "CON"), filepath.Join(w.root, "NUL.txt"), "COM1", "LPT2.txt", `C:f.txt`,
	} {
		_, err := g.Read(p)
		wantReason(t, err, ReasonSpecialPath, p)
		noLeak(t, w, err)
	}
	// a real ADS on a real file is refused and the main stream stays readable
	if f, err := g.Read("f.txt"); err != nil || f.Text != "x" {
		t.Fatalf("control read: %v", err)
	}
}

func TestDirectoryAndSpecialFiles(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "sub", "a.txt"), "x")
	g := w.guard(t, 100)
	_, err := g.Read("sub")
	wantReason(t, err, ReasonNotRegular, "directory")
	_, err = g.Read(w.root)
	wantReason(t, err, ReasonNotRegular, "the root itself")
	_, err = g.Read(".")
	wantReason(t, err, ReasonNotRegular, "dot")
	_, err = g.Read("")
	wantReason(t, err, ReasonBadRequest, "empty")
	_, err = g.Read("   ")
	wantReason(t, err, ReasonBadRequest, "blank")
}

// FILE-006: stat limit and the read limit.
func TestSizeLimit(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "exact.txt"), strings.Repeat("a", 100))
	write(t, filepath.Join(w.root, "big.txt"), strings.Repeat("a", 101))
	g := w.guard(t, 100)
	if f, err := g.Read("exact.txt"); err != nil || f.Bytes != 100 {
		t.Fatalf("file at the limit: %v", err)
	}
	_, err := g.Read("big.txt")
	wantReason(t, err, ReasonTooLarge, "one byte over")
	if !strings.Contains(err.Error(), "101") || !strings.Contains(err.Error(), "100") {
		t.Fatalf("message must state both sizes: %v", err)
	}
	if g.MaxBytes() != 100 {
		t.Fatal("MaxBytes")
	}
}

func TestBinaryAndEncoding(t *testing.T) {
	w := newWorld(t)
	put := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(w.root, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("zero.bin", []byte("abc\x00def"))
	put("png.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0x0D})
	put("ansi.txt", []byte("coment\xe1rio \xe9\n")) // Windows-1252
	put("utf16.txt", []byte{0xFF, 0xFE, 'a', 0, 'b', 0})
	put("empty.txt", nil)
	put("utf8.txt", []byte("coment\u00e1rio \u00e9 \u4e2d\n"))
	g := w.guard(t, 1<<20)
	for _, n := range []string{"zero.bin", "png.png"} {
		_, err := g.Read(n)
		wantReason(t, err, ReasonBinary, n)
	}
	_, err := g.Read("ansi.txt")
	wantReason(t, err, ReasonEncoding, "ansi")
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("explicit message expected: %v", err)
	}
	_, err = g.Read("utf16.txt")
	wantReason(t, err, ReasonEncoding, "utf16")
	if f, err := g.Read("empty.txt"); err != nil || f.Text != "" {
		t.Fatalf("empty file: %v", err)
	}
	if f, err := g.Read("utf8.txt"); err != nil || !strings.Contains(f.Text, "中") {
		t.Fatalf("utf8: %v", err)
	}
}

// FILE-013 / SEC-014: reading creates, changes and removes nothing.
func TestNothingIsWritten(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "hello")
	write(t, filepath.Join(w.root, ".env"), "S")
	before := snapshot(t, w.base)
	g := w.guard(t, 1<<20)
	for _, p := range []string{"a.txt", ".env", "nope", "..", `..\outside\x.txt`, "sub", filepath.Join(w.outside, "x.txt")} {
		g.Read(p)
	}
	after := snapshot(t, w.base)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("file system changed:\nbefore %v\nafter  %v", before, after)
	}
}

func snapshot(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		st, _ := os.Lstat(p)
		out = append(out, p+"|"+st.ModTime().String()+"|"+string(rune('0'+st.Size()%10)))
		return nil
	})
	sort.Strings(out)
	return out
}
