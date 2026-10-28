package doctext

// OCR of PDF pages without text and of images (step 7.9, brief section 11).
//
// The package does not know the engine: the caller injects an OCR through
// Limits.OCR (the binary adapts package ocr, whose workers run in child
// processes). doctext only decides WHICH pages go to it, renders them, skips
// the blank ones, keeps the time budget and the order, and writes the
// markers. It still never opens a file, logs or reads the environment.

import (
	"context"
	"errors"
	"image"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OCR recognizes the text of one 8-bit grayscale page image rendered at dpi.
// conf is the mean confidence (0-100) and rotated the clockwise rotation, in
// degrees, the engine applied before reading (0 when none).
//
// An error that wraps ErrOCRUnavailable means the OCR cannot be used for the
// rest of the call; an error of the context means the page ran out of time;
// any other error is a failure of that page only. Implementations must be
// safe for concurrent use: up to OCROptions.Workers pages are in flight.
type OCR interface {
	Recognize(ctx context.Context, img *image.Gray, dpi int) (text string, conf int, rotated int, err error)
}

// ErrOCRUnavailable is wrapped by an OCR error when the OCR cannot go on in
// this call (its worker died twice, it was closed): the extraction is refused
// with ReasonOCRUnavailable.
var ErrOCRUnavailable = errors.New("doctext: OCR indisponível nesta chamada")

// OCROptions configures the OCR of one extraction. Zero fields mean the
// defaults; a nil Engine disables the OCR.
type OCROptions struct {
	Engine        OCR
	DPI           int // render resolution of a PDF page, default 300
	MaxMegapixels int // pixels of one page image, default 40 MP (the dpi is reduced to fit)
	MaxPages      int // PDF pages sent to the OCR in one call, default 30
	Workers       int // pages in flight at once, default 1
}

// OCR defaults and fixed thresholds.
const (
	DefaultOCRDPI           = 300
	DefaultOCRMaxMegapixels = 40
	DefaultOCRMaxPages      = 30

	// ocrLowConf: below it the marker says "baixa confiança".
	ocrLowConf = 60
	// ocrWhite: a page with no pixel darker than this is blank (brief 11.2).
	ocrWhite = 200
	// ocrMaxDimension: the largest width or height the engine accepts
	// (Tesseract keeps coordinates in 16-bit integers).
	ocrMaxDimension = 32767
	// ocrScreenDPI renders the pages of a long scanned PDF once, small, only
	// to tell blank pages from the ones that count against MaxPages.
	ocrScreenDPI = 50
	// ocrMinImageDPI is the lowest resolution declared for a reduced image.
	ocrMinImageDPI = 70
)

func (o OCROptions) withDefaults() OCROptions {
	if o.DPI <= 0 {
		o.DPI = DefaultOCRDPI
	}
	if o.MaxMegapixels <= 0 {
		o.MaxMegapixels = DefaultOCRMaxMegapixels
	}
	if o.MaxPages <= 0 {
		o.MaxPages = DefaultOCRMaxPages
	}
	if o.Workers <= 0 {
		o.Workers = 1
	}
	return o
}

func (o OCROptions) maxPixels() float64 { return float64(o.MaxMegapixels) * 1e6 }

// ocrStatus is the outcome of one page.
type ocrStatus int

const (
	ocrPending ocrStatus = iota
	ocrOK
	ocrFailed
	ocrTimedOut
	ocrBlank
	ocrUnavailable
)

// ocrPage is the result of one page (or of the image).
type ocrPage struct {
	status  ocrStatus
	text    string // normalized and trimmed (ocrOK only)
	conf    int
	rotated int
}

// ocrMarker is the line that opens the text of a page that went to the OCR:
// "[página 3 — OCR conf=93]", "[página 3 — OCR baixa confiança conf=41 girada=180]",
// "[página 3 — OCR falhou]", "[página 3 — OCR não processado: tempo esgotado]".
// label is "página N" or "imagem".
func ocrMarker(label string, p ocrPage) string {
	switch p.status {
	case ocrOK:
	case ocrTimedOut:
		return "[" + label + " — OCR não processado: tempo esgotado]"
	default:
		return "[" + label + " — OCR falhou]"
	}
	var b strings.Builder
	b.WriteString("[" + label + " — OCR ")
	if p.conf < ocrLowConf {
		b.WriteString("baixa confiança ")
	}
	b.WriteString("conf=" + strconv.Itoa(p.conf))
	if p.rotated != 0 {
		b.WriteString(" girada=" + strconv.Itoa(p.rotated))
	}
	b.WriteString("]")
	return b.String()
}

// ocrBudget derives the time budget of the OCR from ctx: it ends a little
// before ctx does (1/20 of the time left, between 200 ms and 5 s), so that
// the pages still running are marked "tempo esgotado" and the extraction
// returns what it has instead of failing as a whole.
func ocrBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	dl, ok := ctx.Deadline()
	if !ok {
		return context.WithCancel(ctx)
	}
	left := time.Until(dl)
	reserve := left / 20
	if reserve < 200*time.Millisecond {
		reserve = 200 * time.Millisecond
	}
	if reserve > 5*time.Second {
		reserve = 5 * time.Second
	}
	return context.WithDeadline(ctx, dl.Add(-reserve))
}

