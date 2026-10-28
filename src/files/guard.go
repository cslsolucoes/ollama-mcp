package files

import (
	"bytes"
	"context"
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

	// Optional document support (phase 7): nil means every file is plain text,
	// exactly as before, and a document is refused as binary (fail closed).
	extractor DocExtractor
	maxDoc    int64
}

// Extracted is the result of a document extraction, in the terms of this
// package (it must not import the extractor). Text follows the File.Text
// contract: UTF-8 without BOM, LF line endings.
type Extracted struct {
	Text             string
	Format           string // "docx", "pptx", "pdf", "png" or "jpg"
	Pages            int    // PDF
	Slides           int    // PPTX
	PagesWithoutText int    // PDF
	PendingRevisions bool   // DOCX
	// ImportedContent: text imported by a DOCX w:altChunk is in Text.
	ImportedContent bool
	// UnreadContent: part of the content could not be read and is not in Text.
	UnreadContent bool
	// NumberingUnavailable: the list numbering exists but could not be read.
	NumberingUnavailable bool
	// HiddenText: Text has lines labelled "[oculto]" (not shown nor printed).
	HiddenText bool
	// OCR: text recognized from PDF pages without text or from an image.
	OCR OCRInfo
}

// OCRInfo describes the OCR of one extraction (zero value: no OCR ran).
type OCRInfo struct {
	Used         bool // some page (or the image) went to the OCR
	Pages        int  // pages recognized
	ConfMean     int  // mean confidence of those pages, 0-100
	Failed       int  // pages whose recognition failed
	NotProcessed int  // pages left out when the time budget ran out
}

// DocExtractor turns office documents (and, with OCR, images) into text. It
// is injected by the binary, so package files stays pure and testable. The
// guard has no list of extensions of its own: an image (.png, .jpg) is a
// document only because the extractor claims it, and every path defence runs
// before the extractor sees its bytes.
//
// IsDocument decides from the NAME alone (the extension of the resolved file)
// whether the document ceiling and the extractor apply. It must be true for
// every extension the extractor wants to see, including the legacy ones it
// only refuses (.doc, .ppt, .xls), so that their explanation reaches the user
// instead of "binary file".
//
// Extract receives the bytes the guard already read, after every path defence
// ran. A returned error must carry a user-safe pt-BR message (no path, no
// content): it is shown as is. A cancelled context may be returned unchanged.
type DocExtractor interface {
	IsDocument(name string) bool
	Extract(ctx context.Context, name string, data []byte) (*Extracted, error)
}

// ErrNotDocument is what an extractor returns for a name it claimed but whose
// bytes are not a document of that kind (a plain text file called notes.doc):
// the guard then reads it as text, under the text ceiling, as before phase 7.
var ErrNotDocument = errors.New("not a document")

// File is the result of a successful Read. Text is UTF-8 without BOM and with
// LF line endings. For a document, Text is the extracted text and the fields
// below Text describe the extraction; Bytes stays the size on disk.
type File struct {
	Name  string // base name of the resolved file, safe to show
	Bytes int    // size on disk
	Text  string

	Format           string // "txt", "docx", "pptx", "pdf", "png" or "jpg"
	Pages            int    // PDF
	Slides           int    // PPTX
	PagesWithoutText int    // PDF
	PendingRevisions bool   // DOCX
	// The four flags below mirror Extracted (documents only).
	ImportedContent      bool
	UnreadContent        bool
	NumberingUnavailable bool
	HiddenText           bool
	OCR                  OCRInfo
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
		// Only absolute roots: a relative one would be resolved against the
		// working directory of whoever launched the server, which a client
		// does not control (SEC-m-03).
		if !filepath.IsAbs(r) {
			return nil, fmt.Errorf("--allow-root %q: a raiz deve ser um caminho absoluto (raízes relativas são recusadas)", r)
		}
		abs := filepath.Clean(r)
		res, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("--allow-root %q: o diretório não existe ou não pode ser resolvido", r)
		}
		res = filepath.Clean(res)
		// Go's EvalSymlinks leaves a junction (mount point) unresolved, and no
		// read through such a root can work: refuse at the start instead of
		// accepting a root that silently fails every call (SEC-m-06).
		if lst, err := os.Lstat(res); err != nil || lst.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return nil, fmt.Errorf("--allow-root %q: a raiz é uma junction ou ponto de reanálise, não suportada; use o caminho real do diretório", r)
		}
		st, err := os.Stat(res)
		if err != nil || !st.IsDir() {
			return nil, fmt.Errorf("--allow-root %q: não é um diretório", r)
		}
		// Nested (or repeated) roots are refused: with several roots the
		// denylist and the containment would depend on their order (SEC-m-04).
		for i, o := range g.roots {
			if _, in := within(o, res); in {
				return nil, fmt.Errorf("--allow-root %q: está dentro de outra raiz (%q); raízes aninhadas ou repetidas são recusadas", r, g.rawRoots[i])
			}
			if _, in := within(res, o); in {
				return nil, fmt.Errorf("--allow-root %q: contém outra raiz (%q); raízes aninhadas ou repetidas são recusadas", r, g.rawRoots[i])
			}
		}
		g.rawRoots = append(g.rawRoots, abs)
		g.roots = append(g.roots, res)
	}
	return g, nil
}

