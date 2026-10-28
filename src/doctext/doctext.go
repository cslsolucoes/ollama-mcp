// Package doctext extracts plain text from office documents (phase 7): DOCX,
// PPTX and PDF, plus PNG and JPEG images through an injected OCR (step 7.9,
// ocr.go and image.go). This file is the common base: formats, limits, results, the
// typed refusals and the format detection. zipsafe.go opens OOXML packages
// defensively, xmlsafe.go wraps the XML decoder, normalize.go cleans the text.
//
// The package is pure, in the same spirit as package files: it receives bytes
// that the guard already read, and it never opens a file, writes to disk,
// touches the network, reads the environment or logs. It knows nothing about
// Ollama, MCP or prompts. Every message is in pt-BR and carries neither a path
// nor any piece of the document (MCPO-REQ-SEC-011, FILE-014).
package doctext

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// Format is the kind of document a name and its first bytes agree on.
type Format int

const (
	// FormatText is "not a supported document": the caller keeps the plain
	// text path (which refuses binary content, as before phase 7).
	FormatText Format = iota
	FormatDOCX
	FormatPPTX
	FormatPDF
	// FormatPNG and FormatJPEG are images: their only text comes from the OCR
	// (Limits.OCR); without it they are refused with ReasonOCRRequired.
	FormatPNG
	FormatJPEG
)

// String is the value of the "formato=" field of the technical footer.
func (f Format) String() string {
	switch f {
	case FormatDOCX:
		return "docx"
	case FormatPPTX:
		return "pptx"
	case FormatPDF:
		return "pdf"
	case FormatPNG:
		return "png"
	case FormatJPEG:
		return "jpg"
	default:
		return "txt"
	}
}

// Limits bounds what a hostile document can make the process do. A zero field
// means "the default" (never "unlimited"): see DefaultLimits.
type Limits struct {
	MaxEntries    int   // entries in the ZIP central directory
	MaxEntryBytes int64 // real decompressed bytes of one entry
	MaxTotalBytes int64 // real decompressed bytes of every entry read from one package
	MaxXMLDepth   int   // element nesting
	MaxTextRunes  int   // character data accepted from one XML part (and the extracted text)
	MaxPDFPages   int   // pages of a PDF
	// OCR enables the recognition of PDF pages without text and of images.
	// nil means no OCR: a PDF without any text is refused with ReasonNoText
	// (as before step 7.9) and an image with ReasonOCRRequired.
	OCR *OCROptions
}

// DefaultLimits are the values of brief section 3.2. MaxTextRunes has no fixed
// value in the brief (the caller derives it from --max-chunks); 16 Mi runes is
// a ceiling that keeps a lone call from exhausting memory.
func DefaultLimits() Limits {
	return Limits{
		MaxEntries:    2000,
		MaxEntryBytes: 64 << 20,
		MaxTotalBytes: 256 << 20,
		MaxXMLDepth:   256,
		MaxTextRunes:  16 << 20,
		MaxPDFPages:   2000,
	}
}

// withDefaults fills every non-positive field with the default.
func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.MaxEntries > 0 {
		d.MaxEntries = l.MaxEntries
	}
	if l.MaxEntryBytes > 0 {
		d.MaxEntryBytes = l.MaxEntryBytes
	}
	if l.MaxTotalBytes > 0 {
		d.MaxTotalBytes = l.MaxTotalBytes
	}
	if l.MaxXMLDepth > 0 {
		d.MaxXMLDepth = l.MaxXMLDepth
	}
	if l.MaxTextRunes > 0 {
		d.MaxTextRunes = l.MaxTextRunes
	}
	if l.MaxPDFPages > 0 {
		d.MaxPDFPages = l.MaxPDFPages
	}
	if l.OCR != nil && l.OCR.Engine != nil {
		o := l.OCR.withDefaults()
		d.OCR = &o
	}
	return d
}

// Result is what an extraction returns. Text is UTF-8 without BOM, with LF
// line endings: the same contract as files.File.Text.
type Result struct {
	Text             string
	Format           Format
	Pages            int  // PDF
	Slides           int  // PPTX
	PagesWithoutText int  // PDF
	PendingRevisions bool // DOCX
	// ImportedContent: text of a w:altChunk (imported TXT, HTML or DOCX) is in
	// Text, at the place of the chunk (DOCX).
	ImportedContent bool
	// UnreadContent: some content Word shows could not be read (an altChunk
	// of an unsupported type or a broken one) and is NOT in Text (DOCX).
	UnreadContent bool
	// NumberingUnavailable: word/numbering.xml (or the numbering of
	// word/styles.xml) exists but could not be read: the text has no list
	// numbers, so clause numbers are missing (DOCX).
	NumberingUnavailable bool
	// HiddenText: Text has lines labelled "[oculto]" (text the document holds
	// but does not show or print: Word w:vanish, hidden PowerPoint shapes).
	HiddenText bool
	// OCR describes the recognition (PDF pages without text, images).
	OCR OCRStats
}

