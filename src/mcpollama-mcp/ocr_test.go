package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
	"github.com/cslsoftwares/mcpollama/ocr"
)

// The test binary doubles as the OCR worker executable: the pool starts
// os.Executable() with ocr.WorkerFlag, exactly as main() dispatches it.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == ocr.WorkerFlag {
		os.Exit(ocrWorkerMain(os.Args[1:], os.Stderr))
	}
	os.Exit(m.Run())
}

func TestOCRFlagsDefaultsEnvAndFlags(t *testing.T) {
	c := parse(t, nil, nil)
	if c.OCR != defaultOCRFlags() || c.OCR.Lib != "" {
		t.Fatalf("defaults: %+v", c.OCR)
	}
	d := defaultOCRFlags()
	if d.Workers != ocr.DefaultWorkers() || d.PageTimeout != 60*time.Second || d.MaxPages != 30 || d.MaxMem != 1<<30 || d.MaxMegapixels != 40 || d.DPI != 300 {
		t.Fatalf("defaults fora do brief: %+v", d)
	}
	e := map[string]string{
		"MCPOLLAMA_OCR_LIB": `C:\x\libtesseract64.dll`, "MCPOLLAMA_OCR_LIB_SHA256": "ab", "MCPOLLAMA_OCR_TESSDATA": `C:\x\td`,
		"MCPOLLAMA_OCR_WORKERS": "2", "MCPOLLAMA_OCR_PAGE_TIMEOUT": "30s", "MCPOLLAMA_OCR_MAX_PAGES": "5",
		"MCPOLLAMA_OCR_MAX_MEM": "536870912", "MCPOLLAMA_OCR_MAX_MEGAPIXELS": "20", "MCPOLLAMA_OCR_DPI": "200",
	}
	c = parse(t, nil, e)
	want := OCRFlags{Lib: `C:\x\libtesseract64.dll`, LibSHA256: "ab", Tessdata: `C:\x\td`, Workers: 2, PageTimeout: 30 * time.Second,
		MaxPages: 5, MaxMem: 512 << 20, MaxMegapixels: 20, DPI: 200}
	if c.OCR != want {
		t.Fatalf("env: %+v", c.OCR)
	}
	c = parse(t, []string{"--ocr-lib", `D:\y.dll`, "--ocr-lib-sha256", "cd", "--ocr-tessdata", `D:\t`, "--ocr-workers", "3",
		"--ocr-page-timeout", "5s", "--ocr-max-pages", "7", "--ocr-max-mem", "67108864", "--ocr-max-megapixels", "10", "--ocr-dpi", "150"}, e)
	want = OCRFlags{Lib: `D:\y.dll`, LibSHA256: "cd", Tessdata: `D:\t`, Workers: 3, PageTimeout: 5 * time.Second,
		MaxPages: 7, MaxMem: 64 << 20, MaxMegapixels: 10, DPI: 150}
	if c.OCR != want {
		t.Fatalf("flags: %+v", c.OCR)
	}
	for k, v := range map[string]string{"MCPOLLAMA_OCR_PAGE_TIMEOUT": "rapido", "MCPOLLAMA_OCR_MAX_MEM": "1G", "MCPOLLAMA_OCR_WORKERS": "x"} {
		if _, err := parseConfig(nil, env(map[string]string{k: v}), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), k) {
			t.Fatalf("%s=%s: %v", k, v, err)
		}
	}
}

func TestOCRConfigResolve(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "libtesseract64.dll")
	sha := strings.Repeat("a", 64)
	base := Config{OCR: defaultOCRFlags()}
	if _, ok, err := base.ocrConfig(); ok || err != nil {
		t.Fatalf("sem --ocr-lib o OCR fica desligado: ok=%v err=%v", ok, err)
	}
	c := base
	c.OCR.Lib, c.OCR.LibSHA256 = lib, sha
	cfg, ok, err := c.ocrConfig()
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	if cfg.Tessdata != filepath.Join(filepath.Dir(lib), "tessdata", "best") {
		t.Fatalf("tessdata padrão (D-11): %s", cfg.Tessdata)
	}
	bad := map[string]func(*OCRFlags){
		"relative lib":      func(o *OCRFlags) { o.Lib = `rel\libtesseract64.dll` },
		"no hash":           func(o *OCRFlags) { o.LibSHA256 = "" },
		"short hash":        func(o *OCRFlags) { o.LibSHA256 = "abc" },
		"relative tessdata": func(o *OCRFlags) { o.Tessdata = "tessdata" },
		"workers 0":         func(o *OCRFlags) { o.Workers = 0 },
		"workers 17":        func(o *OCRFlags) { o.Workers = 17 },
		"timeout 0":         func(o *OCRFlags) { o.PageTimeout = 0 },
		"pages 0":           func(o *OCRFlags) { o.MaxPages = 0 },
		"mem small":         func(o *OCRFlags) { o.MaxMem = 1 << 20 },
		"megapixels 0":      func(o *OCRFlags) { o.MaxMegapixels = 0 },
		"megapixels 201":    func(o *OCRFlags) { o.MaxMegapixels = 201 },
		"dpi 0":             func(o *OCRFlags) { o.DPI = 0 },
		"dpi 1201":          func(o *OCRFlags) { o.DPI = 1201 },
	}
	for name, mut := range bad {
		c := base
		c.OCR.Lib, c.OCR.LibSHA256 = lib, sha
		mut(&c.OCR)
		if _, ok, err := c.ocrConfig(); !ok || err == nil {
			t.Fatalf("%s: ok=%v err=%v", name, ok, err)
		}
	}
}

