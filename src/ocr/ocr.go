// Package ocr recognizes text in grayscale page images (phase 7, step 7.9).
//
// The engine is Tesseract (libtesseract64.dll, called through its C API
// without cgo). Native code can crash or corrupt memory, and Go's recover does
// not catch that, so the server never loads the DLL itself: a Pool starts
// child processes in the internal "--ocr-worker" mode (RunWorker) and talks to
// them over a small binary protocol on stdin/stdout (protocol.go). On Windows
// each child lives in a Job Object with a memory ceiling and
// KILL_ON_JOB_CLOSE. Other platforms answer ErrUnsupportedPlatform (D-10).
//
// The package never reads flags or the environment for its configuration: the
// caller passes a Config. It never opens a document, never touches the
// network and never writes to disk. Messages are in pt-BR and carry neither a
// document path nor any recognized text.
package ocr

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Engine recognizes the text of one page image. dpi is the resolution the
// page was rendered at (0 means the configured default). Implementations:
// *Pool (child processes, the production path), *Tesseract (in-process,
// used by the worker) and fakes in the tests.
type Engine interface {
	Recognize(ctx context.Context, img *image.Gray, dpi int) (Page, error)
}

// Page is the result of one recognized page.
type Page struct {
	Text string // UTF-8 as returned by the engine (not normalized here)
	Conf int    // mean word confidence, 0-100
	// Rotated is the clockwise rotation, in degrees, that the engine applied
	// to the page before the pass whose text is returned: 0, or 180 when the
	// page was upside down. Pages turned 90 or 270 degrees are read by
	// Tesseract's layout analysis without being reported here (a 270 page
	// reads as 180 + 90 and reports 180). The protocol carries 0/90/180/270.
	Rotated int
}

// Defaults and hard limits.
const (
	DefaultLang          = "por"
	DefaultPageTimeout   = 60 * time.Second
	DefaultMaxMem        = int64(1 << 30) // per worker process (Job Object)
	DefaultMaxMegapixels = 40             // A4 at 300 dpi is about 8.7 MP
	DefaultDPI           = 300

	MaxWorkers       = 16
	MinMaxMem        = int64(32 << 20)
	MaxMaxMegapixels = 200
	MinDPI           = 1
	MaxDPI           = 1200
	// MaxDimension is the largest width or height accepted (Tesseract keeps
	// coordinates in 16-bit integers).
	MaxDimension = 32767
	// MaxStridePad is how far a row stride may exceed the width on the wire.
	MaxStridePad = 64
	// MaxTextBytes caps the recognized text of one page (a dense A4 page is
	// a few KiB).
	MaxTextBytes = 1 << 20
	// maxLibBytes caps the DLL size read for the hash check.
	maxLibBytes = 256 << 20
)

// Errors. A failure confined to one page is a *PageError (the document goes
// on); ErrUnavailable means OCR cannot be used for the rest of this call.
var (
	ErrUnsupportedPlatform = errors.New("ocr: OCR indisponível nesta plataforma")
	ErrUnavailable         = errors.New("ocr: OCR indisponível nesta chamada")
	ErrClosed              = errors.New("ocr: OCR encerrado")
	ErrPageTimeout         = errors.New("ocr: tempo da página esgotado")
	ErrWorkerCrashed       = errors.New("ocr: o processo de OCR terminou inesperadamente")
	ErrBadImage            = errors.New("ocr: imagem inválida para OCR")
	ErrImageTooLarge       = errors.New("ocr: imagem grande demais para OCR")
	ErrTextTooLarge        = errors.New("ocr: texto reconhecido grande demais")
	ErrEngine              = errors.New("ocr: falha do motor de OCR na página")
	ErrConfig              = errors.New("ocr: configuração inválida")
)

// PageError reports that one page could not be recognized; the caller marks
// the page as failed and carries on with the next one.
type PageError struct {
	Err error
}

func (e *PageError) Error() string { return e.Err.Error() }
func (e *PageError) Unwrap() error { return e.Err }

// Config is the OCR configuration. Zero values mean the defaults.
type Config struct {
	Lib           string        // absolute path of libtesseract64.dll
	LibSHA256     string        // expected SHA-256 of Lib, 64 hex digits
	Tessdata      string        // absolute directory with <lang>.traineddata
	Lang          string        // Tesseract language, default "por"
	Workers       int           // child processes, default min(NumCPU/2, 4), at least 1
	PageTimeout   time.Duration // per page, default 60 s
	MaxMem        int64         // bytes per worker process, default 1 GiB (Windows Job Object)
	MaxMegapixels int           // per page, default 40
	DPI           int           // default render resolution, 300
}

