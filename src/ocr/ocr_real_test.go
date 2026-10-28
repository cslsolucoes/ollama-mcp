//go:build ocr && windows

package ocr

// Tests against the real libtesseract64.dll (tag ocr, one run at a time):
//
//	$env:MCPOLLAMA_TEST_OCR_LIB='D:\MCP\libtesseract64.dll'
//	$env:MCPOLLAMA_TEST_OCR_TESSDATA='D:\MCP\tessdata\best'
//	go test -count=1 -tags ocr -run TestReal -v ./ocr
//
// MCPOLLAMA_TEST_OCR_LIB_SHA256 overrides the expected hash (default: the
// 5.3.3 DLL of D-8); MCPOLLAMA_TEST_OCR_TESSDATA_FAST points at the fast
// models (default: the "fast" sibling of the best directory). The page is
// generated here (Go fonts, BSD license, embedded in golang.org/x/image):
// no document of a client is involved.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/sys/windows"
)

const defaultTestLibSHA256 = "964b0972348f6283b1721f9131da59721ce46fb4f7a4e294399b1169c7e71669"

func realConfig(t *testing.T, fast bool) Config {
	t.Helper()
	lib := os.Getenv("MCPOLLAMA_TEST_OCR_LIB")
	td := os.Getenv("MCPOLLAMA_TEST_OCR_TESSDATA")
	if lib == "" || td == "" {
		t.Skip("MCPOLLAMA_TEST_OCR_LIB / MCPOLLAMA_TEST_OCR_TESSDATA not set")
	}
	if fast {
		td = os.Getenv("MCPOLLAMA_TEST_OCR_TESSDATA_FAST")
		if td == "" {
			td = filepath.Join(filepath.Dir(os.Getenv("MCPOLLAMA_TEST_OCR_TESSDATA")), "fast")
		}
	}
	sha := os.Getenv("MCPOLLAMA_TEST_OCR_LIB_SHA256")
	if sha == "" {
		sha = defaultTestLibSHA256
	}
	return Config{Lib: lib, LibSHA256: sha, Tessdata: td, Workers: 1}
}

// Sentences of a made-up service contract (accents, R$ amounts, dates).
var contractLines = []string{
	"CONTRATO DE PRESTAÇÃO DE SERVIÇOS",
	"Cláusula primeira: o contratante pagará a quantia de R$ 1.234,56",
	"em três parcelas mensais, com vencimento no dia 15 de cada mês.",
	"Cláusula segunda: a contratada executará os serviços descritos",
	"no anexo, observando as normas técnicas e a legislação vigente.",
	"Cláusula terceira: o atraso no pagamento implicará multa de 2%",
	"e juros de mora de 1% ao mês, calculados sobre o valor devido.",
	"Cláusula quarta: este contrato vigorará por doze meses, podendo",
	"ser renovado por acordo escrito entre as partes contratantes.",
	"Parágrafo único: a rescisão antecipada exige aviso de trinta dias.",
	"São Paulo, 28 de outubro de 2026.",
}

// mustWords must all appear in the recognized text.
var mustWords = []string{"PRESTAÇÃO", "SERVIÇOS", "Cláusula", "1.234,56", "três", "mês", "São", "Paulo", "outubro", "Parágrafo", "único", "rescisão"}

// renderPage draws the contract text, repeated to fill an A4 page at 300
// dpi (2480x3508), black on white, 12 pt Go Regular.
func renderPage(t *testing.T, repeat int) *image.Gray {
	t.Helper()
	ft, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: 12, DPI: 300, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	img := image.NewGray(image.Rect(0, 0, 2480, 3508))
	for i := range img.Pix {
		img.Pix[i] = 0xFF
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.Gray{Y: 0}), Face: face}
	lineH := face.Metrics().Height.Ceil() * 3 / 2
	y := 300
	for r := 0; r < repeat; r++ {
		for _, l := range contractLines {
			if y > 3508-300 {
				return img
			}
			d.Dot = fixed.P(250, y)
			d.DrawString(l)
			y += lineH
		}
		y += lineH
	}
	return img
}

