//go:build windows

package ocr

import (
	"context"
	"fmt"
	"image"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Tesseract C API constants.
const (
	psmAuto = 3 // PSM_AUTO
	// rotateBelowConf: a first pass under this mean confidence gets a second
	// pass with the page turned 180 degrees. Measured on a generated A4 page
	// (best model): upright 95, upside down 46, so the brief's 50 left no
	// margin; 60 is also the "low confidence" mark of §11.7.
	rotateBelowConf = 60
)

// tessProcs are the C API entry points, resolved once with GetProcAddress.
type tessProcs struct {
	version, create, init3, setPSM, setRes, setVar, setImage, recognize,
	getText, meanConf, deleteText, clear, end, del uintptr
}

// Tesseract is an in-process Tesseract instance. It is meant for the worker
// process: native code runs in the caller's address space. Calls are
// serialized (a TessBaseAPI is not thread-safe).
type Tesseract struct {
	mu        sync.Mutex
	dll       windows.Handle
	api       uintptr
	fn        tessProcs
	version   string
	maxPixels int64
	dpi       int
}

// OpenTesseract checks the DLL hash, loads it from its absolute path only
// (LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32: never PATH
// nor the current directory) and initializes the language in cfg.
func OpenTesseract(cfg WorkerConfig) (*Tesseract, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := checkModels(cfg); err != nil {
		return nil, err
	}
	dll, err := loadVerifiedDLL(cfg.Lib, cfg.LibSHA256)
	if err != nil {
		return nil, err
	}
	t := &Tesseract{dll: dll, maxPixels: cfg.maxPixels(), dpi: cfg.DPI}
	if err := t.resolve(); err != nil {
		_ = windows.FreeLibrary(dll)
		return nil, err
	}
	r, _, _ := syscall.SyscallN(t.fn.version)
	if v, ok := cString(r, 64); ok {
		t.version = v
	}
	t.api, _, _ = syscall.SyscallN(t.fn.create)
	if t.api == 0 {
		_ = windows.FreeLibrary(dll)
		return nil, fmt.Errorf("%w: TessBaseAPICreate falhou", ErrEngine)
	}
	// debug_file is a global parameter: set before Init3 it already silences
	// the messages printed while loading the models; set again after.
	if err := t.setVar("debug_file", "NUL"); err != nil {
		t.free()
		return nil, err
	}
	dp, lg := cBytes(cfg.Tessdata), cBytes(cfg.Lang)
	r, _, _ = syscall.SyscallN(t.fn.init3, t.api, uintptr(unsafe.Pointer(&dp[0])), uintptr(unsafe.Pointer(&lg[0])))
	runtime.KeepAlive(dp)
	runtime.KeepAlive(lg)
	if int32(r) != 0 {
		t.free()
		return nil, fmt.Errorf("%w: o Tesseract não inicializou o idioma (modelo ausente ou inválido)", ErrEngine)
	}
	if err := t.setVar("debug_file", "NUL"); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}

// loadVerifiedDLL opens path denying writers, hashes the opened file and
// loads it while that handle is still open, so the file cannot be swapped
// between the check and the load.
func loadVerifiedDLL(path, want string) (windows.Handle, error) {
	if err := checkAbsPath(path, "biblioteca do OCR"); err != nil {
		return 0, err
	}
	p16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("%w: caminho da biblioteca do OCR inválido", ErrConfig)
	}
	h, err := windows.CreateFile(p16, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, fmt.Errorf("%w: biblioteca do OCR não encontrada ou ilegível", ErrConfig)
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return 0, fmt.Errorf("%w: a biblioteca do OCR não é um arquivo regular", ErrConfig)
	}
	got, err := sha256Reader(f)
	if err != nil {
		return 0, fmt.Errorf("%w: não foi possível calcular o SHA-256 da biblioteca do OCR", ErrConfig)
	}
	if got != want {
		return 0, fmt.Errorf("%w: o SHA-256 da biblioteca do OCR não confere com o esperado", ErrConfig)
	}
	dll, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return 0, fmt.Errorf("%w: não foi possível carregar a biblioteca do OCR (%v)", ErrConfig, err)
	}
	return dll, nil
}

func (t *Tesseract) resolve() error {
	need := []struct {
		name string
		dst  *uintptr
	}{
		{"TessVersion", &t.fn.version},
		{"TessBaseAPICreate", &t.fn.create},
		{"TessBaseAPIInit3", &t.fn.init3},
		{"TessBaseAPISetPageSegMode", &t.fn.setPSM},
		{"TessBaseAPISetSourceResolution", &t.fn.setRes},
		{"TessBaseAPISetVariable", &t.fn.setVar},
		{"TessBaseAPISetImage", &t.fn.setImage},
		{"TessBaseAPIRecognize", &t.fn.recognize},
		{"TessBaseAPIGetUTF8Text", &t.fn.getText},
		{"TessBaseAPIMeanTextConf", &t.fn.meanConf},
		{"TessDeleteText", &t.fn.deleteText},
		{"TessBaseAPIClear", &t.fn.clear},
		{"TessBaseAPIEnd", &t.fn.end},
		{"TessBaseAPIDelete", &t.fn.del},
	}
	for _, n := range need {
		p, err := windows.GetProcAddress(t.dll, n.name)
		if err != nil || p == 0 {
			return fmt.Errorf("%w: a biblioteca do OCR não exporta %s", ErrConfig, n.name)
		}
		*n.dst = p
	}
	return nil
}

