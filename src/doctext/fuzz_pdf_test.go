package doctext

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FuzzPDF feeds arbitrary bytes to the PDF extractor (real PDFium engine).
// Whatever comes in, the outcome is a text or a typed refusal: never a panic
// that escapes, never an untyped error. Seeds are the generated fixtures.
func FuzzPDF(f *testing.F) {
	// Every fuzz worker is a new process: a compilation cache in the temp dir
	// (outside the repository) spares each of them the multi-second start.
	_ = ConfigurePDF(PDFOptions{CacheDir: filepath.Join(os.TempDir(), "mcpollama-doctext-fuzz-wazero")})
	f.Add(pdfASCII(f))
	f.Add(pdfAccents(f))
	f.Add(pdfThreePages(f))
	f.Add(pdfBlankMiddle(f))
	f.Add(pdfAllBlank(f))
	f.Add(pdfHyphen(f))
	f.Add(encPDF(f, 3, "", "dono", []pdfPage{{pdfLine1}}))
	f.Add(encPDF(f, 2, "senha", "dono", []pdfPage{{pdfLine1}}))
	good := pdfThreePages(f)
	f.Add(good[:len(good)/2])
	f.Add([]byte("%PDF-1.4\n"))
	f.Add([]byte("%PDF-"))
	lim := Limits{MaxPDFPages: 50, MaxTextRunes: 1 << 16}
	f.Fuzz(func(t *testing.T, data []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		r, err := Extract(ctx, FormatPDF, data, lim)
		if err != nil {
			if _, ok := ReasonOf(err); !ok {
				t.Fatalf("erro não tipado: %v", err)
			}
			return
		}
		if r.Format != FormatPDF || r.Pages < 1 {
			t.Fatalf("resultado inválido: %+v", r)
		}
	})
}
