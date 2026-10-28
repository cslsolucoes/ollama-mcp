package doctext

// PDF text extraction (phase 7.5, brief section 4.3).
//
// The engine is PDFium compiled to WebAssembly (github.com/klippa-app/go-pdfium
// in WebAssembly mode, running on the pure Go wazero runtime: no cgo). It is
// hidden behind pdfEngine/pdfDoc so that the marker, hyphen and limit logic is
// tested without it and the engine can be swapped.
//
// Lifecycle: the engine is a single resident instance, started lazily by the
// first PDF (never at server start: the compilation of the .wasm costs
// seconds) and kept until ClosePDF. There is no idle release. Every extraction
// holds a one-slot semaphore, so PDFs are processed one at a time, and gets a
// fresh PDFium instance from the pool, closed at the end: no state survives
// from one document to the next.
//
// The package still never reads the environment or a flag. The only way to
// persist the wazero compilation cache is PDFOptions.CacheDir, chosen by the
// caller before the first PDF (ConfigurePDF). With a CacheDir wazero writes its
// cache there (about 22 MB): that is the single exception of this package to
// "no disk I/O", and it is configured by the caller, never implied. Without a
// CacheDir nothing is written.
//
// The WebAssembly module gets no file system, and its stdout/stderr are
// discarded: stdout belongs to the JSON-RPC stream of the server.

import (
	"context"
	"errors"
	"image"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/klippa-app/go-pdfium"
	pdferrors "github.com/klippa-app/go-pdfium/errors"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// pdfMinPageRunes is the brief's threshold: a page with fewer non-blank runes
// than this counts as "without text" (a scanned page, a lone page number).
const pdfMinPageRunes = 20

// pdfMaxWasmPages caps the linear memory of the PDFium module (64 KiB pages):
// without it wazero allows 4 GiB, and a 33 KB PDF with a 16 MiB Flate content
// stream took the process to +5 GiB. A document that needs more is refused.
const pdfMaxWasmPages = 8192 // 512 MiB

// PDFOptions configures the PDF engine. The zero value means "no cache".
type PDFOptions struct {
	// CacheDir is the directory of the wazero compilation cache. Empty means
	// no cache and nothing is written to disk. With it, the first start of the
	// engine still compiles the module (seconds) and fills the cache; the next
	// processes start in a fraction of a second.
	CacheDir string
}

// pdfEngine opens PDF documents. Implementations need not be safe for
// concurrent use: extractPDF serializes them.
type pdfEngine interface {
	open(ctx context.Context, data []byte) (pdfDoc, error)
}

// pdfDoc is one open document. close must be called exactly once.
type pdfDoc interface {
	pages() int
	// pageText returns the plain text of page i (zero-based), in reading order
	// as the engine sees it. Line endings are unspecified (\r\n is common).
	pageText(ctx context.Context, i int) (string, error)
	// renderGray renders page i (zero-based) at the given dpi as an 8-bit
	// grayscale image. The pixels belong to the caller.
	renderGray(ctx context.Context, i int, dpi int) (*image.Gray, error)
	close()
}

// errPDFPassword is what an engine returns when the document needs a password
// to open (an empty user password is tried first by the engine).
var errPDFPassword = errors.New("pdf: senha necessária")

// pdfState is the process-wide state of the resident engine.
var pdfState = struct {
	sem  chan struct{} // one slot: held while the engine is configured, started, used or closed
	opts PDFOptions
	eng  *pdfiumEngine // nil until the first PDF
	test pdfEngine     // set by tests: replaces the real engine
}{sem: make(chan struct{}, 1)}

// lockPDF acquires the engine slot, honoring ctx.
func lockPDF(ctx context.Context) error {
	select {
	case pdfState.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctxErr(ctx)
	}
}

func unlockPDF() { <-pdfState.sem }

// ConfigurePDF sets the PDF engine options. It must be called before the first
// PDF is processed (or after ClosePDF). Once the engine has started it changes
// nothing and returns an error, so a late call is never silently ignored.
func ConfigurePDF(opts PDFOptions) error {
	pdfState.sem <- struct{}{}
	defer unlockPDF()
	if pdfState.eng != nil {
		return errors.New("doctext: o motor de PDF já foi iniciado; ConfigurePDF deve vir antes do primeiro PDF")
	}
	pdfState.opts = opts
	return nil
}

// ClosePDF stops the resident engine and releases its memory (about 300 MB
// while it runs). It is for the server shutdown and for tests. The engine
// restarts lazily if a PDF comes afterwards, and the options set by
// ConfigurePDF are kept.
func ClosePDF() error {
	pdfState.sem <- struct{}{}
	defer unlockPDF()
	if pdfState.eng == nil {
		return nil
	}
	err := pdfState.eng.shutdown()
	pdfState.eng = nil
	return err
}

// currentPDFEngine returns the engine to use, starting the real one on the
// first call. The caller holds the engine slot.
func currentPDFEngine() (pdfEngine, error) {
	if pdfState.test != nil {
		return pdfState.test, nil
	}
	if pdfState.eng == nil {
		e, err := newPDFiumEngine(pdfState.opts)
		if err != nil {
			return nil, err
		}
		pdfState.eng = e
	}
	return pdfState.eng, nil
}

// extractPDF extracts the text of a PDF: "[página N]" plus the page text, for
// every page, in order. Extract applies NormalizeText and the final ceiling on
// top of this.
func extractPDF(ctx context.Context, data []byte, lim Limits) (res *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, refuse(ReasonMalformed)
		}
	}()
	if err := lockPDF(ctx); err != nil {
		return nil, err
	}
	defer unlockPDF()
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	eng, err := currentPDFEngine()
	if err != nil {
		return nil, pdfOpenError(ctx, err)
	}
	doc, err := eng.open(ctx, data)
	if err != nil {
		return nil, pdfOpenError(ctx, err)
	}
	defer doc.close()

	n := doc.pages()
	if n <= 0 || n > lim.MaxPDFPages {
		return nil, refuse(ReasonMalformed)
	}
	var b strings.Builder
	total, blank := 0, 0
	for i := 0; i < n; i++ {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		raw, err := doc.pageText(ctx, i)
		if err != nil {
			if cerr := ctxErr(ctx); cerr != nil {
				return nil, cerr
			}
			return nil, refuse(ReasonMalformed)
		}
		text := strings.TrimSpace(unifyNewlines(raw))
		if countNonBlank(text) < pdfMinPageRunes {
			blank++
		}
		total += utf8.RuneCountInString(text)
		if total > lim.MaxTextRunes {
			return nil, refuse(ReasonTooLong)
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[página " + strconv.Itoa(i+1) + "]")
		if text != "" {
			b.WriteString("\n" + text)
		}
	}
	if blank == n {
		return nil, refuse(ReasonNoText)
	}
	return &Result{Text: b.String(), Pages: n, PagesWithoutText: blank}, nil
}