// recognize runs one page through the engine and classifies the outcome. A
// panic of the engine (in-process fakes; the real one runs in a child
// process) fails the page instead of the server.
func recognize(octx context.Context, eng OCR, img *image.Gray, dpi int) (p ocrPage) {
	defer func() {
		if r := recover(); r != nil {
			p = ocrPage{status: ocrFailed}
		}
	}()
	text, conf, rotated, err := eng.Recognize(octx, img, dpi)
	switch {
	case err != nil && errors.Is(err, ErrOCRUnavailable):
		return ocrPage{status: ocrUnavailable}
	case err != nil && octx.Err() != nil:
		return ocrPage{status: ocrTimedOut}
	case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)):
		return ocrPage{status: ocrTimedOut}
	case err != nil:
		return ocrPage{status: ocrFailed}
	}
	norm, err := NormalizeText(text)
	if err != nil { // more than 1% U+FFFD: the page text is not trusted
		return ocrPage{status: ocrFailed}
	}
	if conf < 0 {
		conf = 0
	}
	if conf > 100 {
		conf = 100
	}
	switch rotated {
	case 0, 90, 180, 270:
	default:
		rotated = 0
	}
	return ocrPage{status: ocrOK, text: strings.TrimSpace(norm), conf: conf, rotated: rotated}
}

// isBlankImage reports whether no pixel of img is darker than ocrWhite.
func isBlankImage(img *image.Gray) bool {
	w := img.Rect.Dx()
	for y := 0; y < img.Rect.Dy(); y++ {
		off := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y)
		for _, v := range img.Pix[off : off+w] {
			if v < ocrWhite {
				return false
			}
		}
	}
	return true
}

// errPageTooLarge: no resolution makes the page fit the pixel ceiling.
var errPageTooLarge = errors.New("doctext: página grande demais para OCR")

// fitDPI returns the highest dpi <= want at which a page of wPt x hPt points
// fits maxPixels and the dimension ceiling (a giant page is reduced, never
// refused, brief 11.6). Sizes are rounded up, so the real render is never
// larger than computed.
func fitDPI(wPt, hPt float64, want int, maxPixels float64) (int, error) {
	if !(wPt > 0 && hPt > 0) || math.IsInf(wPt, 0) || math.IsInf(hPt, 0) || want < 1 {
		return 0, errPageTooLarge
	}
	for d := want; d >= 1; {
		w := math.Ceil(wPt*float64(d)/72) + 1
		h := math.Ceil(hPt*float64(d)/72) + 1
		if w*h <= maxPixels && w <= ocrMaxDimension && h <= ocrMaxDimension {
			return d, nil
		}
		f := math.Min(math.Sqrt(maxPixels/(w*h)), ocrMaxDimension/math.Max(w, h))
		next := int(float64(d) * f)
		if next >= d {
			next = d - 1
		}
		d = next
	}
	return 0, errPageTooLarge
}

