package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/ocr"
)

// OCR wiring (step 7.9). Package doctext only knows the small doctext.OCR
// interface; ocrRuntime adapts the ocr.Pool (worker child processes) to it.
// OCR is opt-in (--ocr-lib, D-7) and never stops the start: a bad setting, a
// hash mismatch or a DLL that does not load leaves the server up without OCR
// and logs a WARNING.

// ocrProbeTimeout bounds the start-time check of the DLL and the models
// (a worker starts, loads them and answers its hello). It runs in the
// background: the MCP handshake never waits for it.
const ocrProbeTimeout = 60 * time.Second

// ocrConfig resolves the --ocr-* flags into the configuration of package
// ocr. ok is false when OCR is off (no --ocr-lib); err explains why the
// given settings cannot be used. It does no I/O.
func (c Config) ocrConfig() (cfg ocr.Config, ok bool, err error) {
	o := c.OCR
	if o.Lib == "" {
		return ocr.Config{}, false, nil
	}
	if !filepath.IsAbs(o.Lib) {
		return ocr.Config{}, true, errors.New("--ocr-lib must be an absolute path")
	}
	if o.LibSHA256 == "" {
		return ocr.Config{}, true, errors.New("--ocr-lib-sha256 is required with --ocr-lib")
	}
	td := o.Tessdata
	if td == "" {
		td = filepath.Join(filepath.Dir(o.Lib), "tessdata", "best") // D-11: best by default
	} else if !filepath.IsAbs(td) {
		return ocr.Config{}, true, errors.New("--ocr-tessdata must be an absolute path")
	}
	switch {
	case o.Workers < 1 || o.Workers > ocr.MaxWorkers:
		return ocr.Config{}, true, fmt.Errorf("--ocr-workers must be between 1 and %d", ocr.MaxWorkers)
	case o.PageTimeout <= 0:
		return ocr.Config{}, true, errors.New("--ocr-page-timeout must be positive")
	case o.MaxPages < 1:
		return ocr.Config{}, true, errors.New("--ocr-max-pages must be positive")
	case o.MaxMem < ocr.MinMaxMem:
		return ocr.Config{}, true, fmt.Errorf("--ocr-max-mem must be at least %d", ocr.MinMaxMem)
	case o.MaxMegapixels < 1 || o.MaxMegapixels > ocr.MaxMaxMegapixels:
		return ocr.Config{}, true, fmt.Errorf("--ocr-max-megapixels must be between 1 and %d", ocr.MaxMaxMegapixels)
	case o.DPI < ocr.MinDPI || o.DPI > ocr.MaxDPI:
		return ocr.Config{}, true, fmt.Errorf("--ocr-dpi must be between %d and %d", ocr.MinDPI, ocr.MaxDPI)
	}
	cfg = ocr.Config{
		Lib:           o.Lib,
		LibSHA256:     o.LibSHA256,
		Tessdata:      td,
		Workers:       o.Workers,
		PageTimeout:   o.PageTimeout,
		MaxMem:        o.MaxMem,
		MaxMegapixels: o.MaxMegapixels,
		DPI:           o.DPI,
	}
	if err := cfg.Validate(); err != nil {
		return ocr.Config{}, true, err
	}
	return cfg, true, nil
}

// ocrRuntime is the OCR of a running server: the pool (children started on
// demand) plus the options doctext gets on every extraction.
type ocrRuntime struct {
	pool ocrEngine
	opts doctext.OCROptions // without Engine
	off  atomic.Bool        // set when the start-time probe failed
	done chan struct{}      // closed when the probe finished
	once sync.Once
}

// ocrEngine is what ocrRuntime needs of the pool (*ocr.Pool; fakes in tests).
type ocrEngine interface {
	Recognize(ctx context.Context, img *image.Gray, dpi int) (ocr.Page, error)
	Close() error
}

// newPoolFn is ocr.NewPool; tests replace it.
var newPoolFn = ocr.NewPool

