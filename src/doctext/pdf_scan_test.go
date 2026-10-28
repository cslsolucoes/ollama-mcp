package doctext

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"image"
	"strings"
	"testing"
)

// scanPage is one page of buildScanPDF: an 8-bit gray image drawn over the
// whole page (a scan), text lines (a text layer), both or neither (blank).
type scanPage struct {
	img  *image.Gray
	text []string
}

// buildScanPDF builds a US Letter PDF whose pages carry a DeviceGray image
// XObject (FlateDecode) and/or Helvetica text: a scanned document generated
// in code, no third-party file.
func buildScanPDF(tb testing.TB, pages []scanPage) []byte {
	tb.Helper()
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	var offs []int
	next := 1
	obj := func(num int, body []byte) {
		for len(offs) < num {
			offs = append(offs, 0)
		}
		offs[num-1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", num)
		out.Write(body)
		out.WriteString("\nendobj\n")
	}
	alloc := func() int { n := next; next++; return n }
	catalog, pagesObj, font := alloc(), alloc(), alloc()
	type ids struct{ page, content, img int }
	pid := make([]ids, len(pages))
	for i, p := range pages {
		pid[i].page, pid[i].content = alloc(), alloc()
		if p.img != nil {
			pid[i].img = alloc()
		}
	}
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", pid[i].page)
	}
	obj(catalog, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesObj)))
	obj(pagesObj, []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), strings.Join(kids, " "))))
	obj(font, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	stream := func(dict string, data []byte) []byte {
		b := []byte(fmt.Sprintf("<< %s /Length %d >>\nstream\n", dict, len(data)))
		b = append(b, data...)
		return append(b, "\nendstream"...)
	}
	for i, p := range pages {
		res := fmt.Sprintf("/Font << /F1 %d 0 R >>", font)
		var content bytes.Buffer
		if p.img != nil {
			res += fmt.Sprintf(" /XObject << /Im1 %d 0 R >>", pid[i].img)
			content.WriteString("q 612 0 0 792 0 0 cm /Im1 Do Q\n")
		}
		if len(p.text) > 0 {
			content.WriteString("BT /F1 12 Tf 72 720 Td 14 TL\n")
			for _, l := range p.text {
				content.Write(pdfLit(l))
				content.WriteString(" Tj T*\n")
			}
			content.WriteString("ET\n")
		}
		obj(pid[i].page, []byte(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << %s >> /Contents %d 0 R >>",
			pagesObj, res, pid[i].content)))
		obj(pid[i].content, stream("", content.Bytes()))
		if p.img != nil {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			w, h := p.img.Rect.Dx(), p.img.Rect.Dy()
			for y := 0; y < h; y++ {
				_, _ = zw.Write(p.img.Pix[y*p.img.Stride : y*p.img.Stride+w])
			}
			_ = zw.Close()
			obj(pid[i].img, stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode", w, h), z.Bytes()))
		}
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, catalog, xref)
	return out.Bytes()
}

// scanImage is a white 850x1100 "scan" with a dark band (ink).
func scanImage() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, 850, 1100))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 200; y < 260; y++ {
		for x := 100; x < 700; x++ {
			img.Pix[y*img.Stride+x] = 20
		}
	}
	return img
}

// TestScannedPDFWithRealEngine runs the real PDFium: a text page, a scanned
// page and a blank page. Only the scanned page reaches the (fake) OCR, at
// 300 dpi (2550x3300 for US Letter, PDFium rounds up to 3301), and a pixel
// ceiling lowers the dpi.
func TestScannedPDFWithRealEngine(t *testing.T) {
	data := buildScanPDF(t, []scanPage{
		{text: []string{pdfLine1, pdfLine2}},
		{img: scanImage()},
		{},
	})
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "texto da digitalização", 94, 0, nil
	}}
	r, err := Extract(context.Background(), FormatPDF, data, ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if sz := f.sizes[0]; f.calls() != 1 || sz.X < 2550 || sz.X > 2551 || sz.Y < 3300 || sz.Y > 3301 || f.dpis[0] != 300 {
		t.Fatalf("chamadas %d tamanhos %v dpi %v", f.calls(), f.sizes, f.dpis)
	}
	want := "[página 2 — OCR conf=94]\ntexto da digitalização\n\n[página 3]"
	if !strings.Contains(r.Text, want) || !strings.HasPrefix(r.Text, "[página 1]\n"+pdfLine1) {
		t.Fatalf("texto:\n%s", r.Text)
	}
	if r.Pages != 3 || r.PagesWithoutText != 2 || r.OCR != (OCRStats{Used: true, Pages: 1, ConfMean: 94, Blank: 1}) {
		t.Fatalf("páginas %d sem texto %d stats %+v", r.Pages, r.PagesWithoutText, r.OCR)
	}

	f = &fakeOCR{}
	if _, err := Extract(context.Background(), FormatPDF, data, ocrLimits(f, func(o *OCROptions) { o.MaxMegapixels = 2 })); err != nil {
		t.Fatal(err)
	}
	if sz := f.sizes[0]; sz.X*sz.Y > 2_000_000 || f.dpis[0] >= 300 {
		t.Fatalf("redução por megapixels com o motor real: %v a %d dpi", sz, f.dpis[0])
	}
	t.Logf("teto de 2 MP: %v a %d dpi", f.sizes[0], f.dpis[0])

	// Without OCR the same document keeps its text page and its markers.
	r, err = Extract(context.Background(), FormatPDF, data, Limits{})
	if err != nil || strings.Contains(r.Text, "OCR") || r.OCR.Used {
		t.Fatalf("sem OCR: %v %q", err, r.Text)
	}
	// And a fully scanned one is refused as before.
	if _, err := Extract(context.Background(), FormatPDF, buildScanPDF(t, []scanPage{{img: scanImage()}}), Limits{}); err == nil {
		t.Fatal("digitalizado sem OCR deveria ser recusado")
	} else if rr, _ := ReasonOf(err); rr != ReasonNoText {
		t.Fatalf("digitalizado sem OCR: %v", err)
	}
}