// OCRStats is the account of the OCR of one extraction. Used is false when no
// page went to the OCR (no OCR configured, or every page had text).
type OCRStats struct {
	Used         bool
	Pages        int // pages (or the image) recognized
	ConfMean     int // mean confidence of the recognized pages, 0-100
	Failed       int // pages whose recognition failed
	NotProcessed int // pages left out because the time budget ran out
	Blank        int // pages skipped as blank (no pixel darker than the threshold)
}

// labelHidden starts a line of hidden text (Word w:vanish, a hidden
// PowerPoint shape): the reader must know it does not show or print.
const labelHidden = "[oculto]"

// Reason classifies a refusal so callers and tests can tell the cases apart
// without parsing the (Portuguese) message.
type Reason int

const (
	// ReasonMismatch: a document extension whose bytes do not match it.
	ReasonMismatch Reason = iota + 1
	// ReasonPassworded: .docx/.pptx inside an OLE container (encrypted Office).
	ReasonPassworded
	// ReasonLegacy: .doc/.ppt in the OLE (97-2003) format.
	ReasonLegacy
	// ReasonLegacyExcel: .xls in the OLE (97-2003) format.
	ReasonLegacyExcel
	// ReasonPDFPassword: a PDF that needs a password to open.
	ReasonPDFPassword
	// ReasonNoText: a PDF with no text layer.
	ReasonNoText
	// ReasonMalformed: malformed ZIP/XML, a limit exceeded, a DTD, a duplicate
	// or unsafe entry. One opaque answer on purpose (never says which limit).
	ReasonMalformed
	// ReasonTooLong: the extracted text is over the ceiling.
	ReasonTooLong
	// ReasonCorruptText: more than 1% of the extracted text is U+FFFD.
	ReasonCorruptText
	// ReasonTimeout: the extraction ran out of time.
	ReasonTimeout
	// ReasonOCRRequired: an image, and the server has no OCR.
	ReasonOCRRequired
	// ReasonOCRTooManyPages: more pages need OCR than OCROptions.MaxPages.
	ReasonOCRTooManyPages
	// ReasonOCRUnavailable: the OCR could not be used in this call (its
	// worker died twice, or it was closed).
	ReasonOCRUnavailable
	// ReasonImageTooLarge: the image declares more pixels than can be decoded
	// safely (a decompression bomb, or simply too big).
	ReasonImageTooLarge
	// ReasonOCRNoText: OCR ran and no text came out of the whole document
	// (blank, illegible or failed pages, or no time left for them).
	ReasonOCRNoText
)

// Error is the typed refusal of this package. Its message is addressed to the
// user (pt-BR) and never carries a path or any document content.
type Error struct {
	Reason Reason
	// Format is set by Extract on a ReasonCorruptText, so the message can name
	// the right symptom; the zero value (FormatText) uses the PDF wording.
	Format Format
}

func (e *Error) Error() string {
	switch e.Reason {
	case ReasonMismatch:
		return "o conteúdo não corresponde à extensão do arquivo (.docx/.pptx/.pdf/.png/.jpg); o arquivo pode estar corrompido ou renomeado"
	case ReasonPassworded:
		return "documento do Office protegido por senha não é suportado; salve uma cópia sem senha"
	case ReasonLegacy:
		return "formato Word/PowerPoint 97-2003 não é suportado; salve como .docx ou .pptx"
	case ReasonLegacyExcel:
		return "formato Excel 97-2003 não é suportado; salve em um formato atual (.xlsx)"
	case ReasonPDFPassword:
		return "PDF protegido por senha não é suportado; gere uma cópia sem senha"
	case ReasonNoText:
		return "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado"
	case ReasonTooLong:
		return "documento extenso demais: o texto extraído passa do limite; divida o documento"
	case ReasonCorruptText:
		// The brief words this one for PDF (fonts without ToUnicode); DOCX and
		// PPTX get an equivalent wording, chosen by the format Extract recorded.
		if e.Format == FormatDOCX || e.Format == FormatPPTX || e.Format == FormatPNG || e.Format == FormatJPEG {
			return "o texto extraído do documento está corrompido (caracteres inválidos em excesso); não é possível resumi-lo com segurança"
		}
		return "o texto extraído do PDF está corrompido (fontes sem mapeamento de caracteres); não é possível resumi-lo com segurança"
	case ReasonTimeout:
		return "a extração do texto excedeu o tempo limite (--timeout)"
	case ReasonOCRRequired:
		return "imagem requer OCR, que não está habilitado no servidor"
	case ReasonOCRTooManyPages:
		return "documento digitalizado extenso demais para OCR nesta chamada (--ocr-max-pages); divida o documento"
	case ReasonOCRUnavailable:
		return "o OCR ficou indisponível nesta chamada (o processo de OCR terminou inesperadamente); tente de novo"
	case ReasonImageTooLarge:
		return "imagem grande demais para OCR (resolução acima do limite de segurança); reduza a resolução da imagem"
	case ReasonOCRNoText:
		return "nenhum texto foi obtido do documento, nem por OCR (páginas em branco, ilegíveis, com falha ou sem tempo para o OCR); confira o original"
	default:
		return "documento corrompido, malformado ou acima dos limites de segurança"
	}
}