// ocrPDF recognizes the pages idx (zero-based, all without text) of an open
// document. Rendering is sequential (the PDF engine is single-threaded and
// the caller holds its slot); recognition runs in up to o.Workers goroutines
// and the results keep the page order. Pages left when the budget ends are
// ocrTimedOut. The error is a refusal (too many pages, OCR unavailable) or a
// context error of ctx itself.
func ocrPDF(ctx context.Context, doc pdfDoc, idx []int, o OCROptions) ([]ocrPage, error) {
	res := make([]ocrPage, len(idx))
	octx, cancel := ocrBudget(ctx)
	defer cancel()

	if len(idx) > o.MaxPages {
		// Only pages with ink count: render small to tell the blank ones.
		inked := 0
		for k, i := range idx {
			if err := ctxErr(ctx); err != nil {
				return nil, err
			}
			blank, err := screenBlank(octx, doc, i, o)
			if err == nil && blank {
				res[k].status = ocrBlank
				continue
			}
			if octx.Err() != nil {
				break // no time left even to count: the full pass marks the rest
			}
			if inked++; inked > o.MaxPages {
				return nil, refuse(ReasonOCRTooManyPages)
			}
		}
	}

	var (
		wg  sync.WaitGroup
		sem = make(chan struct{}, o.Workers)
	)
	for k, i := range idx {
		if res[k].status == ocrBlank {
			continue
		}
		if octx.Err() != nil {
			res[k].status = ocrTimedOut
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-octx.Done():
			res[k].status = ocrTimedOut
			continue
		}
		img, dpi, st := renderForOCR(octx, doc, i, o)
		if st != ocrPending {
			res[k].status = st
			<-sem
			continue
		}
		wg.Add(1)
		go func(k int, img *image.Gray, dpi int) {
			defer wg.Done()
			defer func() { <-sem }()
			p := recognize(octx, o.Engine, img, dpi)
			if p.status == ocrUnavailable {
				cancel() // the rest cannot be recognized either
			}
			res[k] = p
		}(k, img, dpi)
	}
	wg.Wait()
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	for _, p := range res {
		if p.status == ocrUnavailable {
			return nil, refuse(ReasonOCRUnavailable)
		}
	}
	return res, nil
}

// renderForOCR renders page i at the configured dpi, reduced to fit the pixel
// ceiling. It returns ocrPending with the image when the page must go to the
// engine, or the final status (blank, failed, timed out).
func renderForOCR(octx context.Context, doc pdfDoc, i int, o OCROptions) (*image.Gray, int, ocrStatus) {
	wPt, hPt, err := doc.pageSize(i)
	if err != nil {
		return nil, 0, ocrFailed
	}
	dpi, err := fitDPI(wPt, hPt, o.DPI, o.maxPixels())
	if err != nil {
		return nil, 0, ocrFailed
	}
	img, err := doc.renderGray(octx, i, dpi)
	if err != nil || img == nil {
		if octx.Err() != nil {
			return nil, 0, ocrTimedOut
		}
		return nil, 0, ocrFailed
	}
	if float64(img.Rect.Dx())*float64(img.Rect.Dy()) > o.maxPixels() || img.Rect.Empty() {
		return nil, 0, ocrFailed
	}
	if isBlankImage(img) {
		return nil, 0, ocrBlank
	}
	return img, dpi, ocrPending
}

// screenBlank renders page i small and reports whether it is blank.
func screenBlank(octx context.Context, doc pdfDoc, i int, o OCROptions) (bool, error) {
	wPt, hPt, err := doc.pageSize(i)
	if err != nil {
		return false, err
	}
	dpi, err := fitDPI(wPt, hPt, ocrScreenDPI, o.maxPixels())
	if err != nil {
		return false, err
	}
	img, err := doc.renderGray(octx, i, dpi)
	if err != nil || img == nil {
		return false, errors.New("doctext: renderização falhou")
	}
	return isBlankImage(img), nil
}

// ocrStats sums the outcomes of the pages that went to the OCR.
func ocrStats(pages []ocrPage) OCRStats {
	s := OCRStats{Used: len(pages) > 0}
	sum := 0
	for _, p := range pages {
		switch p.status {
		case ocrOK:
			s.Pages++
			sum += p.conf
		case ocrFailed:
			s.Failed++
		case ocrTimedOut:
			s.NotProcessed++
		case ocrBlank:
			s.Blank++
		}
	}
	if s.Pages > 0 {
		s.ConfMean = int(math.Round(float64(sum) / float64(s.Pages)))
	}
	return s
}
