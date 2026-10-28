package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
)

// maxDocTextRunes caps the text one document may yield: the work the file tools
// can do is MaxChunks model calls of MaxPromptChars each, so more text than
// that could never be processed anyway. 16 Mi runes is doctext's own ceiling.
const maxDocTextRunes = 16 << 20

// headWindow is how many leading bytes doctext.Detect gets: it needs the ZIP
// and OLE signatures (first bytes) and the PDF header (first KiB).
const headWindow = 4096

// docExtractor adapts package doctext to files.DocExtractor, so package files
// never imports it. It holds no state besides the limits; the PDF engine is
// the process-wide resident one of doctext and starts on the first PDF.
type docExtractor struct {
	lim     doctext.Limits
	timeout time.Duration
	// logf receives metadata only (format, pages, bytes, ms); nil disables it.
	// Never the text, the name, the author or any document metadata.
	logf func(format string, args ...any)
}

func newDocExtractor(c Config, logf func(string, ...any)) *docExtractor {
	lim := doctext.DefaultLimits()
	n := int64(c.MaxPromptChars) * int64(c.MaxChunks)
	if n > 0 && n < int64(maxDocTextRunes) {
		lim.MaxTextRunes = int(n)
	}
	return &docExtractor{lim: lim, timeout: c.Timeout, logf: logf}
}

// IsDocument claims the document extensions and the legacy ones that doctext
// only refuses with an explanation (.doc, .ppt, .xls).
func (e *docExtractor) IsDocument(name string) bool {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return false
	}
	switch strings.ToLower(name[i:]) {
	case ".docx", ".pptx", ".pdf", ".doc", ".ppt", ".xls":
		return true
	}
	return false
}

func (e *docExtractor) Extract(ctx context.Context, name string, data []byte) (*files.Extracted, error) {
	head := data
	if len(head) > headWindow {
		head = head[:headWindow]
	}
	format, err := doctext.Detect(name, head)
	if err != nil {
		return nil, userSafe(err)
	}
	if format == doctext.FormatText {
		// A .doc/.ppt/.xls that is not OLE (a text file with an old extension).
		return nil, files.ErrNotDocument
	}
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	start := time.Now()
	res, err := doctext.Extract(ctx, format, data, e.lim)
	if err != nil {
		return nil, userSafe(err)
	}
	if e.logf != nil {
		e.logf("doc: format=%s bytes=%d pages=%d slides=%d ms=%d",
			res.Format, len(data), res.Pages, res.Slides, time.Since(start).Milliseconds())
	}
	return &files.Extracted{
		Text:             res.Text,
		Format:           res.Format.String(),
		Pages:            res.Pages,
		Slides:           res.Slides,
		PagesWithoutText: res.PagesWithoutText,
		PendingRevisions: res.PendingRevisions,
	}, nil
}

// userSafe lets only doctext's own refusals (pt-BR, no path, no content) and
// context errors through; anything else is replaced by the generic refusal, so
// an unexpected parser error can never carry document text to the client.
func userSafe(err error) error {
	if _, ok := doctext.ReasonOf(err); ok {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &doctext.Error{Reason: doctext.ReasonMalformed}
}

// setupDocs prepares the PDF engine options at start. It does NOT start the
// engine: the WASM runtime is created on the first PDF (start stays instant).
// With --pdf-cache-dir the directory is created if missing; without it nothing
// is ever written to disk.
func setupDocs(c Config) error {
	if c.PDFCacheDir != "" {
		if err := os.MkdirAll(c.PDFCacheDir, 0o700); err != nil {
			return fmt.Errorf("--pdf-cache-dir: cannot create the directory")
		}
	}
	return doctext.ConfigurePDF(doctext.PDFOptions{CacheDir: c.PDFCacheDir})
}