// BroadDirs are the directories a root must not silently be: the user's home
// and the system directories. Injected so tests never depend on the machine.
type BroadDirs struct {
	Home   string
	System []string
}

// DefaultBroadDirs reads the real home (os.UserHomeDir) and, on Windows, the
// system directories from the environment.
func DefaultBroadDirs() BroadDirs {
	var b BroadDirs
	b.Home, _ = os.UserHomeDir()
	for _, k := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData"} {
		if v := os.Getenv(k); v != "" {
			b.System = append(b.System, v)
		}
	}
	return b
}

// BroadRoots returns the configured roots that are a volume root, the home
// directory or a system directory (owner decision: accepted, but the caller
// must warn on every start). Both sides are resolved and compared whole, case-
// insensitively on Windows.
func (g *Guard) BroadRoots(b BroadDirs) []string {
	if g == nil {
		return nil
	}
	canon := func(p string) string {
		if p == "" {
			return ""
		}
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = filepath.Clean(r)
		}
		return p
	}
	same := func(a, c string) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(a, c)
		}
		return a == c
	}
	var out []string
	for _, r := range g.roots {
		broad := filepath.Dir(r) == r // C:\ or /
		if h := canon(b.Home); h != "" && same(r, h) {
			broad = true
		}
		for _, s := range b.System {
			if c := canon(s); c != "" && same(r, c) {
				broad = true
			}
		}
		if broad {
			out = append(out, r)
		}
	}
	return out
}