func checkText(t *testing.T, label, text string) int {
	t.Helper()
	missing := 0
	for _, w := range mustWords {
		if !strings.Contains(text, w) {
			missing++
			t.Errorf("%s: %q not recognized", label, w)
		}
	}
	return len(mustWords) - missing
}

// memCounters is PROCESS_MEMORY_COUNTERS.
type memCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

func processMemory(pid int) (memCounters, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return memCounters{}, false
	}
	defer windows.CloseHandle(h)
	var m memCounters
	m.cb = uint32(unsafe.Sizeof(m))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.cb))
	return m, r != 0
}

type measured struct {
	startup     time.Duration
	pages       []time.Duration
	conf        []int
	peakWS      uintptr
	sampledWS   uintptr
	peakCommit  uintptr
	idleWS      uintptr
	version     string
	wordsFound  int
	rotatedSeen int
}

// measure runs n pages through a one-worker pool and samples the worker's
// working set every 25 ms during the first page.
func measure(t *testing.T, cfg Config, img *image.Gray, n int) measured {
	t.Helper()
	t.Setenv("OCRTEST_MODE", "real")
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	log := &spawnLog{}
	p.onSpawn = log.add
	var m measured
	start := time.Now()
	if m.version, err = p.Probe(context.Background()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	m.startup = time.Since(start)
	pid := log.list()[0].cmd.Process.Pid
	if mc, ok := processMemory(pid); ok {
		m.idleWS = mc.workingSetSize
	}
	for i := 0; i < n; i++ {
		stop := make(chan struct{})
		var wg sync.WaitGroup
		if i == 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tick := time.NewTicker(25 * time.Millisecond)
				defer tick.Stop()
				for {
					if mc, ok := processMemory(pid); ok && mc.workingSetSize > m.sampledWS {
						m.sampledWS = mc.workingSetSize
					}
					select {
					case <-stop:
						return
					case <-tick.C:
					}
				}
			}()
		}
		t0 := time.Now()
		page, err := p.Recognize(context.Background(), img, 300)
		d := time.Since(t0)
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		m.pages = append(m.pages, d)
		m.conf = append(m.conf, page.Conf)
		if i == 0 {
			m.wordsFound = checkText(t, "page", page.Text)
			m.rotatedSeen = page.Rotated
			if page.Conf <= 80 {
				t.Errorf("confidence %d, want > 80", page.Conf)
			}
		}
	}
	if len(log.list()) != 1 {
		t.Errorf("%d workers started for %d pages, want 1", len(log.list()), n)
	}
	if mc, ok := processMemory(pid); ok {
		m.peakWS, m.peakCommit = mc.peakWorkingSetSize, mc.peakPagefileUsage
	}
	return m
}

func TestRealOCRBestAndFast(t *testing.T) {
	img := renderPage(t, 4)
	for _, model := range []string{"best", "fast"} {
		t.Run(model, func(t *testing.T) {
			cfg := realConfig(t, model == "fast")
			m := measure(t, cfg, img, 3)
			var total time.Duration
			for _, d := range m.pages {
				total += d
			}
			t.Logf("MEASURE model=%s tesseract=%s startup(spawn+DLL+Init3)=%v pages_ms=%v mean_ms=%d conf=%v words=%d/%d rotated=%d",
				model, m.version, m.startup.Round(time.Millisecond), msList(m.pages), (total / time.Duration(len(m.pages))).Milliseconds(),
				m.conf, m.wordsFound, len(mustWords), m.rotatedSeen)
			t.Logf("MEASURE model=%s worker_ws_idle_MiB=%.1f worker_ws_sampled_max_MiB=%.1f worker_ws_peak_MiB=%.1f worker_commit_peak_MiB=%.1f",
				model, mib(m.idleWS), mib(m.sampledWS), mib(m.peakWS), mib(m.peakCommit))
		})
	}
}