// pdfOpenError maps an engine failure at open time to a typed refusal.
func pdfOpenError(ctx context.Context, err error) error {
	if cerr := ctxErr(ctx); cerr != nil {
		return cerr
	}
	if errors.Is(err, errPDFPassword) {
		return refuse(ReasonPDFPassword)
	}
	return refuse(ReasonMalformed)
}

func unifyNewlines(s string) string {
	if strings.IndexByte(s, '\r') < 0 {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func countNonBlank(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// pdfiumSoftHyphens undoes a quirk of PDFium's text extraction. When a line ends
// with a hyphen that follows a letter or digit, PDFium swallows the line break
// and emits U+0002 in place of the hyphen, whatever the next character is.
//
// The hyphen is NEVER dropped (owner decision after 7.7a B6): PDFium emits the
// same U+0002 for a syllable break ("con-" / "trato") and for a real hyphen
// that fell at the end of a line ("obriga-" / "se", "ex-" / "empregado"); it
// was measured that neither its text nor FPDFText_GetUnicode tell U+002D from
// a soft hyphen U+00AD apart. Dropping it turned "obriga-se" into "obrigase".
// So the hyphen always stays: before a lowercase letter the line is joined
// ("obriga-se", and also "con-trato"); otherwise the line break is put back
// ("Rio-", newline, "São"). A "-" at the end of a line that PDFium did not
// mark (after a space, "R$ 10 -") is left as it is, with its line break.
func pdfiumSoftHyphens(s string) string {
	if !strings.Contains(s, "\x02") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		if s[i] != 0x02 {
			b.WriteByte(s[i])
			continue
		}
		next, _ := utf8.DecodeRuneInString(s[i+1:])
		if unicode.IsLower(next) {
			b.WriteByte('-')
		} else {
			b.WriteString("-\n")
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// The real engine: PDFium on wazero.
// ---------------------------------------------------------------------------

type pdfiumEngine struct {
	pool  pdfium.Pool
	cache wazero.CompilationCache // nil without CacheDir
}

func newPDFiumEngine(opts PDFOptions) (*pdfiumEngine, error) {
	// The bundled module lowers setjmp/longjmp to WebAssembly exception
	// handling, which wazero needs enabled explicitly.
	rc := wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
		WithMemoryLimitPages(pdfMaxWasmPages).
		// Kill (in open) cancels the worker context; this makes wazero abort
		// the WebAssembly call in flight instead of letting the page run on.
		WithCloseOnContextDone(true)
	e := &pdfiumEngine{}
	if opts.CacheDir != "" {
		c, err := wazero.NewCompilationCacheWithDir(opts.CacheDir)
		if err != nil {
			return nil, err
		}
		e.cache = c
		rc = rc.WithCompilationCache(c)
	}
	pool, err := webassembly.Init(webassembly.Config{
		MinIdle:       1,
		MaxIdle:       1,
		MaxTotal:      1,
		RuntimeConfig: rc,
		FSConfig:      wazero.NewFSConfig(), // no mounts: go-pdfium would mount the whole drive by default
		Stdout:        io.Discard,
		Stderr:        io.Discard,
	})
	if err != nil {
		if e.cache != nil {
			_ = e.cache.Close(context.Background())
		}
		return nil, err
	}
	e.pool = pool
	return e, nil
}

func (e *pdfiumEngine) shutdown() error {
	err := e.pool.Close()
	if e.cache != nil {
		if cerr := e.cache.Close(context.Background()); err == nil {
			err = cerr
		}
	}
	return err
}

func (e *pdfiumEngine) open(ctx context.Context, data []byte) (pdfDoc, error) {
	inst, err := e.pool.GetInstanceWithContext(ctx)
	if err != nil {
		return nil, err
	}
	// The deadline must cut a single long call (one hostile page took 17 s
	// with a 2 s timeout): when ctx ends, the instance is killed.
	stop := context.AfterFunc(ctx, func() { _ = inst.Kill() })
	empty := "" // permission-only protection has an empty user password
	d, err := inst.OpenDocument(&requests.OpenDocument{File: &data, Password: &empty})
	if err != nil {
		stop()
		_ = inst.Close()
		if errors.Is(err, pdferrors.ErrPassword) {
			return nil, errPDFPassword
		}
		return nil, err
	}
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: d.Document})
	if err != nil {
		stop()
		_ = inst.Close()
		return nil, err
	}
	return &pdfiumDoc{inst: inst, ref: d.Document, n: pc.PageCount, stop: stop}, nil
}

type pdfiumDoc struct {
	inst pdfium.Pdfium
	ref  references.FPDF_DOCUMENT
	n    int
	stop func() bool // releases the ctx watchdog of open
}

func (d *pdfiumDoc) pages() int { return d.n }

func (d *pdfiumDoc) page(i int) requests.Page {
	return requests.Page{ByIndex: &requests.PageByIndex{Document: d.ref, Index: i}}
}

func (d *pdfiumDoc) pageText(ctx context.Context, i int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r, err := d.inst.GetPageText(&requests.GetPageText{Page: d.page(i)})
	if err != nil {
		return "", err
	}
	return pdfiumSoftHyphens(r.Text), nil
}

func (d *pdfiumDoc) renderGray(ctx context.Context, i int, dpi int) (*image.Gray, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := d.inst.RenderPageInDPI(&requests.RenderPageInDPI{
		Page:        d.page(i),
		DPI:         dpi,
		ImageFormat: requests.RenderImageFormatGrayscale,
	})
	if err != nil {
		return nil, err
	}
	defer r.Cleanup() // the pixel buffer is only valid until Cleanup
	g, ok := r.Result.RenderedImage.(*image.Gray)
	if !ok || g == nil {
		return nil, errors.New("pdf: a renderização não devolveu uma imagem em tons de cinza")
	}
	out := image.NewGray(g.Rect)
	for y := 0; y < g.Rect.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+g.Rect.Dx()], g.Pix[y*g.Stride:y*g.Stride+g.Rect.Dx()])
	}
	return out, nil
}

