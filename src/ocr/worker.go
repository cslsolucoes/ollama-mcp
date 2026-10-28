package ocr

import (
	"bufio"
	"context"
	"errors"
	"io"
)

// workerEngine is the engine a worker process drives.
type workerEngine interface {
	Engine
	Close() error
}

// openEngine opens the engine of a worker and returns its version.
type openEngine func(WorkerConfig) (workerEngine, string, error)

// RunWorker is the body of the internal "--ocr-worker" mode: it loads the
// engine once, answers a hello frame, then reads page requests from in and
// writes one response per request to out until in ends. It reads no file
// (besides the DLL and the models), opens no network connection and writes
// nothing but frames to out. The caller should first detach the process
// standard output from out with WorkerStdio, so that native code printing to
// stdout cannot corrupt the frames.
func RunWorker(in io.Reader, out io.Writer, cfg WorkerConfig) error {
	return runWorker(in, out, cfg, func(c WorkerConfig) (workerEngine, string, error) {
		t, err := OpenTesseract(c)
		if err != nil {
			return nil, "", err
		}
		return t, t.Version(), nil
	})
}

func runWorker(in io.Reader, out io.Writer, cfg WorkerConfig, open openEngine) error {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		_ = writeResponse(out, frameResponse{kind: kindHello, status: statusInit, text: shortMessage(err.Error())})
		return err
	}
	eng, version, err := open(cfg)
	if err != nil {
		_ = writeResponse(out, frameResponse{kind: kindHello, status: statusInit, text: shortMessage(err.Error())})
		return err
	}
	defer eng.Close()
	if err := writeResponse(out, frameResponse{kind: kindHello, status: statusOK, text: shortMessage(version)}); err != nil {
		return err
	}
	br := bufio.NewReaderSize(in, 64<<10)
	maxPixels := cfg.maxPixels()
	for {
		req, err := readRequest(br, maxPixels)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			// The stream is out of sync: answer once and stop.
			_ = writeResponse(out, frameResponse{kind: kindResult, status: statusBadFrame, text: shortMessage(err.Error())})
			return err
		}
		dpi := req.dpi
		if dpi == 0 {
			dpi = cfg.DPI
		}
		page, err := eng.Recognize(context.Background(), req.image(), dpi)
		resp := frameResponse{kind: kindResult}
		switch {
		case errors.Is(err, ErrImageTooLarge), errors.Is(err, ErrBadImage):
			resp.status, resp.text = statusBadImage, shortMessage(err.Error())
		case err != nil:
			resp.status, resp.text = statusEngine, shortMessage(err.Error())
		case len(page.Text) > MaxTextBytes:
			resp.status, resp.text = statusTooLarge, shortMessage(ErrTextTooLarge.Error())
		default:
			resp.text, resp.conf, resp.rotated = page.Text, clampConf(page.Conf), page.Rotated
			if !validRotation(resp.rotated) {
				resp.rotated = 0
			}
		}
		if err := writeResponse(out, resp); err != nil {
			return err
		}
	}
}

func clampConf(c int) int {
	if c < 0 {
		return 0
	}
	if c > 100 {
		return 100
	}
	return c
}
