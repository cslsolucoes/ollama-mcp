package files

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Tests of the document path (phase 7.6). Package files never imports doctext:
// everything here runs against a fake DocExtractor, so the guard behaviour
// (ceilings, denylist before reading, links, ctx) is proved on its own.

const docSecretContent = "DOC-SECRET-CONTENT-9f3c"

const docText = "texto extraido\n[pagina 1]\n"

// fakeExtractor claims the document extensions of the real adapter and records
// how often it was called.
type fakeExtractor struct {
	calls atomic.Int32
	seen  []byte
	name  string
	res   *Extracted
	err   error
}

func (f *fakeExtractor) IsDocument(name string) bool {
	n := strings.ToLower(name)
	for _, e := range []string{".docx", ".pptx", ".pdf", ".doc", ".ppt", ".xls"} {
		if strings.HasSuffix(n, e) {
			return true
		}
	}
	return false
}

func (f *fakeExtractor) Extract(_ context.Context, name string, data []byte) (*Extracted, error) {
	f.calls.Add(1)
	f.name, f.seen = name, append([]byte(nil), data...)
	if f.err != nil {
		return nil, f.err
	}
	if f.res != nil {
		return f.res, nil
	}
	return &Extracted{Text: docText, Format: "pdf", Pages: 3, PagesWithoutText: 1}, nil
}

func docGuard(t *testing.T, w world, ex DocExtractor, maxText, maxDoc int64, roots ...string) *Guard {
	t.Helper()
	g := w.guard(t, maxText, roots...)
	if err := g.SetExtractor(ex, maxDoc); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestSetExtractorValidates(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 100)
	if err := g.SetExtractor(&fakeExtractor{}, 0); err == nil {
		t.Fatal("a document ceiling below 1 must be refused")
	}
	if err := g.SetExtractor(nil, 0); err != nil {
		t.Fatalf("nil extractor turns documents off: %v", err)
	}
	if g.MaxDocBytes() != 0 {
		t.Fatalf("MaxDocBytes without extractor = %d", g.MaxDocBytes())
	}
}

// Without an extractor the behaviour is the one before phase 7: a document is
// refused as binary (fail closed), a text file reads normally and carries
// Format "txt".
func TestDocWithoutExtractorFailsClosed(t *testing.T) {
	w := newWorld(t)
	g := w.guard(t, 1<<20)
	write(t, filepath.Join(w.root, "a.docx"), "PK\x03\x04\x00\x00binary")
	write(t, filepath.Join(w.root, "b.pdf"), "%PDF-1.4\n\x00\x01\x02")
	write(t, filepath.Join(w.root, "c.doc"), "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1\x00")
	for _, n := range []string{"a.docx", "b.pdf", "c.doc"} {
		_, err := g.Read(n)
		wantReason(t, err, ReasonBinary, n+" without extractor")
	}
	write(t, filepath.Join(w.root, "t.txt"), "olá\r\nmundo")
	f, err := g.Read("t.txt")
	if err != nil {
		t.Fatal(err)
	}
	if f.Text != "olá\nmundo" || f.Format != "txt" || f.Pages != 0 || f.Slides != 0 || f.PendingRevisions {
		t.Fatalf("text file: %+v", f)
	}
	// the document ceiling does not exist without an extractor
	small := w.guard(t, 10)
	write(t, filepath.Join(w.root, "big.pdf"), strings.Repeat("a", 50))
	_, err = small.Read("big.pdf")
	wantReason(t, err, ReasonTooLarge, "pdf over the text ceiling, no extractor")
	if !strings.Contains(err.Error(), "--max-file-bytes") {
		t.Errorf("message should name --max-file-bytes: %v", err)
	}
}

func TestDocExtractedFieldsAndBytes(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{res: &Extracted{Text: "a\r\nb", Format: "docx", PendingRevisions: true}}
	g := docGuard(t, w, fx, 100, 1000)
	body := "PK\x03\x04 fake docx body"
	write(t, filepath.Join(w.root, "Contrato.DOCX"), body)
	f, err := g.Read("Contrato.DOCX")
	if err != nil {
		t.Fatal(err)
	}
	if f.Text != "a\nb" || f.Format != "docx" || !f.PendingRevisions || f.Bytes != len(body) || f.Name != "Contrato.DOCX" {
		t.Fatalf("file: %+v", f)
	}
	if !bytes.Equal(fx.seen, []byte(body)) || fx.name != "Contrato.DOCX" || fx.calls.Load() != 1 {
		t.Fatalf("extractor saw name=%q calls=%d", fx.name, fx.calls.Load())
	}
	// a text file never reaches the extractor
	write(t, filepath.Join(w.root, "n.txt"), "plain")
	if f, err = g.Read("n.txt"); err != nil || f.Format != "txt" || fx.calls.Load() != 1 {
		t.Fatalf("text through the extractor? %+v %v calls=%d", f, err, fx.calls.Load())
	}
}

