package doctext

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fakes -----------------------------------------------------------------

// fakeOCR answers from fn and records every call. The page a fake render
// produced is identified by its first pixel (pageImage).
type fakeOCR struct {
	fn func(ctx context.Context, page int, img *image.Gray, dpi int) (string, int, int, error)

	mu    sync.Mutex
	pages []int
	dpis  []int
	sizes []image.Point

	inflight, peak atomic.Int32
}

func (f *fakeOCR) Recognize(ctx context.Context, img *image.Gray, dpi int) (string, int, int, error) {
	n := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	page := int(img.Pix[0])
	f.mu.Lock()
	f.pages = append(f.pages, page)
	f.dpis = append(f.dpis, dpi)
	f.sizes = append(f.sizes, img.Rect.Size())
	f.mu.Unlock()
	if f.fn == nil {
		return fmt.Sprintf("texto reconhecido da página %d", page+1), 93, 0, nil
	}
	return f.fn(ctx, page, img, dpi)
}

func (f *fakeOCR) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pages)
}

// pageImage is a fake rendering of page i at dpi (US Letter): white, with the
// first pixel set to the page index (dark: < 200) unless blank.
func pageImage(i, dpi int, blank bool) *image.Gray {
	w, h := int(math.Ceil(612*float64(dpi)/72)), int(math.Ceil(792*float64(dpi)/72))
	img := image.NewGray(image.Rect(0, 0, w, h))
	for k := range img.Pix {
		img.Pix[k] = 255
	}
	if !blank {
		img.Pix[0] = byte(i)
	}
	return img
}

// useFakeScan installs a fake PDF whose pages are texts[i] ("" = no text
// layer); pages in blank render white. It records the dpi of every render.
func useFakeScan(t *testing.T, texts []string, blank map[int]bool) (*fakeEngine, *[]int) {
	t.Helper()
	e := useFakePDF(t, texts)
	var mu sync.Mutex
	var dpis []int
	e.doc.render = func(_ context.Context, i, dpi int) (*image.Gray, error) {
		mu.Lock()
		dpis = append(dpis, dpi)
		mu.Unlock()
		return pageImage(i, dpi, blank[i]), nil
	}
	return e, &dpis
}

func ocrLimits(eng OCR, mut func(*OCROptions)) Limits {
	o := OCROptions{Engine: eng, Workers: 1}
	if mut != nil {
		mut(&o)
	}
	return Limits{OCR: &o}
}

const longText = "texto suficientemente longo para a pagina ter camada de texto"

// --- PDF ---------------------------------------------------------------------

func TestOCRMarkersStatsAndBlankPage(t *testing.T) {
	useFakeScan(t, []string{longText, "", "", " 4 "}, map[int]bool{2: true})
	f := &fakeOCR{fn: func(_ context.Context, page int, _ *image.Gray, _ int) (string, int, int, error) {
		if page == 3 {
			return "texto girado\r\nsegunda linha\f", 41, 180, nil
		}
		return "Cláusula 1ª — São Paulo", 93, 0, nil
	}}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	want := "[página 1]\n" + longText +
		"\n\n[página 2 — OCR conf=93]\nCláusula 1ª — São Paulo" +
		"\n\n[página 3]" +
		"\n\n[página 4 — OCR baixa confiança conf=41 girada=180]\ntexto girado\nsegunda linha"
	if r.Text != want {
		t.Fatalf("texto:\n%q\nesperado:\n%q", r.Text, want)
	}
	if r.Pages != 4 || r.PagesWithoutText != 3 {
		t.Fatalf("páginas %d, sem texto %d", r.Pages, r.PagesWithoutText)
	}
	wantStats := OCRStats{Used: true, Pages: 2, ConfMean: 67, Blank: 1}
	if r.OCR != wantStats {
		t.Fatalf("stats %+v, esperado %+v", r.OCR, wantStats)
	}
	if f.calls() != 2 {
		t.Fatalf("o OCR foi chamado %d vezes; a página branca não deveria ir ao motor", f.calls())
	}
	for _, d := range f.dpis {
		if d != DefaultOCRDPI {
			t.Fatalf("dpi %d, esperado %d", d, DefaultOCRDPI)
		}
	}
}

func TestOCROnlyPagesWithoutText(t *testing.T) {
	_, dpis := useFakeScan(t, []string{longText, longText}, nil)
	f := &fakeOCR{}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if f.calls() != 0 || len(*dpis) != 0 || r.OCR.Used {
		t.Fatalf("PDF com texto: OCR chamado %d vezes, %d renderizações, used=%v", f.calls(), len(*dpis), r.OCR.Used)
	}
	if strings.Contains(r.Text, "OCR") {
		t.Fatalf("marcador de OCR num PDF com texto: %q", r.Text)
	}
}