// fakeDLL writes a file to stand for the DLL and returns its path and hash,
// plus a tessdata directory with a placeholder por model.
func fakeDLL(t *testing.T) (lib, sha, tessdata string) {
	t.Helper()
	dir := t.TempDir()
	lib = filepath.Join(dir, "libtesseract64.dll")
	body := []byte("not a real DLL")
	if err := os.WriteFile(lib, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	tessdata = filepath.Join(dir, "tessdata", "best")
	if err := os.MkdirAll(tessdata, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tessdata, "por.traineddata"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return lib, hex.EncodeToString(sum[:]), tessdata
}

func testLogger() (*logger, *syncBuf) {
	var b syncBuf
	return &logger{w: &b, redact: func(s string) string { return s }}, &b
}

// Every OCR problem at start is a WARNING and no OCR, never a failed start.
func TestSetupOCRWarnings(t *testing.T) {
	lib, sha, _ := fakeDLL(t)
	cases := map[string]OCRFlags{
		"relative":     {Lib: `rel\libtesseract64.dll`, LibSHA256: sha},
		"no hash":      {Lib: lib},
		"hash differs": {Lib: lib, LibSHA256: strings.Repeat("0", 64)},
		"missing dll":  {Lib: filepath.Join(filepath.Dir(lib), "nada.dll"), LibSHA256: sha},
		"no models":    {Lib: lib, LibSHA256: sha, Tessdata: t.TempDir()},
	}
	for name, o := range cases {
		d := defaultOCRFlags()
		d.Lib, d.LibSHA256, d.Tessdata = o.Lib, o.LibSHA256, o.Tessdata
		lg, out := testLogger()
		if rt := setupOCR(Config{OCR: d}, lg); rt != nil {
			rt.Close()
			t.Fatalf("%s: OCR ligado", name)
		}
		if !strings.Contains(out.String(), "WARNING: OCR disabled: ") {
			t.Fatalf("%s: sem WARNING: %q", name, out.String())
		}
	}
	lg, out := testLogger()
	if rt := setupOCR(Config{OCR: defaultOCRFlags()}, lg); rt != nil || out.String() != "" {
		t.Fatalf("sem --ocr-lib: rt=%v log=%q", rt, out.String())
	}
}

// The server starts and answers normally with a wrong hash; the WARNING is
// on stderr and stdout carries JSON-RPC only.
func TestWrongHashServerStarts(t *testing.T) {
	lib, _, _ := fakeDLL(t)
	root := rootWithFiles(t)
	lines, errw, code := runMain(t, []string{"--allow-root", root, "--ocr-lib", lib, "--ocr-lib-sha256", strings.Repeat("0", 64)}, nil,
		rpc(1, "tools/list", ""))
	if code != 0 || len(lines) != 1 || !strings.Contains(lines[0], "summarize_file") {
		t.Fatalf("exit %d, %d linhas\n%s", code, len(lines), errw)
	}
	if !strings.Contains(errw, "WARNING: OCR disabled: ") || !strings.Contains(errw, "SHA-256") {
		t.Fatalf("stderr: %s", errw)
	}
	if strings.Contains(errw, root) {
		t.Fatalf("stderr com caminho de documento: %s", errw)
	}
}

// A DLL that does not load is found by the background probe: OCR turns off
// with a WARNING, the server goes on. The probe runs the real worker mode of
// this test binary (TestMain), which refuses the fake DLL.
func TestProbeFailureTurnsOCROff(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("OCR is Windows only (D-10)")
	}
	lib, sha, td := fakeDLL(t)
	d := defaultOCRFlags()
	d.Lib, d.LibSHA256, d.Tessdata, d.Workers = lib, sha, td, 1
	lg, out := testLogger()
	start := time.Now()
	rt := setupOCR(Config{OCR: d}, lg)
	if rt == nil {
		t.Fatalf("a configuração é válida até a carga da DLL: %s", out.String())
	}
	defer rt.Close()
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("setupOCR bloqueou a partida por %v", el)
	}
	select {
	case <-rt.done:
	case <-time.After(30 * time.Second):
		t.Fatal("probe não terminou")
	}
	if rt.options() != nil || !strings.Contains(out.String(), "WARNING: OCR disabled: ") {
		t.Fatalf("probe com DLL inválida deveria desligar o OCR: %q", out.String())
	}
	t.Logf("partida %v; log: %s", time.Since(start).Round(time.Millisecond), strings.TrimSpace(out.String()))
}

