package files

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// Guard is the allowlist of readable directories plus the read limits. The
// zero value has no roots and refuses everything.
type Guard struct {
	rawRoots []string // absolute and cleaned, as configured
	roots    []string // absolute, cleaned and symlink-resolved
	maxBytes int64
}

// File is the result of a successful Read. Text is UTF-8 without BOM and with
// LF line endings.
type File struct {
	Name  string // base name of the resolved file, safe to show
	Bytes int    // size on disk
	Text  string
}

// NewGuard validates the roots (each must exist and be a directory; the
// message names the offending root, which is the operator's own input) and
// resolves them once, so every later comparison is between canonical paths.
// maxBytes < 1 is an error: a missing limit must never mean "unlimited".
func NewGuard(roots []string, maxBytes int64) (*Guard, error) {
	if maxBytes < 1 {
		return nil, errors.New("o limite de bytes do arquivo deve ser positivo")
	}
	g := &Guard{maxBytes: maxBytes}
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if rf := checkSyntax(r, runtime.GOOS == "windows"); rf != nil {
			return nil, fmt.Errorf("--allow-root %q: %s", r, rf.Error())
		}
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, fmt.Errorf("--allow-root %q: caminho inválido", r)
		}
		abs = filepath.Clean(abs)
		res, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("--allow-root %q: o diretório não existe ou não pode ser resolvido", r)
		}
		st, err := os.Stat(res)
		if err != nil || !st.IsDir() {
			return nil, fmt.Errorf("--allow-root %q: não é um diretório", r)
		}
		g.rawRoots = append(g.rawRoots, abs)
		g.roots = append(g.roots, filepath.Clean(res))
	}
	return g, nil
}