// The ceiling depends on the extension of the resolved name: text keeps the
// text ceiling, a document gets the document one, and above it the file is
// refused before the extractor sees a byte.
func TestDocCeilingByExtension(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.root, "mid.pdf"), strings.Repeat("x", 3000))
	if _, err := g.Read("mid.pdf"); err != nil {
		t.Fatalf("3000-byte pdf under a 5000 document ceiling: %v", err)
	}
	write(t, filepath.Join(w.root, "mid.txt"), strings.Repeat("x", 3000))
	_, err := g.Read("mid.txt")
	wantReason(t, err, ReasonTooLarge, "3000-byte txt over the 1000 text ceiling")
	if !strings.Contains(err.Error(), "--max-file-bytes") {
		t.Errorf("text message: %v", err)
	}
	before := fx.calls.Load()
	write(t, filepath.Join(w.root, "huge.pptx"), strings.Repeat("x", 6000))
	_, err = g.Read("huge.pptx")
	wantReason(t, err, ReasonTooLarge, "6000-byte pptx over the 5000 document ceiling")
	if !strings.Contains(err.Error(), "--max-doc-bytes") {
		t.Errorf("document message: %v", err)
	}
	if fx.calls.Load() != before {
		t.Fatal("the extractor ran on a document over the ceiling")
	}
	noLeak(t, w, err)
}

// The real defaults: a PDF between 4 MiB and 32 MiB is accepted, one above
// 32 MiB is refused.
func TestDocCeilingRealDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 33 MiB")
	}
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 4<<20, 32<<20)
	write(t, filepath.Join(w.root, "five.pdf"), strings.Repeat("p", 5<<20))
	if f, err := g.Read("five.pdf"); err != nil || f.Bytes != 5<<20 {
		t.Fatalf("5 MiB pdf: %+v %v", f, err)
	}
	write(t, filepath.Join(w.root, "five.txt"), strings.Repeat("p", 5<<20))
	_, err := g.Read("five.txt")
	wantReason(t, err, ReasonTooLarge, "5 MiB txt")
	before := fx.calls.Load()
	write(t, filepath.Join(w.root, "big.pdf"), strings.Repeat("p", 32<<20+1))
	_, err = g.Read("big.pdf")
	wantReason(t, err, ReasonTooLarge, "32 MiB + 1 pdf")
	if fx.calls.Load() != before {
		t.Fatal("extractor ran above 32 MiB")
	}
}

// The denylist runs before any read: the extractor is never called and the
// answer is the denylist refusal, even for documents.
func TestDocDenylistBeforeRead(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	for _, rel := range []string{
		filepath.Join("secrets", "contrato.docx"),
		"credentials.docx",
		"my-credential-list.pdf",
		".env.docx",
		filepath.Join(".git", "x.pdf"),
		"id_rsa.pdf",
	} {
		write(t, filepath.Join(w.root, rel), "PK\x03\x04 "+docSecretContent)
		_, err := g.Read(rel)
		wantReason(t, err, ReasonDenylisted, rel)
		noLeak(t, w, err)
		if strings.Contains(err.Error(), docSecretContent) {
			t.Errorf("%s leaks content", rel)
		}
	}
	if fx.calls.Load() != 0 {
		t.Fatalf("the extractor ran %d times on denylisted names", fx.calls.Load())
	}
}

// A symlink with an innocent .docx name pointing at a denylisted file or at a
// file outside the root: refused, extractor not called.
func TestDocSymlinkAndHardlink(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.outside, "out.docx"), "PK\x03\x04"+outsideContent)
	write(t, filepath.Join(w.root, ".env"), docSecretContent)

	if err := os.Symlink(filepath.Join(w.outside, "out.docx"), filepath.Join(w.root, "link-out.docx")); err != nil {
		t.Logf("symlinks unavailable here (%v): the symlink half is covered where they can be created", err)
	} else {
		_, err := g.Read("link-out.docx")
		wantReason(t, err, ReasonNotAllowed, "symlink .docx to outside")
		noLeak(t, w, err)
		if err := os.Symlink(filepath.Join(w.root, ".env"), filepath.Join(w.root, "innocent.docx")); err == nil {
			_, err = g.Read("innocent.docx")
			wantReason(t, err, ReasonDenylisted, "symlink .docx to .env")
		}
	}

	// hardlink (works without privileges): a second name for a file outside
	if err := os.Link(filepath.Join(w.outside, "out.docx"), filepath.Join(w.root, "hard.docx")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	_, err := g.Read("hard.docx")
	wantReason(t, err, ReasonNotAllowed, "hardlink .docx to a file outside the root")
	noLeak(t, w, err)
	// and a hardlink to a denylisted file inside the root
	if err := os.Link(filepath.Join(w.root, ".env"), filepath.Join(w.root, "hard2.pdf")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	_, err = g.Read("hard2.pdf")
	wantReason(t, err, ReasonNotAllowed, "hardlink .pdf to .env")
	if fx.calls.Load() != 0 {
		t.Fatalf("the extractor ran %d times behind a link", fx.calls.Load())
	}
}

func TestDocJunction(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.outside, "out.docx"), "PK\x03\x04"+outsideContent)
	mklinkJunction(t, filepath.Join(w.root, "jout"), w.outside)
	_, err := g.Read(filepath.Join("jout", "out.docx"))
	wantReason(t, err, ReasonNotAllowed, "docx through a junction to outside")
	noLeak(t, w, err)
	if fx.calls.Load() != 0 {
		t.Fatal("the extractor ran through a junction")
	}
}

