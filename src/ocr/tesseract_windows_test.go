//go:build windows

package ocr

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// The loading checks need no real DLL: every refusal happens before (or at)
// LoadLibraryEx.
func TestOpenTesseractRefusals(t *testing.T) {
	base := fakeConfig(t).WorkerConfig()
	cases := []struct {
		name string
		edit func(*WorkerConfig)
		want string
	}{
		{"relative path", func(w *WorkerConfig) { w.Lib = "libtesseract64.dll" }, "absoluto"},
		{"hash mismatch", func(w *WorkerConfig) { w.LibSHA256 = strings.Repeat("a", 64) }, "SHA-256"},
		{"missing DLL", func(w *WorkerConfig) { w.Lib = filepath.Join(filepath.Dir(w.Lib), "absent.dll") }, "não encontrada"},
		{"missing model", func(w *WorkerConfig) { w.Lang = "deu" }, "deu"},
		// Right hash, but the bytes are not a PE image: the loader refuses.
		{"not a DLL", nil, "não foi possível carregar"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := base
			if c.edit != nil {
				c.edit(&w)
			}
			tess, err := OpenTesseract(w)
			if err == nil {
				tess.Close()
				t.Fatal("accepted")
			}
			if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want ErrConfig with %q", err, c.want)
			}
		})
	}
}

func TestRotateGray(t *testing.T) {
	src := testImage(5, 3)
	for _, deg := range []int{90, 180, 270} {
		r := rotateGrayCW(src, deg)
		back := rotateGrayCW(r, (360-deg)%360)
		if back.Rect.Dx() != 5 || back.Rect.Dy() != 3 {
			t.Fatalf("%d: back to %v", deg, back.Rect)
		}
		for y := 0; y < 3; y++ {
			for x := 0; x < 5; x++ {
				if back.GrayAt(x, y) != src.GrayAt(x, y) {
					t.Fatalf("%d: pixel (%d,%d) changed", deg, x, y)
				}
			}
		}
	}
	// 90 clockwise: the top-left pixel goes to the top-right corner.
	r := rotateGrayCW(src, 90)
	if r.Rect.Dx() != 3 || r.Rect.Dy() != 5 || r.GrayAt(2, 0) != src.GrayAt(0, 0) {
		t.Fatal("90 degrees is not clockwise")
	}
	if rotateGrayCW(src, 45) != src {
		t.Fatal("45 degrees changed the image")
	}
}

// cStringFixture is a package variable so that it lives on the heap (Go
// does not move heap objects; a stack slice could move).
var cStringFixture = []byte("ação\x00rest")

func TestCString(t *testing.T) {
	p := uintptr(unsafe.Pointer(&cStringFixture[0]))
	if s, ok := cString(p, 64); !ok || s != "ação" {
		t.Fatalf("got %q %v", s, ok)
	}
	if _, ok := cString(p, 3); ok {
		t.Fatal("string longer than max accepted")
	}
	if s, ok := cString(0, 10); !ok || s != "" {
		t.Fatal("NULL pointer")
	}
	runtime.KeepAlive(cStringFixture)
}
