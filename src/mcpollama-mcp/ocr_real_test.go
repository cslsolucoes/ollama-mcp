//go:build ocr && windows

package main

// OCR with the real libtesseract64.dll (tag ocr, one run at a time):
//
//	$env:MCPOLLAMA_TEST_OCR_LIB='D:\MCP\libtesseract64.dll'
//	$env:MCPOLLAMA_TEST_OCR_TESSDATA='D:\MCP\tessdata\best'
//	go test -count=1 -tags ocr -run TestRealOCR -v ./mcpollama-mcp
//
// MCPOLLAMA_TEST_OCR_LIB_SHA256 overrides the expected hash (default: the
// 5.3.3 DLL of D-8). The scanned page is generated here (Go fonts, BSD
// license): no document of a client is involved.

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const defaultRealLibSHA256 = "964b0972348f6283b1721f9131da59721ce46fb4f7a4e294399b1169c7e71669"

var realLines = []string{
	"CONTRATO DE PRESTAÇÃO DE SERVIÇOS",
	"Cláusula primeira: o contratante pagará a quantia de R$ 1.234,56",
	"em três parcelas mensais, com vencimento no dia 15 de cada mês.",
	"Cláusula segunda: a contratada executará os serviços descritos",
	"no anexo, observando as normas técnicas e a legislação vigente.",
	"Parágrafo único: a rescisão antecipada exige aviso de trinta dias.",
	"São Paulo, 28 de outubro de 2026.",
}

var realWords = []string{"PRESTAÇÃO", "SERVIÇOS", "Cláusula", "1.234,56", "três", "mês", "São", "Paulo", "outubro", "Parágrafo", "único", "rescisão", "técnicas", "legislação"}

func realOCRFlags(t *testing.T) OCRFlags {
	t.Helper()
	lib, td := os.Getenv("MCPOLLAMA_TEST_OCR_LIB"), os.Getenv("MCPOLLAMA_TEST_OCR_TESSDATA")
	if lib == "" || td == "" {
		t.Skip("MCPOLLAMA_TEST_OCR_LIB / MCPOLLAMA_TEST_OCR_TESSDATA not set")
	}
	sha := os.Getenv("MCPOLLAMA_TEST_OCR_LIB_SHA256")
	if sha == "" {
		sha = defaultRealLibSHA256
	}
	o := defaultOCRFlags()
	o.Lib, o.LibSHA256, o.Tessdata = lib, sha, td
	return o
}

// a4Page draws realLines on an A4 page at 300 dpi (2480x3508), 12 pt.
func a4Page(t *testing.T) *image.Gray {
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
	for i, l := range realLines {
		d.Dot = fixed.P(250, 300+i*lineH)
		d.DrawString(l)
	}
	return img
}

// scanPDF is a one-page A4 PDF whose only content is img (DeviceGray,
// FlateDecode): a scan without any text layer.
func scanPDF(t *testing.T, img *image.Gray) []byte {
	t.Helper()
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := 0; y < h; y++ {
		zw.Write(img.Pix[y*img.Stride : y*img.Stride+w])
	}
	zw.Close()
	content := []byte("q 595 0 0 842 0 0 cm /Im1 Do Q\n")
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /XObject << /Im1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream",
			w, h, z.Len(), z.String()),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return out.Bytes()
}

func wordsFound(text string) (int, []string) {
	var missing []string
	for _, w := range realWords {
		if !strings.Contains(text, w) {
			missing = append(missing, w)
		}
	}
	return len(realWords) - len(missing), missing
}