// The extractor's refusal reaches the caller as a ReasonDocument refusal with
// its own message; a legacy .doc gets the 97-2003 explanation, not "binary".
func TestDocExtractorRefusal(t *testing.T) {
	w := newWorld(t)
	const legacy = "formato Word/PowerPoint 97-2003 não é suportado; salve como .docx ou .pptx"
	fx := &fakeExtractor{err: errors.New(legacy)}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.root, "velho.doc"), "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1\x00\x00")
	_, err := g.Read("velho.doc")
	wantReason(t, err, ReasonDocument, "legacy .doc")
	if err.Error() != legacy {
		t.Fatalf("message = %q", err)
	}
	noLeak(t, w, err)
}

// A name the extractor claims whose content is plain text (notes.doc) reads as
// text, under the text ceiling, exactly as before phase 7.
func TestDocClaimedButText(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{err: ErrNotDocument}
	g := docGuard(t, w, fx, 100, 5000)
	write(t, filepath.Join(w.root, "notes.doc"), "texto simples\r\nlinha 2")
	f, err := g.Read("notes.doc")
	if err != nil || f.Text != "texto simples\nlinha 2" || f.Format != "txt" {
		t.Fatalf("%+v %v", f, err)
	}
	write(t, filepath.Join(w.root, "long.doc"), strings.Repeat("y", 300))
	_, err = g.Read("long.doc")
	wantReason(t, err, ReasonTooLarge, "text posing as .doc over the text ceiling")
	if !strings.Contains(err.Error(), "--max-file-bytes") {
		t.Errorf("message: %v", err)
	}
}

func TestDocContext(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.root, "a.pdf"), "%PDF-1.4 x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.ReadContext(ctx, "a.pdf")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: %v", err)
	}
	if fx.calls.Load() != 0 {
		t.Fatal("the extractor ran on a cancelled context")
	}
	// cancelled while extracting: the context error wins over the extractor's
	ctx2, cancel2 := context.WithCancel(context.Background())
	fx2 := &cancelingExtractor{cancel: cancel2}
	g2 := docGuard(t, w, fx2, 1000, 5000)
	_, err = g2.ReadContext(ctx2, "a.pdf")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled during extraction: %v", err)
	}
	var r *Refusal
	if errors.As(err, &r) {
		t.Fatalf("a cancellation must not become a refusal: %v", err)
	}
}

type cancelingExtractor struct{ cancel context.CancelFunc }

func (c *cancelingExtractor) IsDocument(string) bool { return true }
func (c *cancelingExtractor) Extract(ctx context.Context, _ string, _ []byte) (*Extracted, error) {
	c.cancel()
	return nil, errors.New("o extrator desistiu")
}

// A name ending in dots or spaces opens the file without them on Windows; the
// document rules must see the same extension, so the PDF cannot be read as
// text by hiding behind "x.pdf.".
func TestDocTrailingDotName(t *testing.T) {
	if os.PathSeparator != '\\' {
		t.Skip("trailing dots are dropped by Windows only")
	}
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.root, "x.pdf"), "%PDF-1.4 plain ascii body")
	for _, req := range []string{"x.pdf.", "x.pdf ", "x.pdf. ."} {
		f, err := g.Read(req)
		if err != nil {
			t.Logf("%q: refused (%v)", req, err)
			continue
		}
		if f.Format != "pdf" {
			t.Errorf("%q was read as %q instead of going to the extractor", req, f.Format)
		}
	}
}

// Windows special paths with a document extension stay refused by the syntax
// check, before any I/O.
func TestDocSpecialPaths(t *testing.T) {
	w := newWorld(t)
	fx := &fakeExtractor{}
	g := docGuard(t, w, fx, 1000, 5000)
	write(t, filepath.Join(w.root, "a.pdf"), "%PDF-1.4 x")
	for _, req := range []string{
		`\\?\` + filepath.Join(w.root, "a.pdf"),
		`\\.\` + filepath.Join(w.root, "a.pdf"),
		`\\srv\share\a.pdf`,
		filepath.Join(w.root, "a.pdf") + ":stream",
		filepath.Join(w.root, "a.pdf") + ":stream:$DATA",
		"a.pdf:stream",
		"C:a.pdf",
		"CON.pdf",
		"nul.docx",
		"COM1.pdf",
		"a*.pdf",
		"a?.docx",
	} {
		_, err := g.Read(req)
		if err == nil {
			t.Errorf("%q must be refused", req)
			continue
		}
		var r *Refusal
		if !errors.As(err, &r) {
			t.Errorf("%q: not a refusal: %v", req, err)
		}
	}
	if fx.calls.Load() != 0 {
		t.Fatal("the extractor ran for a special path")
	}
}