// Version is the TessVersion string.
func (t *Tesseract) Version() string { return t.version }

func (t *Tesseract) setVar(name, value string) error {
	n, v := cBytes(name), cBytes(value)
	r, _, _ := syscall.SyscallN(t.fn.setVar, t.api, uintptr(unsafe.Pointer(&n[0])), uintptr(unsafe.Pointer(&v[0])))
	runtime.KeepAlive(n)
	runtime.KeepAlive(v)
	if int32(r) == 0 {
		return fmt.Errorf("%w: TessBaseAPISetVariable(%s) recusou o valor", ErrEngine, name)
	}
	return nil
}

// Recognize runs the §11.3 sequence on img. Pages turned 90 degrees are
// handled by Tesseract's own layout analysis (vertical text lines); a page
// upside down is not, so when the first pass scores under rotateBelowConf a
// second pass runs on the page turned 180 degrees, and the better one wins
// (Page.Rotated = 180). The 5.3.3 DLL has no orientation detection
// (TessBaseAPIDetectOrientationScript is not exported: an LSTM-only build),
// so osd.traineddata is not used. ctx is not consulted inside the native
// call (the pool enforces the page timeout by killing the worker process).
func (t *Tesseract) Recognize(ctx context.Context, img *image.Gray, dpi int) (Page, error) {
	if err := checkImage(img, t.maxPixels); err != nil {
		return Page{}, err
	}
	if dpi == 0 {
		dpi = t.dpi
	}
	if dpi < MinDPI || dpi > MaxDPI {
		return Page{}, ErrBadImage
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.api == 0 {
		return Page{}, ErrClosed
	}
	text, conf, err := t.pass(img, dpi)
	if err != nil {
		return Page{}, err
	}
	page := Page{Text: text, Conf: conf}
	if conf < rotateBelowConf && strings.TrimSpace(text) != "" {
		if text2, conf2, err := t.pass(rotateGrayCW(img, 180), dpi); err == nil && conf2 > conf {
			page = Page{Text: text2, Conf: conf2, Rotated: 180}
		}
	}
	return page, nil
}

// setImage hands the pixels to Tesseract, which copies them into its own
// image; the buffer only has to stay alive until the call returns.
func (t *Tesseract) setImage(img *image.Gray, dpi int) {
	off := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
	syscall.SyscallN(t.fn.setPSM, t.api, psmAuto)
	syscall.SyscallN(t.fn.setRes, t.api, uintptr(dpi))
	syscall.SyscallN(t.fn.setImage, t.api, uintptr(unsafe.Pointer(&img.Pix[off])),
		uintptr(img.Rect.Dx()), uintptr(img.Rect.Dy()), 1, uintptr(img.Stride))
	runtime.KeepAlive(img)
}

func (t *Tesseract) pass(img *image.Gray, dpi int) (string, int, error) {
	defer syscall.SyscallN(t.fn.clear, t.api)
	t.setImage(img, dpi)
	if r, _, _ := syscall.SyscallN(t.fn.recognize, t.api, 0); int32(r) != 0 {
		return "", 0, fmt.Errorf("%w: TessBaseAPIRecognize devolveu %d", ErrEngine, int32(r))
	}
	p, _, _ := syscall.SyscallN(t.fn.getText, t.api)
	if p == 0 {
		return "", 0, fmt.Errorf("%w: TessBaseAPIGetUTF8Text sem resultado", ErrEngine)
	}
	text, ok := cString(p, MaxTextBytes)
	syscall.SyscallN(t.fn.deleteText, p)
	if !ok {
		return "", 0, ErrTextTooLarge
	}
	r, _, _ := syscall.SyscallN(t.fn.meanConf, t.api)
	return text, clampConf(int(int32(r))), nil
}

func (t *Tesseract) free() {
	if t.api != 0 {
		syscall.SyscallN(t.fn.del, t.api)
		t.api = 0
	}
	if t.dll != 0 {
		_ = windows.FreeLibrary(t.dll)
		t.dll = 0
	}
}

// Close ends and deletes the API instance and unloads the DLL.
func (t *Tesseract) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.api != 0 {
		syscall.SyscallN(t.fn.end, t.api)
	}
	t.free()
	return nil
}

// cBytes is s as a NUL-terminated C string. s never contains NUL here (the
// configuration validation refuses it), but a NUL would only truncate.
func cBytes(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b
}

// cString copies the NUL-terminated C string at p, reading at most max+1
// bytes; ok is false when no NUL is found within max bytes.
func cString(p uintptr, max int) (string, bool) {
	if p == 0 {
		return "", true
	}
	// Converted through memory to keep go vet's unsafeptr check quiet: p is
	// C memory, not managed by the Go heap.
	base := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	n := 0
	for ; n <= max; n++ {
		if *(*byte)(unsafe.Add(base, n)) == 0 {
			break
		}
	}
	if n > max {
		return "", false
	}
	return string(unsafe.Slice((*byte)(base), n)), true
}