func TestRealOCRScannedPDFAndImages(t *testing.T) {
	o := realOCRFlags(t)
	t.Cleanup(func() { doctext.ClosePDF() }) // realMain configures the PDF engine of the next test
	lg, logs := testLogger()
	start := time.Now()
	rt := setupOCR(Config{OCR: o}, lg)
	if rt == nil {
		t.Fatalf("setupOCR: %s", logs.String())
	}
	defer rt.Close()
	setup := time.Since(start)
	<-rt.done
	if rt.options() == nil {
		t.Fatalf("probe: %s", logs.String())
	}
	t.Logf("setupOCR %v (síncrono); probe concluído em %v; %s", setup.Round(time.Millisecond), time.Since(start).Round(time.Millisecond), strings.TrimSpace(logs.String()))

	ex := newDocExtractor(Config{MaxPromptChars: 128000, MaxChunks: 40, Timeout: 2 * time.Minute, ocrRT: rt}, nil)
	page := a4Page(t)
	var pngB, jpgB bytes.Buffer
	if err := png.Encode(&pngB, page); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpgB, page, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, file string
		data       []byte
	}{
		{"pdf digitalizado", "scan.pdf", scanPDF(t, page)},
		{"png", "scan.png", pngB.Bytes()},
		{"jpg", "scan.jpg", jpgB.Bytes()},
	} {
		t0 := time.Now()
		x, err := ex.Extract(context.Background(), c.file, c.data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		n, missing := wordsFound(x.Text)
		t.Logf("%s: formato=%s paginas_ocr=%d conf=%d falhas=%d palavras %d/%d %v em %v",
			c.name, x.Format, x.OCR.Pages, x.OCR.ConfMean, x.OCR.Failed, n, len(realWords), missing, time.Since(t0).Round(time.Millisecond))
		if x.OCR.Pages != 1 || x.OCR.ConfMean <= 85 || n < len(realWords)-1 {
			t.Fatalf("%s: OCR fraco: %+v, faltam %v\n%s", c.name, x.OCR, missing, x.Text)
		}
	}
}

// End to end through JSON-RPC: the binary's own wiring with --ocr-lib, the
// Ollama simulated; the footer has paginas_ocr and the [aviso] follows it.
func TestRealOCREndToEnd(t *testing.T) {
	o := realOCRFlags(t)
	var mu sync.Mutex
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		prompts = append(prompts, req.Prompt)
		mu.Unlock()
		answerOK(w, r)
	}))
	defer srv.Close()
	root := rootWithFiles(t)
	if err := os.WriteFile(filepath.Join(root, "digitalizado.pdf"), scanPDF(t, a4Page(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--ollama-url", srv.URL, "--allow-root", root, "--ocr-lib", o.Lib, "--ocr-lib-sha256", o.LibSHA256, "--ocr-tessdata", o.Tessdata}
	t0 := time.Now()
	lines, errw, code := runMain(t, args, nil, toolCall(1, "summarize_file", `{"path":"digitalizado.pdf"}`))
	if code != 0 || len(lines) != 1 {
		t.Fatalf("exit %d\n%s", code, errw)
	}
	res, _ := resultOf(t, lines[0])
	out := res.Content[0].Text
	if res.IsError || !strings.Contains(out, " paginas_ocr=1 ") || !strings.Contains(out, "\n[aviso] texto obtido por OCR: números, datas, nomes e CPF/CNPJ podem conter erros de reconhecimento; confira os valores críticos no original") {
		t.Fatalf("resposta:\n%s\nstderr:\n%s", out, errw)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) != 1 || !strings.Contains(prompts[0], "[página 1 — OCR conf=") || !strings.Contains(prompts[0], "PRESTAÇÃO") {
		t.Fatalf("o prompt não traz o texto do OCR: %d prompts", len(prompts))
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "[arquivo]") {
			t.Logf("%s", l)
		}
	}
	t.Logf("stderr: %s; total %v", strings.TrimSpace(errw), time.Since(t0).Round(time.Millisecond))
	if strings.Contains(errw, "digitalizado") || strings.Contains(errw, root) || strings.Contains(errw, "PRESTAÇÃO") {
		t.Fatalf("stderr com nome, caminho ou texto: %s", errw)
	}
}