func TestOCRWholeScannedDocumentIsRead(t *testing.T) {
	useFakeScan(t, []string{"", "", ""}, nil)
	f := &fakeOCR{}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatalf("documento todo digitalizado com OCR deveria ser lido: %v", err)
	}
	if r.OCR.Pages != 3 || r.PagesWithoutText != 3 || !strings.Contains(r.Text, "[página 3 — OCR conf=93]\ntexto reconhecido da página 3") {
		t.Fatalf("resultado %+v\n%q", r.OCR, r.Text)
	}
}

func TestOCRWithoutEngineKeepsTheCurrentRefusal(t *testing.T) {
	useFakeScan(t, []string{"", ""}, nil)
	_, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), Limits{})
	if r, _ := ReasonOf(err); r != ReasonNoText {
		t.Fatalf("sem OCR: %v", err)
	}
	if err.Error() != "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado" {
		t.Fatalf("mensagem mudou: %q", err.Error())
	}
	// An OCROptions without an engine is no OCR.
	_, err = Extract(context.Background(), FormatPDF, []byte("%PDF-"), Limits{OCR: &OCROptions{}})
	if r, _ := ReasonOf(err); r != ReasonNoText {
		t.Fatalf("OCROptions sem motor: %v", err)
	}
}

func TestOCRNothingRecognizedIsRefused(t *testing.T) {
	useFakeScan(t, []string{"", ""}, map[int]bool{1: true})
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) { return " \n ", 0, 0, nil }}
	_, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonOCRNoText {
		t.Fatalf("nada reconhecido: %v", err)
	}
	// All blank: no OCR call at all, and the same refusal.
	useFakeScan(t, []string{"", ""}, map[int]bool{0: true, 1: true})
	f = &fakeOCR{}
	_, err = Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if r, _ := ReasonOf(err); r != ReasonOCRNoText || f.calls() != 0 {
		t.Fatalf("todas brancas: %v, %d chamadas", err, f.calls())
	}
}

func TestOCRPageFailureAndPanic(t *testing.T) {
	useFakeScan(t, []string{"", "", "", ""}, nil)
	f := &fakeOCR{fn: func(_ context.Context, page int, _ *image.Gray, _ int) (string, int, int, error) {
		switch page {
		case 1:
			return "", 0, 0, errors.New("falha simulada do motor")
		case 2:
			panic("motor em pânico")
		case 3:
			return strings.Repeat("�", 5) + "abc", 90, 0, nil // > 1% U+FFFD
		}
		return "ok página um", 88, 0, nil
	}}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"[página 1 — OCR conf=88]\nok página um", "[página 2 — OCR falhou]", "[página 3 — OCR falhou]", "[página 4 — OCR falhou]"} {
		if !strings.Contains(r.Text, m) {
			t.Fatalf("falta %q em\n%s", m, r.Text)
		}
	}
	if r.OCR.Pages != 1 || r.OCR.Failed != 3 || r.OCR.ConfMean != 88 {
		t.Fatalf("stats %+v", r.OCR)
	}
	if strings.ContainsRune(r.Text, '�') {
		t.Fatal("texto de página com U+FFFD em excesso vazou")
	}
}

func TestOCRUnavailableRefuses(t *testing.T) {
	useFakeScan(t, []string{longText, "", "", ""}, nil)
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "", 0, 0, fmt.Errorf("pool: %w", ErrOCRUnavailable)
	}}
	_, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, func(o *OCROptions) { o.Workers = 2 }))
	if r, _ := ReasonOf(err); r != ReasonOCRUnavailable {
		t.Fatalf("OCR indisponível: %v", err)
	}
}

