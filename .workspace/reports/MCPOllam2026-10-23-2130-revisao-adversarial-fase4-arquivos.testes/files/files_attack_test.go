package files

// Adversarial review of phase 4 (2026-10-23). Every vector logs one line
//
//	VEC|<id>|BLOQUEADO|<reason>      or      VEC|<id>|ESCAPOU|<what leaked>
//
// and an escape is a test failure. The sandbox is newWorld (guard_test.go):
// <base>/root is allowed, <base>/outside/x.txt holds outsideContent.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const secretContent = "SENSITIVE-DENYLIST-CONTENT-4b2a"

// attempt runs one request that must be refused and must not leak.
func attempt(t *testing.T, w world, g *Guard, id, req string) {
	t.Helper()
	f, err := g.Read(req)
	if err == nil {
		leak := ""
		if strings.Contains(f.Text, outsideContent) {
			leak = "conteudo-de-fora"
		} else if strings.Contains(f.Text, secretContent) {
			leak = "conteudo-da-denylist"
		} else {
			leak = fmt.Sprintf("leu %q (%d bytes)", f.Name, f.Bytes)
		}
		t.Errorf("VEC|%s|ESCAPOU|%s", id, leak)
		return
	}
	var r *Refusal
	if !errors.As(err, &r) {
		t.Errorf("VEC|%s|ESCAPOU|erro nao-Refusal: %v", id, err)
		return
	}
	m := err.Error()
	if strings.Contains(m, w.base) || strings.Contains(m, filepath.Base(w.base)) ||
		strings.Contains(m, outsideContent) || strings.Contains(m, secretContent) {
		t.Errorf("VEC|%s|ESCAPOU|mensagem vaza caminho/conteudo: %q", id, m)
		return
	}
	t.Logf("VEC|%s|BLOQUEADO|reason=%d %s", id, r.Reason, m)
}

// mustRead is the positive control: a legitimate request must still work.
func mustRead(t *testing.T, g *Guard, id, req, want string) {
	t.Helper()
	f, err := g.Read(req)
	if err != nil || f.Text != want {
		t.Errorf("CTRL|%s|FALHOU|%v", id, err)
		return
	}
	t.Logf("CTRL|%s|OK", id)
}

func winOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
}

// ---- 1. traversal in every spelling ---------------------------------------

