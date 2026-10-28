package ocr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"os"
)

// rotateGrayCW returns src turned clockwise by deg (90, 180 or 270), as a new
// image with origin (0, 0). Any other angle returns src unchanged.
func rotateGrayCW(src *image.Gray, deg int) *image.Gray {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	at := func(x, y int) byte { return src.Pix[src.PixOffset(src.Rect.Min.X+x, src.Rect.Min.Y+y)] }
	switch deg {
	case 90:
		dst := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Pix[x*dst.Stride+(h-1-y)] = at(x, y)
			}
		}
		return dst
	case 180:
		dst := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Pix[(h-1-y)*dst.Stride+(w-1-x)] = at(x, y)
			}
		}
		return dst
	case 270:
		dst := image.NewGray(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Pix[(w-1-x)*dst.Stride+y] = at(x, y)
			}
		}
		return dst
	}
	return src
}

// sha256Reader hashes at most maxLibBytes of r; a longer input is refused.
func sha256Reader(r io.Reader) (string, error) {
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(r, maxLibBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxLibBytes {
		return "", fmt.Errorf("%w: biblioteca do OCR grande demais", ErrConfig)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkLibFile verifies that path is a regular file whose SHA-256 is want.
// The worker repeats the check right before loading (tesseract_windows.go);
// this one lets the pool refuse early with a clear message.
func checkLibFile(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: biblioteca do OCR não encontrada ou ilegível", ErrConfig)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: a biblioteca do OCR não é um arquivo regular", ErrConfig)
	}
	got, err := sha256Reader(f)
	if err != nil {
		return fmt.Errorf("%w: não foi possível calcular o SHA-256 da biblioteca do OCR", ErrConfig)
	}
	if got != want {
		return fmt.Errorf("%w: o SHA-256 da biblioteca do OCR não confere com o esperado", ErrConfig)
	}
	return nil
}