func TestOCRTimeBudget(t *testing.T) {
	useFakeScan(t, []string{"", "", "", "", "", ""}, nil)
	f := &fakeOCR{fn: func(ctx context.Context, page int, _ *image.Gray, _ int) (string, int, int, error) {
		select {
		case <-time.After(400 * time.Millisecond):
			return "lida", 90, 0, nil
		case <-ctx.Done():
			return "", 0, 0, ctx.Err()
		}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := Extract(ctx, FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatalf("o orçamento deveria marcar as páginas restantes, não falhar: %v", err)
	}
	if el := time.Since(start); el > 1500*time.Millisecond {
		t.Fatalf("passou do prazo do ctx: %v", el)
	}
	s := r.OCR
	if s.Pages < 1 || s.NotProcessed < 1 || s.Pages+s.NotProcessed != 6 || s.Failed != 0 {
		t.Fatalf("stats %+v", s)
	}
	if n := strings.Count(r.Text, "OCR não processado: tempo esgotado]"); n != s.NotProcessed {
		t.Fatalf("%d marcadores de tempo esgotado, stats %d\n%s", n, s.NotProcessed, r.Text)
	}
	if !strings.Contains(r.Text, "[página 6 — OCR não processado: tempo esgotado]") {
		t.Fatalf("a última página deveria ficar sem processar:\n%s", r.Text)
	}
	t.Logf("orçamento: %d lidas, %d sem tempo, %v", s.Pages, s.NotProcessed, time.Since(start).Round(time.Millisecond))
}

func TestOCRCancelIsReturned(t *testing.T) {
	useFakeScan(t, []string{"", ""}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeOCR{fn: func(ctx context.Context, _ int, _ *image.Gray, _ int) (string, int, int, error) {
		cancel()
		<-ctx.Done()
		return "", 0, 0, ctx.Err()
	}}
	_, err := Extract(ctx, FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento: %v", err)
	}
}

func TestOCRMaxPages(t *testing.T) {
	// Three pages with ink and a ceiling of 2: refused before any OCR.
	_, dpis := useFakeScan(t, []string{"", "", "", longText}, nil)
	f := &fakeOCR{}
	_, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, func(o *OCROptions) { o.MaxPages = 2 }))
	if r, _ := ReasonOf(err); r != ReasonOCRTooManyPages {
		t.Fatalf("teto de páginas: %v", err)
	}
	if f.calls() != 0 {
		t.Fatalf("o OCR rodou %d páginas antes de recusar", f.calls())
	}
	if !strings.Contains(err.Error(), "documento digitalizado extenso demais para OCR nesta chamada") {
		t.Fatalf("mensagem: %q", err.Error())
	}
	for _, d := range *dpis {
		if d != ocrScreenDPI {
			t.Fatalf("a triagem deveria renderizar a %d dpi, não %d", ocrScreenDPI, d)
		}
	}
	// Blank pages do not count: 4 without text, 2 of them blank, ceiling 2.
	useFakeScan(t, []string{"", "", "", ""}, map[int]bool{0: true, 2: true})
	f = &fakeOCR{}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, func(o *OCROptions) { o.MaxPages = 2 }))
	if err != nil {
		t.Fatalf("páginas em branco não deveriam contar no teto: %v", err)
	}
	if r.OCR.Pages != 2 || r.OCR.Blank != 2 || f.calls() != 2 {
		t.Fatalf("stats %+v, chamadas %d", r.OCR, f.calls())
	}
}

func TestOCRReducesDPIToFitMegapixels(t *testing.T) {
	_, dpis := useFakeScan(t, []string{""}, nil)
	f := &fakeOCR{}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, func(o *OCROptions) { o.MaxMegapixels = 1 }))
	if err != nil {
		t.Fatalf("página acima do teto deveria ser reduzida, não recusada: %v", err)
	}
	if r.OCR.Pages != 1 || len(*dpis) != 1 {
		t.Fatalf("stats %+v, renderizações %v", r.OCR, *dpis)
	}
	d := (*dpis)[0]
	if d >= DefaultOCRDPI || d < 90 {
		t.Fatalf("dpi reduzido = %d", d)
	}
	sz := f.sizes[0]
	if sz.X*sz.Y > 1_000_000 {
		t.Fatalf("imagem %v passa de 1 MP", sz)
	}
	if f.dpis[0] != d {
		t.Fatalf("o OCR recebeu dpi %d, a página foi renderizada a %d", f.dpis[0], d)
	}
	t.Logf("Carta a 1 MP: %d dpi, %dx%d", d, sz.X, sz.Y)
}