func TestAttackTraversal(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, "ok.txt"), "OK")
	write(t, filepath.Join(w.root, "sub", "a.txt"), "A")
	g := w.guard(t, 1<<20)
	mustRead(t, g, "controle-relativo", "ok.txt", "OK")

	rel := map[string]string{
		"dotdot-backslash":       `..\outside\x.txt`,
		"dotdot-slash":           `../outside/x.txt`,
		"dotdot-mixed":           `..\/outside/x.txt`,
		"dotdot-deep":            `sub\..\..\outside\x.txt`,
		"dotdot-many":            strings.Repeat(`..\`, 40) + `outside\x.txt`,
		"four-dots-slash":        `....//outside/x.txt`,
		"four-dots-back":         `....\\outside\x.txt`,
		"three-dots":             `...\outside\x.txt`,
		"dotdot-space":           `.. \outside\x.txt`,
		"dotdot-2space":          `..  \outside\x.txt`,
		"dotdot-dot":             `...\..\outside\x.txt`,
		"dotdot-space-dot":       `.. .\outside\x.txt`,
		"dotdot-nbsp":            "..\u00a0\\outside\\x.txt",
		"dot-dotdot":             `.\..\outside\x.txt`,
		"dotdot-dot-seg":         `..\.\outside\x.txt`,
		"urlenc-dotdot":          `%2e%2e/outside/x.txt`,
		"urlenc-backslash":       `%2e%2e%5coutside%5cx.txt`,
		"urlenc-slash":           `..%2foutside%2fx.txt`,
		"double-urlenc":          `%252e%252e/outside/x.txt`,
		"unicode-fullstop":       "\uff0e\uff0e\\outside\\x.txt",
		"unicode-fwslash":        "..\uff0foutside\uff0fx.txt",
		"tilde":                  `~\outside\x.txt`,
		"envvar-percent":         `%USERPROFILE%\x.txt`,
		"envvar-ps":              `$env:USERPROFILE\x.txt`,
		"tab":                    "\t../outside/x.txt",
		"leading-space":          ` ..\outside\x.txt`,
		"trailing-sep":           `..\outside\x.txt\`,
		"rooted-no-drive":        strings.TrimPrefix(filepath.Join(w.outside, "x.txt"), filepath.VolumeName(w.outside)),
		"rooted-no-drive-slash":  filepath.ToSlash(strings.TrimPrefix(filepath.Join(w.outside, "x.txt"), filepath.VolumeName(w.outside))),
		"drive-relative-dotdot":  filepath.VolumeName(w.root) + `..\outside\x.txt`,
		"drive-relative":         filepath.VolumeName(w.root) + `outside\x.txt`,
	}
	for id, p := range rel {
		attempt(t, w, g, "trav/"+id, p)
	}

	abs := map[string]string{
		"abs-dotdot":        filepath.Join(w.root) + `\..\outside\x.txt`,
		"abs-dotdot-slash":  filepath.ToSlash(w.root) + `/../outside/x.txt`,
		"abs-outside":       filepath.Join(w.outside, "x.txt"),
		"abs-sibling":       filepath.Join(w.sibling, "f.txt"),
		"abs-sibling-upper": strings.ToUpper(filepath.Join(w.sibling, "f.txt")),
		"abs-root-dotdot-space": w.root + `\.. \outside\x.txt`,
		"abs-root-dot-trailing": w.root + `.\..\outside\x.txt`,
		"abs-root-space-trailing": w.root + ` \..\outside\x.txt`,
		"abs-other-drive":   `C:\Windows\win.ini`,
		"abs-other-drive-fwd": `C:/Windows/win.ini`,
		"abs-winini-dotdot": w.root + strings.Repeat(`\..`, 30) + `\Windows\win.ini`,
	}
	for id, p := range abs {
		attempt(t, w, g, "trav/"+id, p)
	}
}

// ---- 2. Windows special syntax --------------------------------------------

func TestAttackWindowsSpecial(t *testing.T) {
	winOnly(t)
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "A")
	g := w.guard(t, 1<<20)
	vol := filepath.VolumeName(w.outside) // e.g. D:
	drive := strings.TrimSuffix(vol, ":")
	outX := filepath.Join(w.outside, "x.txt")
	noVol := strings.TrimPrefix(outX, vol)
	cases := map[string]string{
		"unc-localhost-admin":   `\\localhost\` + drive + `$` + noVol,
		"unc-127":               `\\127.0.0.1\` + drive + `$` + noVol,
		"unc-fwd":               `//localhost/` + drive + `$` + filepath.ToSlash(noVol),
		"unc-mixed":             `\/localhost\` + drive + `$` + noVol,
		"unc-mixed2":            `/\localhost\` + drive + `$` + noVol,
		"long-prefix":           `\\?\` + outX,
		"long-prefix-fwd":       `//?/` + filepath.ToSlash(outX),
		"long-prefix-unc":       `\\?\UNC\localhost\` + drive + `$` + noVol,
		"device-prefix":         `\\.\` + outX,
		"device-prefix-fwd":     `//./` + filepath.ToSlash(outX),
		"nt-prefix":             `\??\` + outX,
		"nt-prefix-fwd":         `/??/` + filepath.ToSlash(outX),
		"globalroot":            `\\?\GLOBALROOT\Device\HarddiskVolume1\Windows\win.ini`,
		"lanman":                `\\;LanmanRedirector\localhost\` + drive + `$` + noVol,
		"ads-name":              `a.txt:x`,
		"ads-data":              `a.txt::$DATA`,
		"ads-named-data":        `a.txt:x:$DATA`,
		"ads-dir-index":         `sub::$INDEX_ALLOCATION\a.txt`,
		"ads-abs":               filepath.Join(w.root, "a.txt") + `:x`,
		"ads-on-dir-dotdot":     `..::$INDEX_ALLOCATION\outside\x.txt`,
		"wildcard-star":         `*.txt`,
		"wildcard-q":            `a.tx?`,
		"wildcard-lt":           `a<txt`,
		"wildcard-gt":           `a>txt`,
		"wildcard-quote":        `a"txt`,
		"pipe":                  `a|b`,
		"nul-byte":              "a.txt\x00.png",
		"nul-only":              "\x00",
		"empty":                 "",
		"spaces":                "   ",
		"ctrl-chars":            "a\x01b.txt",
	}
	for id, p := range cases {
		attempt(t, w, g, "win/"+id, p)
	}
	// ADS with real content: the stream exists and holds a secret.
	if err := os.WriteFile(filepath.Join(w.root, "a.txt")+":hidden", []byte(secretContent), 0o644); err == nil {
		attempt(t, w, g, "win/ads-real-stream", "a.txt:hidden")
		attempt(t, w, g, "win/ads-real-stream-data", "a.txt:hidden:$DATA")
	} else {
		t.Logf("VEC|win/ads-real-stream|NAO-TESTAVEL|%v", err)
	}
}