// SplitRoots splits the value of --allow-root / MCPOLLAMA_ALLOW_ROOT. The
// separator is os.PathListSeparator (';' on Windows, ':' elsewhere), like the
// PATH variable, so a comma is a valid character of a directory name
// (SEC-m-02). Blanks are dropped.
func SplitRoots(s string) []string {
	var out []string
	for _, p := range strings.Split(s, string(os.PathListSeparator)) {
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

// SetExtractor enables documents: ex is consulted for the names it claims and
// maxDocBytes (--max-doc-bytes) replaces the text ceiling for them. It must be
// called before the guard is shared between goroutines. A nil extractor turns
// documents off again.
func (g *Guard) SetExtractor(ex DocExtractor, maxDocBytes int64) error {
	if ex != nil && maxDocBytes < 1 {
		return errors.New("o limite de bytes do documento deve ser positivo")
	}
	g.extractor, g.maxDoc = ex, maxDocBytes
	return nil
}

// docName is the name the document rules see: without the trailing dots and
// spaces that Windows drops ("x.pdf." opens "x.pdf"), so such a name cannot
// dodge the extension rules.
func docName(name string) string { return strings.TrimRight(name, ". ") }

// isDoc reports whether the resolved name goes to the extractor.
func (g *Guard) isDoc(name string) bool {
	return g.extractor != nil && g.extractor.IsDocument(docName(name))
}

// ceilingFor is the byte ceiling that applies to the resolved name: decided
// from the extension before the file is opened, so the size check on the
// directory entry and the bounded read use the same figure.
func (g *Guard) ceilingFor(name string) int64 {
	if g.isDoc(name) {
		return g.maxDoc
	}
	return g.maxBytes
}

// MaxDocBytes is the per-document ceiling (--max-doc-bytes); 0 when documents
// are not enabled.
func (g *Guard) MaxDocBytes() int64 {
	if g == nil || g.extractor == nil {
		return 0
	}
	return g.maxDoc
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
//  9. regular file only, size limit (the text or the document ceiling, chosen
//     by the extension of the resolved name), binary and encoding checks
//  10. the path still resolves to the same place (TOCTOU window)
//
// Every failure of 4, 5, 7 and 8 returns the same opaque message.
//
// A document (when an extractor is injected) takes the same ten steps; the
// only differences are the byte ceiling, decided before anything is opened,
// and the last step, where the bytes go to the extractor instead of the text
// decoder.
func (g *Guard) Read(requested string) (*File, error) {
	return g.ReadContext(context.Background(), requested)
}

// ReadContext is Read with a context: the extraction honors its cancellation
// and deadline.
func (g *Guard) ReadContext(ctx context.Context, requested string) (*File, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !g.Enabled() {
		return nil, errNotAllowed()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	name := filepath.Base(resolved)
	isDoc := g.isDoc(name)
	ceiling := g.ceilingFor(name)

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
	if pre.Size() > ceiling {
		return nil, tooLarge(pre.Size(), ceiling, isDoc)
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
	data, err := io.ReadAll(io.LimitReader(f, ceiling+1))
	if err != nil {
		return nil, refuse(ReasonBadRequest, "não foi possível ler o arquivo")
	}
	if int64(len(data)) > ceiling {
		return nil, tooLarge(int64(len(data)), ceiling, isDoc)
	}

	again, err := filepath.EvalSymlinks(abs)
	if err != nil || filepath.Clean(again) != resolved {
		return nil, errNotAllowed()
	}

	if isDoc {
		f, err := g.extract(ctx, name, data)
		if !errors.Is(err, ErrNotDocument) {
			return f, err
		}
		// Claimed but not a document: the text path, with the text ceiling.
		if int64(len(data)) > g.maxBytes {
			return nil, tooLarge(int64(len(data)), g.maxBytes, false)
		}
	}
	text, rf := decode(data)
	if rf != nil {
		return nil, rf
	}
	return &File{Name: name, Bytes: len(data), Text: text, Format: "txt"}, nil
}

// extract is the last step for a document: the bytes already passed every
// path defence. The extractor's refusal is shown as is (its messages are
// user-safe by contract); the text it returns gets a last check behind the
// extractor's own normalization.
func (g *Guard) extract(ctx context.Context, name string, data []byte) (*File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ex, err := g.extractor.Extract(ctx, docName(name), data)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrNotDocument) {
			return nil, err
		}
		return nil, refuse(ReasonDocument, err.Error())
	}
	if ex == nil || !utf8.ValidString(ex.Text) {
		return nil, refuse(ReasonDocument, "documento corrompido, malformado ou acima dos limites de segurança")
	}
	text := strings.ReplaceAll(strings.TrimPrefix(ex.Text, "\xEF\xBB\xBF"), "\r\n", "\n")
	return &File{
		Name: name, Bytes: len(data), Text: text, Format: ex.Format,
		Pages: ex.Pages, Slides: ex.Slides, PagesWithoutText: ex.PagesWithoutText, PendingRevisions: ex.PendingRevisions,
		ImportedContent: ex.ImportedContent, UnreadContent: ex.UnreadContent,
		NumberingUnavailable: ex.NumberingUnavailable, HiddenText: ex.HiddenText,
		OCR: ex.OCR,
	}, nil
}

func tooLarge(size, max int64, doc bool) *Refusal {
	flag := "--max-file-bytes"
	if doc {
		flag = "--max-doc-bytes"
	}
	return refuse(ReasonTooLarge, fmt.Sprintf(
		"arquivo grande demais: %d bytes, o teto é %d (%s); divida o arquivo ou aumente o teto", size, max, flag))
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