func TestFitDPI(t *testing.T) {
	cases := []struct {
		w, h   float64
		want   int
		maxPix float64
	}{
		{612, 792, 300, 40e6},     // Letter fits at 300
		{595, 842, 300, 40e6},     // A4 fits
		{612, 792, 300, 1e6},      // reduced
		{14400, 14400, 300, 40e6}, // the largest PDF page: reduced to fit
		{14400, 10, 300, 40e6},    // a strip: limited by the dimension ceiling
		{1, 1, 1200, 40e6},
	}
	for _, c := range cases {
		d, err := fitDPI(c.w, c.h, c.want, c.maxPix)
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		w, h := math.Ceil(c.w*float64(d)/72), math.Ceil(c.h*float64(d)/72)
		if d > c.want || w*h > c.maxPix || w > ocrMaxDimension || h > ocrMaxDimension {
			t.Fatalf("%v: dpi %d dá %vx%v", c, d, w, h)
		}
		// The next dpi up would not fit (the reduction is not wasteful).
		if d < c.want {
			w2, h2 := math.Ceil(c.w*float64(d+2)/72)+1, math.Ceil(c.h*float64(d+2)/72)+1
			if w2*h2 <= c.maxPix && w2 <= ocrMaxDimension && h2 <= ocrMaxDimension {
				t.Fatalf("%v: dpi %d é baixo demais", c, d)
			}
		}
	}
	for _, bad := range [][2]float64{{0, 792}, {612, -1}, {math.NaN(), 1}, {math.Inf(1), 1}} {
		if _, err := fitDPI(bad[0], bad[1], 300, 40e6); err == nil {
			t.Fatalf("%v deveria falhar", bad)
		}
	}
}

func TestOCRParallelKeepsOrder(t *testing.T) {
	const n = 8
	texts := make([]string, n)
	e, _ := useFakeScan(t, texts, nil)
	// Small pages: the render (sequential) must be quicker than the OCR, as
	// in production, or there is nothing to overlap (seen under -race).
	e.doc.size = func(int) (float64, float64, error) { return 72, 72, nil }
	e.doc.render = func(_ context.Context, i, _ int) (*image.Gray, error) {
		img := image.NewGray(image.Rect(0, 0, 50, 50))
		for k := range img.Pix {
			img.Pix[k] = 255
		}
		img.Pix[0] = byte(i)
		return img, nil
	}
	f := &fakeOCR{fn: func(_ context.Context, page int, _ *image.Gray, _ int) (string, int, int, error) {
		time.Sleep(time.Duration(n-page) * 15 * time.Millisecond) // later pages finish first
		return fmt.Sprintf("conteúdo %d", page+1), 80 + page, 0, nil
	}}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, func(o *OCROptions) { o.Workers = 4 }))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for i := 0; i < n; i++ {
		want = append(want, fmt.Sprintf("[página %d — OCR conf=%d]\nconteúdo %d", i+1, 80+i, i+1))
	}
	if r.Text != strings.Join(want, "\n\n") {
		t.Fatalf("ordem:\n%s", r.Text)
	}
	if p := f.peak.Load(); p < 2 || p > 4 {
		t.Fatalf("pico de páginas em paralelo = %d (esperado 2..4)", p)
	}
	t.Logf("pico de paralelismo: %d", f.peak.Load())
}

func TestOCRTextCountsAgainstMaxTextRunes(t *testing.T) {
	useFakeScan(t, []string{"", ""}, nil)
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return strings.Repeat("a", 600), 90, 0, nil
	}}
	lim := ocrLimits(f, nil)
	lim.MaxTextRunes = 1000
	_, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), lim)
	if r, _ := ReasonOf(err); r != ReasonTooLong {
		t.Fatalf("teto de texto com OCR: %v", err)
	}
}

func TestOCRMarker(t *testing.T) {
	cases := []struct {
		p    ocrPage
		want string
	}{
		{ocrPage{status: ocrOK, conf: 60}, "[página 2 — OCR conf=60]"},
		{ocrPage{status: ocrOK, conf: 59}, "[página 2 — OCR baixa confiança conf=59]"},
		{ocrPage{status: ocrOK, conf: 95, rotated: 180}, "[página 2 — OCR conf=95 girada=180]"},
		{ocrPage{status: ocrFailed}, "[página 2 — OCR falhou]"},
		{ocrPage{status: ocrTimedOut}, "[página 2 — OCR não processado: tempo esgotado]"},
	}
	for _, c := range cases {
		if got := ocrMarker("página 2", c.p); got != c.want {
			t.Fatalf("%+v: %q", c.p, got)
		}
	}
}

func TestOCRFailureKeepsTheFewNativeRunes(t *testing.T) {
	useFakeScan(t, []string{longText, "pág. 2"}, nil)
	f := &fakeOCR{fn: func(context.Context, int, *image.Gray, int) (string, int, int, error) {
		return "", 0, 0, errors.New("falha")
	}}
	r, err := Extract(context.Background(), FormatPDF, []byte("%PDF-"), ocrLimits(f, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(r.Text, "[página 2 — OCR falhou]\npág. 2") {
		t.Fatalf("texto: %q", r.Text)
	}
}