func (d *pdfiumDoc) close() {
	defer func() { _ = recover() }()
	if d.stop != nil {
		d.stop()
	}
	_, _ = d.inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.ref})
	_ = d.inst.Close()
}

// pdfRenderPage renders page i (zero-based) of a PDF as an 8-bit grayscale
// image at the given dpi. It is the hook for the OCR stage (7.9) and has no
// caller yet. It goes through the same resident engine and slot as the text
// extraction, and the limits on pages apply.
func pdfRenderPage(ctx context.Context, data []byte, lim Limits, i, dpi int) (img *image.Gray, err error) {
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, refuse(ReasonMalformed)
		}
	}()
	lim = lim.withDefaults()
	if dpi < 1 || dpi > 1200 {
		return nil, errors.New("doctext: dpi fora do intervalo 1..1200")
	}
	if err := lockPDF(ctx); err != nil {
		return nil, err
	}
	defer unlockPDF()
	eng, err := currentPDFEngine()
	if err != nil {
		return nil, pdfOpenError(ctx, err)
	}
	doc, err := eng.open(ctx, data)
	if err != nil {
		return nil, pdfOpenError(ctx, err)
	}
	defer doc.close()
	if n := doc.pages(); n <= 0 || n > lim.MaxPDFPages || i < 0 || i >= n {
		return nil, refuse(ReasonMalformed)
	}
	img, err = doc.renderGray(ctx, i, dpi)
	if err != nil {
		if cerr := ctxErr(ctx); cerr != nil {
			return nil, cerr
		}
		return nil, refuse(ReasonMalformed)
	}
	return img, nil
}