func TestOCRWorkerModeHiddenAndStrict(t *testing.T) {
	hidden := regexp.MustCompile(`--ocr-worker([^s]|$)`)
	if hidden.MatchString(usageText) {
		t.Fatal("--ocr-worker aparece no --help")
	}
	_, errw, code := runMain(t, []string{"--help"}, nil)
	if code != 0 || hidden.MatchString(errw) || !strings.Contains(errw, "--ocr-lib") {
		t.Fatalf("--help: %d %q", code, errw)
	}
	// Through realMain (the server path) the flag does not exist.
	_, errw, code = runMain(t, []string{ocr.WorkerFlag}, nil)
	if code != 2 {
		t.Fatalf("realMain --ocr-worker: exit %d %s", code, errw)
	}
	var e bytes.Buffer
	for _, args := range [][]string{
		{ocr.WorkerFlag},
		{ocr.WorkerFlag, "--api-key", "segredo"},
		{ocr.WorkerFlag, "--ocr-lib", "rel.dll", "--ocr-lib-sha256", strings.Repeat("a", 64), "--ocr-tessdata", `C:\t`},
	} {
		e.Reset()
		if code := ocrWorkerMain(args, &e); code != 2 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if strings.Contains(e.String(), "segredo") {
			t.Fatal("o worker ecoou um argumento")
		}
	}
}

// fakePool stands for the ocr.Pool behind ocrRuntime.
type fakePool struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakePool) Recognize(_ context.Context, img *image.Gray, dpi int) (ocr.Page, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return ocr.Page{}, f.err
	}
	return ocr.Page{Text: "Recibo — São Paulo", Conf: 92, Rotated: 180}, nil
}

func (f *fakePool) Close() error { return nil }

func grayPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 50, 30))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	img.Pix[100] = 0
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func readyRuntime(p ocrEngine) *ocrRuntime {
	rt := &ocrRuntime{pool: p, opts: doctext.OCROptions{DPI: 300, MaxMegapixels: 40, MaxPages: 30, Workers: 2}, done: make(chan struct{})}
	close(rt.done)
	return rt
}

func TestDocExtractorWithOCR(t *testing.T) {
	c := Config{MaxPromptChars: 128000, MaxChunks: 40, Timeout: time.Minute}
	e := newDocExtractor(c, nil)
	for _, n := range []string{"a.png", "B.JPG", "c.jpeg"} {
		if !e.IsDocument(n) {
			t.Fatalf("%s deveria ser documento", n)
		}
	}
	// No OCR: an image is refused with the new message.
	_, err := e.Extract(context.Background(), "a.png", grayPNG(t))
	if err == nil || err.Error() != "imagem requer OCR, que não está habilitado no servidor" {
		t.Fatalf("sem OCR: %v", err)
	}
	// With OCR: the stats reach files.Extracted.
	fp := &fakePool{}
	c.ocrRT = readyRuntime(fp)
	e = newDocExtractor(c, nil)
	x, err := e.Extract(context.Background(), "a.png", grayPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if x.Format != "png" || x.OCR != (files.OCRInfo{Used: true, Pages: 1, ConfMean: 92}) ||
		x.Text != "[imagem — OCR conf=92 girada=180]\nRecibo — São Paulo" {
		t.Fatalf("%+v", x)
	}
	// The pool's "cannot go on" becomes the clear refusal.
	for _, perr := range []error{ocr.ErrUnavailable, ocr.ErrClosed, ocr.ErrUnsupportedPlatform} {
		fp.err = perr
		_, err = e.Extract(context.Background(), "a.png", grayPNG(t))
		if r, _ := doctext.ReasonOf(err); r != doctext.ReasonOCRUnavailable {
			t.Fatalf("%v: %v", perr, err)
		}
	}
	// A page error is a failed page: for an image, nothing was read.
	fp.err = &ocr.PageError{Err: ocr.ErrEngine}
	_, err = e.Extract(context.Background(), "a.png", grayPNG(t))
	if r, _ := doctext.ReasonOf(err); r != doctext.ReasonOCRNoText {
		t.Fatalf("falha de página: %v", err)
	}
	// After a failed probe the runtime is off: back to the refusal.
	c.ocrRT.off.Store(true)
	_, err = e.Extract(context.Background(), "a.png", grayPNG(t))
	if r, _ := doctext.ReasonOf(err); r != doctext.ReasonOCRRequired {
		t.Fatalf("OCR desligado após a sondagem: %v", err)
	}
}