func refuse(r Reason) *Error { return &Error{Reason: r} }

// ReasonOf returns the Reason of err (or any error it wraps) and whether err
// is a doctext refusal at all.
func ReasonOf(err error) (Reason, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason, true
	}
	return 0, false
}

var (
	zipMagic = []byte("PK\x03\x04")
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	pdfMagic = []byte("%PDF-")
	pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	jpgMagic = []byte{0xFF, 0xD8, 0xFF}
)

// pdfHeadWindow is how far into the file "%PDF-" may start: some producers
// put junk before the header.
const pdfHeadWindow = 1024

// extOf returns the lower-cased extension (with the dot) of the last path
// segment of name, accepting both separators.
func extOf(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	j := strings.LastIndexByte(name, '.')
	if j < 0 {
		return ""
	}
	return strings.ToLower(name[j:])
}

// Detect decides the format from the EXTENSION of the resolved name and from
// the signature in head (the first bytes of the file); both must agree. .png
// needs the PNG signature and .jpg/.jpeg the JPEG one (FF D8 FF). It
// never detects by content alone: any other extension is FormatText, with no
// error, and the caller follows the plain text path. .docm/.pptm/.dotx/.potx
// are deliberately FormatText (out of scope, refused as binary as before).
//
// Detect only sees head, so it cannot check that the ZIP really holds
// word/document.xml or ppt/presentation.xml: the extraction does that with
// Package.Has.
func Detect(name string, head []byte) (Format, error) {
	isOLE := bytes.HasPrefix(head, oleMagic)
	switch ext := extOf(name); ext {
	case ".docx", ".pptx":
		if isOLE {
			return FormatText, refuse(ReasonPassworded)
		}
		if !bytes.HasPrefix(head, zipMagic) {
			return FormatText, refuse(ReasonMismatch)
		}
		if ext == ".docx" {
			return FormatDOCX, nil
		}
		return FormatPPTX, nil
	case ".pdf":
		w := head
		if len(w) > pdfHeadWindow {
			w = w[:pdfHeadWindow]
		}
		if !bytes.Contains(w, pdfMagic) {
			return FormatText, refuse(ReasonMismatch)
		}
		return FormatPDF, nil
	case ".png":
		if !bytes.HasPrefix(head, pngMagic) {
			return FormatText, refuse(ReasonMismatch)
		}
		return FormatPNG, nil
	case ".jpg", ".jpeg":
		if !bytes.HasPrefix(head, jpgMagic) {
			return FormatText, refuse(ReasonMismatch)
		}
		return FormatJPEG, nil
	case ".doc", ".ppt":
		if isOLE {
			return FormatText, refuse(ReasonLegacy)
		}
	case ".xls":
		if isOLE {
			return FormatText, refuse(ReasonLegacyExcel)
		}
	}
	return FormatText, nil
}

// testHookExtract is called at the start of Extract, inside its recover scope.
// Tests use it to inject a panic and prove that it becomes ReasonMalformed.
var testHookExtract func(f Format)

// ctxErr maps a finished context to the error Extract returns: a deadline is
// the typed ReasonTimeout; a plain cancellation is returned as is, so callers
// can still match it with errors.Is(err, context.Canceled).
func ctxErr(ctx context.Context) error {
	err := ctx.Err()
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return refuse(ReasonTimeout)
	}
	return err
}

// Extract extracts the text of a document that the caller already read (and
// already detected with Detect). It is the single entry point of the package:
// it dispatches by format, recovers any panic of a parser into the opaque
// ReasonMalformed, honors ctx, applies NormalizeText to the final text and
// enforces lim.MaxTextRunes over the sum of every part of the document.
//
// FormatText is a usage error: the plain text path does not go through here.
func Extract(ctx context.Context, f Format, data []byte, lim Limits) (res *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, refuse(ReasonMalformed)
		}
	}()
	if testHookExtract != nil {
		testHookExtract(f)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lim = lim.withDefaults()
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	var r *Result
	switch f {
	case FormatDOCX:
		r, err = extractDOCX(ctx, data, lim)
	case FormatPPTX:
		r, err = extractPPTX(ctx, data, lim)
	case FormatPDF:
		r, err = extractPDF(ctx, data, lim)
	case FormatPNG, FormatJPEG:
		r, err = extractImage(ctx, f, data, lim)
	default:
		return nil, errors.New("formato sem extrator de documento: use o caminho de texto")
	}
	if err != nil {
		return nil, err
	}
	text, err := NormalizeText(r.Text)
	if err != nil {
		var de *Error
		if errors.As(err, &de) && de.Reason == ReasonCorruptText {
			return nil, &Error{Reason: de.Reason, Format: f}
		}
		return nil, err
	}
	if utf8.RuneCountInString(text) > lim.MaxTextRunes {
		return nil, refuse(ReasonTooLong)
	}
	r.Text = text
	r.Format = f
	return r, nil
}