func TestRealOCRRotatedPage(t *testing.T) {
	cfg := realConfig(t, false)
	t.Setenv("OCRTEST_MODE", "real")
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	upright := renderPage(t, 2)
	for _, deg := range []int{90, 180, 270} {
		img := rotateGrayCW(upright, deg)
		t0 := time.Now()
		page, err := p.Recognize(context.Background(), img, 300)
		if err != nil {
			t.Fatalf("%d: %v", deg, err)
		}
		found := 0
		for _, w := range mustWords {
			if strings.Contains(page.Text, w) {
				found++
			}
		}
		t.Logf("MEASURE input_rotated_cw=%d reported_rotated=%d conf=%d words=%d/%d ms=%d",
			deg, page.Rotated, page.Conf, found, len(mustWords), time.Since(t0).Milliseconds())
		// 90: Tesseract reads vertical lines itself; 180: the second pass;
		// 270: the 180 pass leaves a 90 page that Tesseract reads.
		if want := map[int]int{90: 0, 180: 180, 270: 180}[deg]; page.Rotated != want {
			t.Errorf("input turned %d degrees clockwise, Rotated = %d, want %d", deg, page.Rotated, want)
		}
		if page.Conf <= 80 || found < len(mustWords)-1 {
			t.Errorf("rotated %d: conf %d, words %d/%d", deg, page.Conf, found, len(mustWords))
		}
	}
}

// TestRealWorkerStdoutClean runs the real worker the way 7.9b will
// (WorkerStdio + RunWorker), with extra noise printed to every standard
// output after the redirection, and feeds it pages that make Tesseract talk
// (dpi 10 and an empty page). Its stdout must hold the hello and one
// response per request, and not one byte more.
func TestRealWorkerStdoutClean(t *testing.T) {
	cfg := realConfig(t, false).WorkerConfig()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, WorkerArgs(cfg)...)
	cmd.Env = append(workerEnv(os.Environ()), "OCRTEST_MODE=real-noisy")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	white := image.NewGray(image.Rect(0, 0, 800, 600))
	for i := range white.Pix {
		white.Pix[i] = 0xFF
	}
	small := renderPage(t, 1).SubImage(image.Rect(0, 0, 2480, 1200)).(*image.Gray)
	reqs := []struct {
		img *image.Gray
		dpi int
	}{{small, 300}, {white, 300}, {small, 10}}
	for _, r := range reqs {
		if err := writeRequest(stdin, r.img, r.dpi); err != nil {
			t.Fatal(err)
		}
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("worker: %v (stderr %d bytes)", err, stderr.Len())
	}
	raw := stdout.Bytes()
	if bytes.Contains(raw, []byte("NOISE")) {
		t.Fatal("noise reached the frame channel")
	}
	rd := bytes.NewReader(raw)
	hello, err := readResponse(rd)
	if err != nil || hello.kind != kindHello || hello.status != statusOK {
		t.Fatalf("hello: %+v %v", hello, err)
	}
	for i := range reqs {
		resp, err := readResponse(rd)
		if err != nil || resp.kind != kindResult || resp.status != statusOK {
			t.Fatalf("response %d: kind=%d status=%d err=%v", i, resp.kind, resp.status, err)
		}
		t.Logf("response %d: conf=%d text_bytes=%d", i, resp.conf, len(resp.text))
	}
	if rest, _ := io.ReadAll(rd); len(rest) != 0 {
		t.Fatalf("%d stray bytes after the last frame: %q", len(rest), rest[:min(len(rest), 80)])
	}
	t.Logf("stdout: %d bytes, exactly %d frames; worker stderr: %d bytes", len(raw), 1+len(reqs), stderr.Len())
}

func msList(ds []time.Duration) []int64 {
	out := make([]int64, len(ds))
	for i, d := range ds {
		out[i] = d.Milliseconds()
	}
	return out
}

func mib(b uintptr) float64 { return float64(b) / (1 << 20) }