func TestAttackDeviceNames(t *testing.T) {
	winOnly(t)
	w := newWorld(t)
	g := w.guard(t, 1<<20)
	for _, n := range []string{
		"con", "CON", "CON.txt", "con .txt", "CON.", "CON ", "nul.tar.gz", "NUL", "prn", "aux.c",
		"COM1", "com9.log", "COM¹", "com²", "LPT³.txt", "lpt1", "conin$", "CONOUT$", "CLOCK$",
		`sub\aux\x.txt`, `sub\con`, "COM0", "LPT0", "COM10",
	} {
		attempt(t, w, g, "dev/"+n, n)
	}
}

// ---- 3. denylist aliases ----------------------------------------------------

func denyWorld(t *testing.T, w world) {
	for _, n := range []string{".env", "secret.txt", "credentials.json", "id_rsa", "server.key", "my.pem",
		".mcp.json", "settings.local.json", filepath.Join(".git", "config"), filepath.Join("secrets", "a.txt")} {
		write(t, filepath.Join(w.root, n), secretContent)
	}
}

func TestAttackDenylistAliases(t *testing.T) {
	w := newWorld(t)
	denyWorld(t, w)
	g := w.guard(t, 1<<20)
	cases := []string{
		".ENV", ".Env", `.\.env`, `sub\..\.env`, `.env.`, `.env..`, `.env `, `.env. . .`, `.env  .`,
		"SECRET.TXT", "secret.txt.", "secret.txt ", "Secret.Txt...",
		"\u017fecret.txt",          // long s: Unicode-folds to s
		"credent\u0131als.json",    // dotless i
		"CREDENTIALS.JSON",
		"\u0131d_rsa", "id_r\u017fa", "ID_RSA", "id_rsa.",
		"server.KEY", "server.key.", "my.PEM ",
		".MCP.JSON", ".mcp.json.", "SETTINGS.LOCAL.JSON",
		`.git\config`, `.GIT\config`, `.git.\config`, `.git \config`, `.git\.\config`, `.g\u0131t\config`,
		`secrets\a.txt`, `SECRETS\a.txt`, `secrets.\a.txt`, `\u017fecrets\a.txt`,
		`.env::$DATA`, `.env:$DATA`,
	}
	for _, c := range cases {
		attempt(t, w, g, "deny/"+c, c)
	}
	// absolute forms
	attempt(t, w, g, "deny/abs-upper", strings.ToUpper(filepath.Join(w.root, ".env")))
	attempt(t, w, g, "deny/abs-dotdot", filepath.Join(w.root, "sub")+`\..\.env`)
}

// ---- 4. symlinks, junctions, hardlinks -------------------------------------

func mklink(t *testing.T, args ...string) bool {
	t.Helper()
	out, err := exec.Command("cmd", append([]string{"/c", "mklink"}, args...)...).CombinedOutput()
	if err != nil {
		t.Logf("mklink %v: %v %s", args, err, out)
		return false
	}
	return true
}