// DefaultWorkers is min(NumCPU/2, 4), at least 1.
func DefaultWorkers() int {
	n := runtime.NumCPU() / 2
	if n > 4 {
		n = 4
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (c Config) withDefaults() Config {
	if c.Lang == "" {
		c.Lang = DefaultLang
	}
	if c.Workers == 0 {
		c.Workers = DefaultWorkers()
	}
	if c.PageTimeout == 0 {
		c.PageTimeout = DefaultPageTimeout
	}
	if c.MaxMem == 0 {
		c.MaxMem = DefaultMaxMem
	}
	if c.MaxMegapixels == 0 {
		c.MaxMegapixels = DefaultMaxMegapixels
	}
	if c.DPI == 0 {
		c.DPI = DefaultDPI
	}
	c.LibSHA256 = strings.ToLower(strings.TrimSpace(c.LibSHA256))
	return c
}

// Validate checks the configuration with the defaults applied: absolute
// paths, a 64-hex-digit hash and every number in range.
func (c Config) Validate() error {
	c = c.withDefaults()
	if err := c.WorkerConfig().validate(); err != nil {
		return err
	}
	if c.Workers < 1 || c.Workers > MaxWorkers {
		return configErr("workers fora do intervalo 1..%d", MaxWorkers)
	}
	if c.PageTimeout < 0 {
		return configErr("tempo por página negativo")
	}
	if c.MaxMem < MinMaxMem {
		return configErr("memória máxima do worker abaixo de %d MiB", MinMaxMem>>20)
	}
	return nil
}

// WorkerConfig is the part of Config that a worker process needs.
func (c Config) WorkerConfig() WorkerConfig {
	c = c.withDefaults()
	return WorkerConfig{
		Lib:           c.Lib,
		LibSHA256:     c.LibSHA256,
		Tessdata:      c.Tessdata,
		Lang:          c.Lang,
		DPI:           c.DPI,
		MaxMegapixels: c.MaxMegapixels,
	}
}

// WorkerConfig configures RunWorker and OpenTesseract.
type WorkerConfig struct {
	Lib           string
	LibSHA256     string
	Tessdata      string
	Lang          string
	DPI           int // used when a request carries dpi 0
	MaxMegapixels int
}

func (w WorkerConfig) withDefaults() WorkerConfig {
	if w.Lang == "" {
		w.Lang = DefaultLang
	}
	if w.DPI == 0 {
		w.DPI = DefaultDPI
	}
	if w.MaxMegapixels == 0 {
		w.MaxMegapixels = DefaultMaxMegapixels
	}
	w.LibSHA256 = strings.ToLower(strings.TrimSpace(w.LibSHA256))
	return w
}

var (
	hexSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	langRE    = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}(\+[A-Za-z0-9_]{1,32}){0,7}$`)
)

func (w WorkerConfig) validate() error {
	w = w.withDefaults()
	if err := checkAbsPath(w.Lib, "biblioteca do OCR"); err != nil {
		return err
	}
	if !hexSHA256.MatchString(w.LibSHA256) {
		return configErr("o SHA-256 da biblioteca do OCR deve ter 64 dígitos hexadecimais")
	}
	if err := checkAbsPath(w.Tessdata, "pasta tessdata"); err != nil {
		return err
	}
	for _, r := range w.Tessdata {
		if r > 0x7e || r < 0x20 {
			// Tesseract opens the models with fopen (ANSI code page).
			return configErr("a pasta tessdata deve ter um caminho só com caracteres ASCII")
		}
	}
	if !langRE.MatchString(w.Lang) {
		return configErr("idioma do OCR inválido")
	}
	if w.DPI < MinDPI || w.DPI > MaxDPI {
		return configErr("dpi fora do intervalo %d..%d", MinDPI, MaxDPI)
	}
	if w.MaxMegapixels < 1 || w.MaxMegapixels > MaxMaxMegapixels {
		return configErr("megapixels máximos fora do intervalo 1..%d", MaxMaxMegapixels)
	}
	return nil
}

func (w WorkerConfig) maxPixels() int64 { return int64(w.MaxMegapixels) * 1_000_000 }

// checkAbsPath refuses relative paths and network or device paths: the DLL
// and the models must come from a local, absolute location (never PATH, never
// the current directory).
func checkAbsPath(p, what string) error {
	if p == "" {
		return configErr("%s não informada", what)
	}
	if strings.ContainsRune(p, 0) {
		return configErr("%s: caminho inválido", what)
	}
	if !filepath.IsAbs(p) {
		return configErr("%s: o caminho deve ser absoluto", what)
	}
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, `//`) {
		return configErr("%s: caminho de rede ou de dispositivo não é aceito", what)
	}
	return nil
}

func configErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConfig, fmt.Sprintf(format, a...))
}

// Worker command line. The pool builds it with WorkerArgs and the worker
// entry point parses it with ParseWorkerArgs, so the flag names live in one
// place. The API key is never part of it.
const (
	WorkerFlag           = "--ocr-worker"
	flagLib              = "ocr-lib"
	flagLibSHA256        = "ocr-lib-sha256"
	flagTessdata         = "ocr-tessdata"
	flagLang             = "ocr-lang"
	flagDPI              = "ocr-dpi"
	flagMaxMegapixels    = "ocr-max-megapixels"
	maxWorkerArgsEntries = 16
)

// WorkerArgs returns the arguments (after the program name) that start a
// worker process for cfg.
func WorkerArgs(cfg WorkerConfig) []string {
	cfg = cfg.withDefaults()
	return []string{
		WorkerFlag,
		"--" + flagLib, cfg.Lib,
		"--" + flagLibSHA256, cfg.LibSHA256,
		"--" + flagTessdata, cfg.Tessdata,
		"--" + flagLang, cfg.Lang,
		"--" + flagDPI, strconv.Itoa(cfg.DPI),
		"--" + flagMaxMegapixels, strconv.Itoa(cfg.MaxMegapixels),
	}
}

// ParseWorkerArgs parses the arguments given (after the program name); the
// first one must be WorkerFlag. It reads nothing else: no os.Args, no
// environment.
func ParseWorkerArgs(args []string) (WorkerConfig, error) {
	if len(args) == 0 || args[0] != WorkerFlag {
		return WorkerConfig{}, configErr("argumentos do worker de OCR inválidos")
	}
	if len(args) > maxWorkerArgsEntries {
		return WorkerConfig{}, configErr("argumentos do worker de OCR em excesso")
	}
	fs := flag.NewFlagSet("ocr-worker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var w WorkerConfig
	fs.StringVar(&w.Lib, flagLib, "", "")
	fs.StringVar(&w.LibSHA256, flagLibSHA256, "", "")
	fs.StringVar(&w.Tessdata, flagTessdata, "", "")
	fs.StringVar(&w.Lang, flagLang, DefaultLang, "")
	fs.IntVar(&w.DPI, flagDPI, DefaultDPI, "")
	fs.IntVar(&w.MaxMegapixels, flagMaxMegapixels, DefaultMaxMegapixels, "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return WorkerConfig{}, configErr("argumentos do worker de OCR inválidos")
	}
	w = w.withDefaults()
	if err := w.validate(); err != nil {
		return WorkerConfig{}, err
	}
	return w, nil
}

// checkModels verifies that every language model of cfg exists as a regular
// file in the tessdata directory.
func checkModels(cfg WorkerConfig) error {
	for _, l := range strings.Split(cfg.Lang, "+") {
		fi, err := os.Stat(filepath.Join(cfg.Tessdata, l+".traineddata"))
		if err != nil || !fi.Mode().IsRegular() {
			return configErr("modelo do idioma %q não encontrado na pasta tessdata", l)
		}
	}
	return nil
}

// checkImage validates a page image against the limits, before any copy.
func checkImage(img *image.Gray, maxPixels int64) error {
	if img == nil {
		return ErrBadImage
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 || img.Stride < w {
		return ErrBadImage
	}
	if w > MaxDimension || h > MaxDimension || int64(w)*int64(h) > maxPixels {
		return ErrImageTooLarge
	}
	off := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
	need := int64(off) + int64(img.Stride)*int64(h-1) + int64(w)
	if off < 0 || need > int64(len(img.Pix)) {
		return ErrBadImage
	}
	return nil
}