// SplitRoots splits the value of --allow-root / MCPOLLAMA_ALLOW_ROOT. The
// separator is the comma of the flag's contract; blanks are dropped.
func SplitRoots(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Enabled reports whether at least one root is configured.
func (g *Guard) Enabled() bool { return g != nil && len(g.roots) > 0 }

// Roots returns the resolved roots (for the startup banner).
func (g *Guard) Roots() []string {
	if g == nil {
		return nil
	}
	return append([]string(nil), g.roots...)
}

// MaxBytes is the per-file ceiling (--max-file-bytes).
func (g *Guard) MaxBytes() int64 {
	if g == nil {
		return 0
	}
	return g.maxBytes
}

// Read validates the request and returns the file text. The steps, in order,
// and what each one defends against:
//
//  1. syntax: UNC, \\?\, \\.\, ADS, device names, drive-relative (Windows)
//  2. shape: relative paths only with exactly one root (FILE-002)
//  3. lexical clean, then the denylist on the REQUESTED name (before any I/O,
//     so a denied name never reveals whether the file exists)
//  4. EvalSymlinks: the target that will really be opened
//  5. containment of the resolved target in a resolved root, by segments
//  6. the denylist again on the RESOLVED name (a symlink named notes.txt that
//     points at .env inside the root)
//  7. no unresolved reparse point (junction) on the directories below the root
//  8. open through os.Root: the kernel-level traversal guard, so a swap
//     between steps 4 and 8 still cannot leave the root
//  9. regular file only, size limit, binary and encoding checks
//  10. the path still resolves to the same place (TOCTOU window)
//
// Every failure of 4, 5, 7 and 8 returns the same opaque message.
func (g *Guard) Read(requested string) (*File, error) {
	if !g.Enabled() {
		return nil, errNotAllowed()
	}
	if strings.TrimSpace(requested) == "" {
		return nil, refuse(ReasonBadRequest, "informe o caminho do arquivo")
	}
	windows := runtime.GOOS == "windows"
	if rf := checkSyntax(requested, windows); rf != nil {
		return nil, rf
	}

	abs, rf := g.absolute(requested, windows)
	if rf != nil {
		return nil, rf
	}
	if Denied(g.requestSegments(abs)) {
		return nil, errDenylisted()
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, errNotAllowed()
	}
	resolved = filepath.Clean(resolved)
	rootIdx, rel := -1, ""
	for i, r := range g.roots {
		if rr, ok := within(r, resolved); ok {
			rootIdx, rel = i, rr
			break
		}
	}
	if rootIdx < 0 {
		return nil, errNotAllowed()
	}
	if Denied(components(rel)) {
		return nil, errDenylisted()
	}
	root := g.roots[rootIdx]
	if hasReparseDir(root, rel) {
		return nil, errNotAllowed()
	}

	rt, err := os.OpenRoot(root)
	if err != nil {
		return nil, errNotAllowed()
	}
	defer rt.Close()
	pre, err := rt.Stat(rel)
	if err != nil {
		return nil, errNotAllowed()
	}
	if !pre.Mode().IsRegular() {
		return nil, refuse(ReasonNotRegular, "o caminho não é um arquivo regular (diretório, dispositivo ou pipe)")
	}
	if pre.Size() > g.maxBytes {
		return nil, tooLarge(pre.Size(), g.maxBytes)
	}
	f, err := rt.Open(rel)
	if err != nil {
		return nil, errNotAllowed()
	}
	defer f.Close()
	// Validate the descriptor that is actually open, not the path (FILE-005).
	post, err := f.Stat()
	if err != nil || !post.Mode().IsRegular() || !os.SameFile(pre, post) {
		return nil, errNotAllowed()
	}
	// A hard link is a second name for the same file: the denylist and the
	// containment only ever saw this name, so a file with more than one link is
	// refused (SEC-C-01 of the adversarial review). Fail-closed by design.
	if n, err := linkCount(f); err != nil || n != 1 {
		return nil, errNotAllowed()
	}
	data, err := io.ReadAll(io.LimitReader(f, g.maxBytes+1))
	if err != nil {
		return nil, refuse(ReasonBadRequest, "não foi possível ler o arquivo")
	}
	if int64(len(data)) > g.maxBytes {
		return nil, tooLarge(int64(len(data)), g.maxBytes)
	}

	again, err := filepath.EvalSymlinks(abs)
	if err != nil || filepath.Clean(again) != resolved {
		return nil, errNotAllowed()
	}

	text, rf := decode(data)
	if rf != nil {
		return nil, rf
	}
	return &File{Name: filepath.Base(resolved), Bytes: len(data), Text: text}, nil
}

func tooLarge(size, max int64) *Refusal {
	return refuse(ReasonTooLarge, fmt.Sprintf(
		"arquivo grande demais: %d bytes, o teto é %d (--max-file-bytes); divida o arquivo ou aumente o teto", size, max))
}

// absolute turns the request into an absolute, cleaned path. Only the cleaned
// string is ever used afterwards: what is checked is what is opened.
func (g *Guard) absolute(requested string, windows bool) (string, *Refusal) {
	if filepath.IsAbs(requested) {
		return filepath.Clean(requested), nil
	}
	if len(g.roots) != 1 {
		return "", refuse(ReasonBadRequest, "há mais de uma raiz permitida: informe o caminho absoluto do arquivo")
	}
	if windows && filepath.VolumeName(requested) != "" {
		return "", errSpecial("caminhos relativos à unidade são recusados")
	}
	return filepath.Join(g.roots[0], requested), nil
}

// requestSegments are the segments the denylist sees for the requested path:
// those below a root when the path is lexically under one (so a root called
// "secrets" does not block its own files), the whole path otherwise.
func (g *Guard) requestSegments(abs string) []string {
	for _, rs := range [][]string{g.rawRoots, g.roots} {
		for _, r := range rs {
			if rel, ok := within(r, abs); ok {
				return components(rel)
			}
		}
	}
	return components(abs)
}

// hasReparseDir reports whether a directory between root and the file is a
// reparse point that EvalSymlinks left unresolved. On Windows Go's
// EvalSymlinks does not follow junctions (they fail to resolve) but the check
// is kept as a second line: a junction must never be crossed.
func hasReparseDir(root, rel string) bool {
	parts := components(rel)
	cur := root
	for i := 0; i < len(parts)-1; i++ {
		cur = filepath.Join(cur, parts[i])
		st, err := os.Lstat(cur)
		if err != nil {
			return true
		}
		if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}

// decode applies the encoding rules of FILE-007 and FILE-017: a NUL byte or
// invalid UTF-8 means "not text" and is refused with an explicit message; a
// UTF-8 BOM is dropped and CRLF becomes LF. Nothing is guessed: an ANSI
// (Windows-1252) file is refused, not silently mangled.
func decode(data []byte) (string, *Refusal) {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return "", refuse(ReasonEncoding, "arquivo em UTF-16 não é suportado; converta para UTF-8")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", refuse(ReasonBinary, "arquivo binário recusado (contém bytes nulos); só texto UTF-8 é lido")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return "", refuse(ReasonEncoding, "o arquivo não é UTF-8 válido (talvez ANSI/Windows-1252); converta para UTF-8 antes de usar")
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
}