func TestAttackSymlinks(t *testing.T) {
	w := newWorld(t)
	denyWorld(t, w)
	write(t, filepath.Join(w.root, "real", "in.txt"), "INSIDE")
	g := w.guard(t, 1<<20)
	type ln struct{ name, target string }
	links := []ln{
		{"f-out.txt", filepath.Join(w.outside, "x.txt")},
		{"f-out-rel.txt", `..\outside\x.txt`},
		{"f-out-long.txt", `\\?\` + filepath.Join(w.outside, "x.txt")},
		{"f-env.txt", filepath.Join(w.root, ".env")},
		{"f-env-rel.txt", `.env`},
		{"f-gitcfg.txt", filepath.Join(w.root, ".git", "config")},
		{"d-out", w.outside},
		{"d-git", filepath.Join(w.root, ".git")},
		{"d-secrets", filepath.Join(w.root, "secrets")},
		{"chain1.txt", filepath.Join(w.root, "chain2.txt")},
		{"chain2.txt", filepath.Join(w.outside, "x.txt")},
		{"loop1.txt", filepath.Join(w.root, "loop2.txt")},
		{"loop2.txt", filepath.Join(w.root, "loop1.txt")},
		{"d-up", `..`},
	}
	if runtime.GOOS == "windows" {
		drive := strings.TrimSuffix(filepath.VolumeName(w.outside), ":")
		links = append(links, ln{"f-unc.txt", `\\localhost\` + drive + `$` + strings.TrimPrefix(filepath.Join(w.outside, "x.txt"), filepath.VolumeName(w.outside))})
	}
	for _, l := range links {
		p := filepath.Join(w.root, l.name)
		if err := os.Symlink(l.target, p); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	mustRead(t, g, "symlink-dentro", filepath.Join("real", "in.txt"), "INSIDE")
	for _, req := range []string{"f-out.txt", "f-out-rel.txt", "f-out-long.txt", "f-env.txt", "f-env-rel.txt", "f-gitcfg.txt",
		`d-out\x.txt`, `d-git\config`, `d-secrets\a.txt`, "chain1.txt", "loop1.txt", `d-up\outside\x.txt`, `d-up\root\.env`, "f-unc.txt"} {
		attempt(t, w, g, "sym/"+req, req)
	}
}

func TestAttackJunctions(t *testing.T) {
	winOnly(t)
	w := newWorld(t)
	denyWorld(t, w)
	write(t, filepath.Join(w.root, "real", "in.txt"), "INSIDE")
	g := w.guard(t, 1<<20)
	if !mklink(t, "/J", filepath.Join(w.root, "j-out"), w.outside) {
		t.Skip("no junctions")
	}
	mklink(t, "/J", filepath.Join(w.root, "j-in"), filepath.Join(w.root, "real"))
	mklink(t, "/J", filepath.Join(w.root, "j-git"), filepath.Join(w.root, ".git"))
	mklink(t, "/J", filepath.Join(w.root, "real", "j-deep"), w.base)
	for _, req := range []string{`j-out\x.txt`, `j-in\in.txt`, `j-git\config`, `real\j-deep\outside\x.txt`, `real\j-deep\root\.env`,
		filepath.Join(w.root, "j-out", "x.txt"), `J-OUT\x.txt`, `j-out.\x.txt`, `j-out \x.txt`} {
		attempt(t, w, g, "junc/"+req, req)
	}
	mustRead(t, g, "junction-caminho-real", filepath.Join("real", "in.txt"), "INSIDE")
}

// The root itself reached through a junction: the operator chose it; reads
// below it must work and must stay below the junction target.
func TestAttackRootIsJunction(t *testing.T) {
	winOnly(t)
	w := newWorld(t)
	write(t, filepath.Join(w.root, "a.txt"), "A")
	jr := filepath.Join(w.base, "jroot")
	if !mklink(t, "/J", jr, w.root) {
		t.Skip("no junctions")
	}
	g, err := NewGuard([]string{jr}, 100)
	if err != nil {
		t.Logf("CTRL|root-junction|RECUSADO-NA-PARTIDA|%v", err)
		return
	}
	t.Logf("root junction resolved roots=%v", g.Roots())
	mustRead(t, g, "root-junction-leitura", "a.txt", "A")
	attempt(t, w, g, "rootj/dotdot", `..\outside\x.txt`)
	attempt(t, w, g, "rootj/abs-dotdot", jr+`\..\outside\x.txt`)
}

func TestAttackHardlinks(t *testing.T) {
	w := newWorld(t)
	denyWorld(t, w)
	g := w.guard(t, 1<<20)
	ok := 0
	for name, target := range map[string]string{
		"hl-env.txt":     filepath.Join(w.root, ".env"),
		"hl-gitcfg.txt":  filepath.Join(w.root, ".git", "config"),
		"hl-outside.txt": filepath.Join(w.outside, "x.txt"),
	} {
		if err := os.Link(target, filepath.Join(w.root, name)); err != nil {
			t.Logf("VEC|hard/%s|NAO-TESTAVEL|%v", name, err)
			continue
		}
		ok++
		attempt(t, w, g, "hard/"+name, name)
	}
	if ok == 0 {
		t.Skip("hardlinks unavailable")
	}
}

// ---- 5. TOCTOU ---------------------------------------------------------------

// swapLoop alternates p between a benign regular file and a symlink to target
// until stop is closed.
func swapLoop(p, target string, stop <-chan struct{}, wg *sync.WaitGroup, swaps *atomic.Int64) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
		}
		os.Remove(p)
		os.Symlink(target, p)
		os.Remove(p)
		os.WriteFile(p, []byte("BENIGN"), 0o644)
		swaps.Add(1)
	}
}

func raceReads(t *testing.T, w world, g *Guard, id, req string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	var reads, okReads, leaks int
	for time.Now().Before(deadline) {
		f, err := g.Read(req)
		reads++
		if err == nil {
			okReads++
			if strings.Contains(f.Text, secretContent) || strings.Contains(f.Text, outsideContent) {
				leaks++
			}
		}
	}
	if leaks > 0 {
		t.Errorf("VEC|%s|ESCAPOU|%d de %d leituras devolveram conteudo proibido", id, leaks, reads)
		return
	}
	t.Logf("VEC|%s|BLOQUEADO|%d leituras, %d ok (benignas), 0 vazamentos", id, reads, okReads)
}

func TestAttackTOCTOUFileSwap(t *testing.T) {
	w := newWorld(t)
	denyWorld(t, w)
	g := w.guard(t, 1<<20)
	probe := filepath.Join(w.root, "probe")
	if err := os.Symlink(filepath.Join(w.root, ".env"), probe); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	os.Remove(probe)
	for _, tc := range []struct{ id, target string }{
		{"toctou/file->symlink-.env-dentro", filepath.Join(w.root, ".env")},
		{"toctou/file->symlink-fora", filepath.Join(w.outside, "x.txt")},
	} {
		p := filepath.Join(w.root, "notes.txt")
		os.WriteFile(p, []byte("BENIGN"), 0o644)
		stop := make(chan struct{})
		var wg sync.WaitGroup
		var swaps atomic.Int64
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go swapLoop(p, tc.target, stop, &wg, &swaps)
		}
		raceReads(t, w, g, tc.id, "notes.txt", 4*time.Second)
		close(stop)
		wg.Wait()
		t.Logf("%s: %d swaps", tc.id, swaps.Load())
	}
}

// directory swap: sub (real dir) <-> sub (symlink to outside)
func TestAttackTOCTOUDirSwap(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 1<<20)
	write(t, filepath.Join(w.outside, "x.txt"), outsideContent)
	sub := filepath.Join(w.root, "sub")
	if err := os.Symlink(w.outside, sub); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	os.Remove(sub)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			os.RemoveAll(sub)
			os.MkdirAll(sub, 0o755)
			os.WriteFile(filepath.Join(sub, "x.txt"), []byte("BENIGN"), 0o644)
			os.RemoveAll(sub)
			os.Symlink(w.outside, sub)
		}
	}()
	raceReads(t, w, g, "toctou/dir->symlink-fora", `sub\x.txt`, 4*time.Second)
	close(stop)
	wg.Wait()
}

// a file that grows past the ceiling while it is read
func TestAttackGrowingFile(t *testing.T) {
	w := newWorld(t)
	p := filepath.Join(w.root, "grow.txt")
	os.WriteFile(p, []byte(strings.Repeat("a", 50)), 0o644)
	g := w.guard(t, 100)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			os.WriteFile(p, []byte(strings.Repeat("b", 5000)), 0o644)
			os.WriteFile(p, []byte(strings.Repeat("a", 50)), 0o644)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	over := 0
	for time.Now().Before(deadline) {
		if f, err := g.Read("grow.txt"); err == nil && f.Bytes > 100 {
			over++
		}
	}
	close(stop)
	wg.Wait()
	if over > 0 {
		t.Errorf("VEC|size/growing|ESCAPOU|%d leituras acima do teto", over)
	} else {
		t.Log("VEC|size/growing|BLOQUEADO|nenhuma leitura acima do teto")
	}
}

// ---- 6. sizes, encodings, long paths ---------------------------------------

func TestAttackSizesAndEncodings(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 1000)
	write(t, filepath.Join(w.root, "big.txt"), strings.Repeat("x", 1001))
	attempt(t, w, g, "size/one-over", "big.txt")
	// sparse 1 TiB file
	sp := filepath.Join(w.root, "sparse.txt")
	os.WriteFile(sp, nil, 0o644)
	if runtime.GOOS == "windows" {
		exec.Command("fsutil", "sparse", "setflag", sp).Run()
	}
	if err := os.Truncate(sp, 1<<40); err == nil {
		attempt(t, w, g, "size/sparse-1TiB", "sparse.txt")
	} else {
		t.Logf("VEC|size/sparse-1TiB|NAO-TESTAVEL|%v", err)
	}
	os.Remove(sp)
	enc := map[string][]byte{
		"utf16le-bom":   {0xFF, 0xFE, 'a', 0, 'b', 0},
		"utf16be-bom":   {0xFE, 0xFF, 0, 'a', 0, 'b'},
		"utf16le-nobom": {'a', 0, 'b', 0},
		"utf32le-bom":   {0xFF, 0xFE, 0, 0, 'a', 0, 0, 0},
		"cesu-surrogate": []byte("a\xed\xa0\x80b"),
		"overlong-slash": []byte("a\xc0\xafb"),
		"latin1":        []byte("ol\xe1"),
	}
	for id, b := range enc {
		os.WriteFile(filepath.Join(w.root, id), b, 0o644)
		attempt(t, w, g, "enc/"+id, id)
	}
	// positive: BOM + CRLF + no final newline + injection text are plain text
	os.WriteFile(filepath.Join(w.root, "bom.txt"), []byte("\xEF\xBB\xBFa\r\nb"), 0o644)
	mustRead(t, g, "bom-crlf-sem-lf-final", "bom.txt", "a\nb")
}

func TestAttackLongPaths(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 1000)
	// a legitimate deep path above MAX_PATH must still work, not crash
	deep := w.root
	segs := []string{}
	for len(deep) < 300 {
		segs = append(segs, strings.Repeat("d", 50))
		deep = filepath.Join(deep, strings.Repeat("d", 50))
	}
	if err := os.MkdirAll(deep, 0o755); err == nil && os.WriteFile(filepath.Join(deep, "f.txt"), []byte("DEEP"), 0o644) == nil {
		mustRead(t, g, "caminho>260-legitimo", filepath.Join(append(segs, "f.txt")...), "DEEP")
	} else {
		t.Logf("CTRL|caminho>260|NAO-TESTAVEL|%v", err)
	}
	attempt(t, w, g, "long/40k-name", strings.Repeat("a", 40000))
	attempt(t, w, g, "long/40k-dotdot", strings.Repeat(`a\..\`, 8000)+`..\outside\x.txt`)
	attempt(t, w, g, "long/100k-seps", strings.Repeat(`\`, 100000)+"x")
	attempt(t, w, g, "long/1M", strings.Repeat("ab/", 350000))
}

// ---- 7. --allow-root configuration -------------------------------------------

func TestAttackAllowRootConfig(t *testing.T) {
	w := newWorld(t)
	// a comma inside a directory name splits it into two roots
	comma := filepath.Join(w.base, "Cliente, Inc")
	os.MkdirAll(comma, 0o755)
	roots := SplitRoots(comma)
	t.Logf("SplitRoots(%q) = %q", "<base>\\Cliente, Inc", roots)
	if len(roots) != 1 {
		// does the first half become a root that is the PARENT prefix?
		os.MkdirAll(filepath.Join(w.base, "Cliente"), 0o755)
		g, err := NewGuard(roots, 100)
		t.Logf("VEC|cfg/comma-in-root|ACHADO|SplitRoots corta o nome; NewGuard err=%v roots=%d", err, func() int {
			if g == nil {
				return 0
			}
			return len(g.Roots())
		}())
	}
	// relative root resolved against the process working directory
	wd, _ := os.Getwd()
	if g, err := NewGuard([]string{"."}, 100); err == nil {
		t.Logf("VEC|cfg/relative-root|ACHADO|raiz '.' aceita e resolvida para o cwd do processo (%v)", strings.EqualFold(g.Roots()[0], wd))
	}
	// drive root
	if runtime.GOOS == "windows" {
		if g, err := NewGuard([]string{`C:\`}, 1<<20); err == nil {
			_, rerr := g.Read(`C:\Windows\win.ini`)
			t.Logf("VEC|cfg/drive-root|ACHADO|--allow-root C:\\ aceito; leitura de C:\\Windows\\win.ini err=%v", rerr)
		}
		for _, bad := range []string{`\\?\C:\Windows`, `\\localhost\c$`, `C:`, `C:Windows`, "C:\\Windows\\win.ini", `C:\Windows::$INDEX_ALLOCATION`} {
			if _, err := NewGuard([]string{bad}, 100); err == nil {
				t.Errorf("VEC|cfg/root-%q|ESCAPOU|aceito", bad)
			} else {
				t.Logf("VEC|cfg/root-%q|BLOQUEADO|%v", bad, err)
			}
		}
	}
	// maxBytes
	if _, err := NewGuard([]string{w.root}, 0); err == nil {
		t.Error("VEC|cfg/maxbytes-0|ESCAPOU|aceito")
	}
}

// Nested roots: the denylist result depends on the order of --allow-root.
func TestAttackNestedRootsOrder(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, ".git", "hooks", "h.txt"), secretContent)
	inner := filepath.Join(w.root, ".git", "hooks")
	req := filepath.Join(inner, "h.txt")
	gA, _ := NewGuard([]string{w.root, inner}, 100)
	gB, _ := NewGuard([]string{inner, w.root}, 100)
	_, errA := gA.Read(req)
	_, errB := gB.Read(req)
	t.Logf("VEC|cfg/nested-roots|INFO|ordem [raiz, raiz/.git/hooks] err=%v ; ordem inversa err=%v", errA, errB)
	if (errA == nil) != (errB == nil) {
		t.Logf("VEC|cfg/nested-roots|ACHADO|resultado depende da ordem das raizes")
	}
}

// ---- 8. read-only: nothing is created anywhere by refused or accepted reads --

func TestAttackNoWrites(t *testing.T) {
	w := newWorld(t)
	denyWorld(t, w)
	write(t, filepath.Join(w.root, "ok.txt"), "OK")
	before := atkSnapshot(t, w.base)
	g := w.guard(t, 1<<20)
	for _, r := range []string{"ok.txt", ".env", `..\outside\x.txt`, "CON", "nao-existe.txt", `sub\novo\x.txt`} {
		g.Read(r)
	}
	if after := atkSnapshot(t, w.base); after != before {
		t.Errorf("VEC|nowrite|ESCAPOU|a arvore mudou")
	} else {
		t.Log("VEC|nowrite|BLOQUEADO|nenhuma mudanca na arvore")
	}
}

func atkSnapshot(t *testing.T, dir string) string {
	var b strings.Builder
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil {
			st, _ := os.Lstat(p)
			fmt.Fprintf(&b, "%s|%d|%v\n", p, st.Size(), st.ModTime())
		}
		return nil
	})
	return b.String()
}
