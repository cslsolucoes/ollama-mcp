//go:build !windows

package ocr

import (
	"context"
	"image"
	"io"
	"os/exec"
)

// OCR is Windows only for now (D-10): elsewhere every entry point answers
// ErrUnsupportedPlatform.

const platformSupported = false

// Tesseract is unavailable on this platform.
type Tesseract struct{}

// OpenTesseract answers ErrUnsupportedPlatform on this platform.
func OpenTesseract(WorkerConfig) (*Tesseract, error) { return nil, ErrUnsupportedPlatform }

// Version is empty on this platform.
func (*Tesseract) Version() string { return "" }

// OSD is false on this platform.
func (*Tesseract) OSD() bool { return false }

// Recognize answers ErrUnsupportedPlatform on this platform.
func (*Tesseract) Recognize(context.Context, *image.Gray, int) (Page, error) {
	return Page{}, ErrUnsupportedPlatform
}

// Close does nothing on this platform.
func (*Tesseract) Close() error { return nil }

// WorkerStdio answers ErrUnsupportedPlatform on this platform.
func WorkerStdio() (io.Reader, io.Writer, error) { return nil, nil, ErrUnsupportedPlatform }

type jobObject struct{}

func newJob(int64) (*jobObject, error) { return nil, ErrUnsupportedPlatform }
func (*jobObject) prepare(*exec.Cmd)   {}
func (*jobObject) adopt(int) error     { return ErrUnsupportedPlatform }
func (*jobObject) close()              {}