// setupOCR turns the OCR on when --ocr-lib is set. It checks the settings,
// the DLL hash and the models synchronously (milliseconds) and probes a
// worker in the background with a throwaway pool, so that the serving pool
// starts its children only on demand. It returns nil (and logs a WARNING
// when --ocr-lib was given) whenever OCR cannot be used.
func setupOCR(c Config, lg *logger) *ocrRuntime {
	cfg, ok, err := c.ocrConfig()
	if !ok {
		return nil
	}
	warn := func(err error) {
		lg.logf("WARNING: OCR disabled: %s; the file tools run without OCR (scanned PDFs and images are refused)", err)
	}
	if err != nil {
		warn(err)
		return nil
	}
	pool, err := newPoolFn(cfg)
	if err != nil {
		warn(err)
		return nil
	}
	rt := &ocrRuntime{
		pool: pool,
		opts: doctext.OCROptions{
			DPI:           c.OCR.DPI,
			MaxMegapixels: c.OCR.MaxMegapixels,
			MaxPages:      c.OCR.MaxPages,
			Workers:       c.OCR.Workers,
		},
		done: make(chan struct{}),
	}
	models := filepath.Base(cfg.Tessdata)
	go func() {
		defer close(rt.done)
		probe, err := newPoolFn(cfg)
		var version string
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), ocrProbeTimeout)
			version, err = probe.Probe(ctx)
			cancel()
			_ = probe.Close()
		}
		if err != nil {
			rt.off.Store(true)
			warn(err)
			return
		}
		lg.logf("OCR enabled: Tesseract %s, lang %s, models %s, workers %d, dpi %d, max_pages %d, max_megapixels %d",
			version, ocr.DefaultLang, models, c.OCR.Workers, c.OCR.DPI, c.OCR.MaxPages, c.OCR.MaxMegapixels)
	}()
	return rt
}

// options is what doctext gets per extraction: nil when OCR is off.
func (r *ocrRuntime) options() *doctext.OCROptions {
	if r == nil || r.off.Load() {
		return nil
	}
	o := r.opts
	o.Engine = r
	return &o
}

// Recognize implements doctext.OCR over the pool. The pool's "cannot go on"
// errors become doctext.ErrOCRUnavailable; context errors pass through; any
// other error is a failure of that page.
func (r *ocrRuntime) Recognize(ctx context.Context, img *image.Gray, dpi int) (string, int, int, error) {
	p, err := r.pool.Recognize(ctx, img, dpi)
	if err != nil {
		if errors.Is(err, ocr.ErrUnavailable) || errors.Is(err, ocr.ErrClosed) || errors.Is(err, ocr.ErrUnsupportedPlatform) {
			return "", 0, 0, fmt.Errorf("%w: %w", doctext.ErrOCRUnavailable, err)
		}
		return "", 0, 0, err
	}
	return p.Text, p.Conf, p.Rotated, nil
}

// Close stops the workers (after the probe, so none is left behind).
func (r *ocrRuntime) Close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
		}
		_ = r.pool.Close()
	})
}

// ocrWorkerMain is the internal "--ocr-worker" mode (D-9): the pool starts
// this same executable with it. It is not in the public --help, refuses a
// terminal on stdin (WorkerStdio), reads only its own arguments (never the
// environment, never --api-key, nothing of Ollama), loads the DLL, answers
// frames on the original stdout and exits when stdin ends.
func ocrWorkerMain(args []string, stderr io.Writer) int {
	cfg, err := ocr.ParseWorkerArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+err.Error())
		return 2
	}
	in, out, err := ocr.WorkerStdio()
	if err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+err.Error())
		return 2
	}
	if err := ocr.RunWorker(in, out, cfg); err != nil {
		fmt.Fprintln(stderr, "mcpollama-mcp: "+err.Error())
		return 1
	}
	return 0
}
