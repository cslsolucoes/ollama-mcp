package doctext

import (
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"strings"
	"testing"
	"time"
)

// --- helpers ---------------------------------------------------------------

func extractPDFBytes(t *testing.T, data []byte) (*Result, error) {
	t.Helper()
	return Extract(context.Background(), FormatPDF, data, Limits{})
}

func mustPDF(t *testing.T, data []byte) *Result {
	t.Helper()
	r, err := extractPDFBytes(t, data)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return r
}

// --- tests with the real engine ---------------------------------------------

func TestPDFASCII(t *testing.T) {
	r := mustPDF(t, pdfASCII(t))
	want := "[página 1]\n" + pdfLine1 + "\n" + pdfLine2
	if r.Text != want {
		t.Fatalf("texto = %q, queria %q", r.Text, want)
	}
	if r.Pages != 1 || r.PagesWithoutText != 0 || r.Format != FormatPDF {
		t.Fatalf("resultado = %+v", r)
	}
}

func TestPDFAccents(t *testing.T) {
	r := mustPDF(t, pdfAccents(t))
	for _, w := range []string{"Ação de cobrança, concessão e opinião é válida", "Título: coração, órgão, lição"} {
		if !strings.Contains(r.Text, w) {
			t.Fatalf("falta %q em %q", w, r.Text)
		}
	}
}

func TestPDFThreePagesMarkers(t *testing.T) {
	r := mustPDF(t, pdfThreePages(t))
	want := "[página 1]\nPrimeira pagina com texto suficiente para contar\n\n" +
		"[página 2]\nSegunda pagina com texto suficiente para contar\n\n" +
		"[página 3]\nTerceira pagina com texto suficiente para contar"
	if r.Text != want {
		t.Fatalf("texto = %q", r.Text)
	}
	if r.Pages != 3 || r.PagesWithoutText != 0 {
		t.Fatalf("resultado = %+v", r)
	}
}

func TestPDFBlankMiddlePage(t *testing.T) {
	r := mustPDF(t, pdfBlankMiddle(t))
	if r.Pages != 3 || r.PagesWithoutText != 1 {
		t.Fatalf("resultado = %+v", r)
	}
	if !strings.Contains(r.Text, "[página 2]\n\n[página 3]") {
		t.Fatalf("a página em branco deveria ter só o marcador: %q", r.Text)
	}
}

func TestPDFAllBlankIsNoText(t *testing.T) {
	_, err := extractPDFBytes(t, pdfAllBlank(t))
	wantReason(t, err, ReasonNoText)
}

func TestPDFHyphenation(t *testing.T) {
	r := mustPDF(t, pdfHyphen(t))
	for _, w := range []string{
		"Este contra-to termina aqui e continua", // joined, hyphen kept (B6): next line starts lowercase
		"Rio-\nSao nao junta",                    // capital: kept
		"valor 10 -\nmais nao junta",             // hyphen after a space: kept
	} {
		if !strings.Contains(r.Text, w) {
			t.Fatalf("falta %q em %q", w, r.Text)
		}
	}
}

func TestPDFTruncatedAndGarbage(t *testing.T) {
	good := pdfThreePages(t)
	cases := map[string][]byte{
		"cabeçalho cortado": good[:20],
		"só cabeçalho":      []byte("%PDF-1.4\n"),
		"lixo":              []byte("%PDF-1.4\n\x00\x01\x02 not a pdf at all \xff\xfe"),
		"vazio":             {},
	}
	for name, data := range cases {
		_, err := extractPDFBytes(t, data)
		if err == nil {
			t.Fatalf("%s: esperava recusa", name)
		}
		wantReason(t, err, ReasonMalformed)
	}
	// A cut in the middle may or may not be recoverable by the engine: it must
	// never panic and the outcome must be a typed refusal or a text.
	for _, n := range []int{len(good) / 2, len(good) - 40, len(good) - 1} {
		r, err := extractPDFBytes(t, good[:n])
		if err != nil {
			if _, ok := ReasonOf(err); !ok {
				t.Fatalf("corte em %d: erro não tipado: %v", n, err)
			}
			continue
		}
		if r.Pages < 1 {
			t.Fatalf("corte em %d: resultado sem páginas", n)
		}
	}
}

func TestPDFPermissionPasswordOpens(t *testing.T) {
	pages := []pdfPage{{pdfLine1}, {pdfLine2}}
	for _, rev := range []int{2, 3} {
		data := encPDF(t, rev, "", "dono-secreto", pages)
		if !strings.Contains(string(data), "/Encrypt") {
			t.Fatal("o fixture deveria estar cifrado")
		}
		if strings.Contains(string(data), pdfLine1) {
			t.Fatal("o fixture cifrado não deveria ter o texto em claro")
		}
		r, err := extractPDFBytes(t, data)
		if err != nil {
			t.Fatalf("rev %d: %v", rev, err)
		}
		want := "[página 1]\n" + pdfLine1 + "\n\n[página 2]\n" + pdfLine2
		if r.Text != want {
			t.Fatalf("rev %d: texto = %q", rev, r.Text)
		}
	}
}

func TestPDFOpenPasswordRefused(t *testing.T) {
	pages := []pdfPage{{pdfLine1}}
	for _, rev := range []int{2, 3} {
		_, err := extractPDFBytes(t, encPDF(t, rev, "abre-se-com-isto", "dono", pages))
		wantReason(t, err, ReasonPDFPassword)
	}
}

func TestPDFMaxPages(t *testing.T) {
	data := pdfThreePages(t)
	if _, err := Extract(context.Background(), FormatPDF, data, Limits{MaxPDFPages: 3}); err != nil {
		t.Fatalf("3 páginas com limite 3: %v", err)
	}
	_, err := Extract(context.Background(), FormatPDF, data, Limits{MaxPDFPages: 2})
	wantReason(t, err, ReasonMalformed)
}

func TestPDFMaxTextRunes(t *testing.T) {
	_, err := Extract(context.Background(), FormatPDF, pdfThreePages(t), Limits{MaxTextRunes: 60})
	wantReason(t, err, ReasonTooLong)
}

func TestPDFContext(t *testing.T) {
	data := pdfThreePages(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, FormatPDF, data, Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelado: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	_, err := Extract(ctx, FormatPDF, data, Limits{})
	wantReason(t, err, ReasonTimeout)
}

func TestPDFDeterminism(t *testing.T) {
	for name, data := range map[string][]byte{"acentos": pdfAccents(t), "3pag": pdfThreePages(t), "cifrado": encPDF(t, 3, "", "x", []pdfPage{{pdfLine1}})} {
		var first [32]byte
		for i := 0; i < 20; i++ {
			r := mustPDF(t, data)
			h := sha256.Sum256([]byte(r.Text))
			if i == 0 {
				first = h
			} else if h != first {
				t.Fatalf("%s: extração %d difere da primeira", name, i)
			}
		}
	}
}

func TestPDFRenderGray(t *testing.T) {
	data := pdfASCII(t)
	for _, c := range []struct{ dpi, w, h int }{{72, 612, 792}, {144, 1224, 1584}} {
		img, err := pdfRenderPage(context.Background(), data, Limits{}, 0, c.dpi)
		if err != nil {
			t.Fatalf("dpi %d: %v", c.dpi, err)
		}
		if b := img.Bounds(); b.Dx() != c.w || b.Dy() != c.h {
			t.Fatalf("dpi %d: %dx%d, queria %dx%d", c.dpi, b.Dx(), b.Dy(), c.w, c.h)
		}
		dark := 0
		for _, v := range img.Pix {
			if v < 128 {
				dark++
			}
		}
		if dark == 0 {
			t.Fatalf("dpi %d: a página renderizada não tem pixels escuros (texto)", c.dpi)
		}
	}
	if _, err := pdfRenderPage(context.Background(), data, Limits{}, 5, 72); err == nil {
		t.Fatal("página fora do intervalo deveria falhar")
	}
	if _, err := pdfRenderPage(context.Background(), data, Limits{}, 0, 0); err == nil {
		t.Fatal("dpi 0 deveria falhar")
	}
}

// TestPDFEngineLifecycle measures the cold start without and with the wazero
// cache, and checks ConfigurePDF/ClosePDF. It restarts the engine twice.
func TestPDFEngineLifecycle(t *testing.T) {
	data := pdfASCII(t)
	if err := ClosePDF(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ClosePDF()
		_ = ConfigurePDF(PDFOptions{})
	})
	timeFirst := func(label string) {
		t0 := time.Now()
		mustPDF(t, data)
		first := time.Since(t0)
		t0 = time.Now()
		mustPDF(t, data)
		t.Logf("%s: 1ª extração %v (inclui a inicialização do motor); 2ª %v", label, first.Round(time.Millisecond), time.Since(t0).Round(time.Millisecond))
	}

	if err := ConfigurePDF(PDFOptions{}); err != nil {
		t.Fatal(err)
	}
	timeFirst("sem cache")
	if err := ConfigurePDF(PDFOptions{CacheDir: "x"}); err == nil {
		t.Fatal("ConfigurePDF depois do início deveria falhar")
	}
	if err := ClosePDF(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := ConfigurePDF(PDFOptions{CacheDir: dir}); err != nil {
		t.Fatal(err)
	}
	timeFirst("com cache, cache vazio")
	if err := ClosePDF(); err != nil {
		t.Fatal(err)
	}
	timeFirst("com cache, cache preenchido (reinício do motor)")
	if err := ClosePDF(); err != nil {
		t.Fatal(err)
	}
	if err := ClosePDF(); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestPDFTimePerPage(t *testing.T) {
	var pages []pdfPage
	for i := 0; i < 20; i++ {
		pages = append(pages, pdfPage{pdfLine1, pdfLine2, "linha tres com mais algum texto para a pagina"})
	}
	data := buildPDF(t, pages, true, nil, nil)
	mustPDF(t, data) // warm
	const reps = 5
	t0 := time.Now()
	for i := 0; i < reps; i++ {
		mustPDF(t, data)
	}
	el := time.Since(t0)
	t.Logf("20 páginas x %d: %v por extração, %.2f ms/página (inclui abrir o documento)", reps, (el / reps).Round(time.Microsecond), float64(el.Microseconds())/float64(reps*20)/1000)
}

// --- tests with a fake engine -----------------------------------------------

type fakeDoc struct {
	texts    []string
	failPage int // -1: none
	panicOn  int // -1: none
	closed   *int
	delay    func(i int)
}

func (d *fakeDoc) pages() int { return len(d.texts) }
func (d *fakeDoc) pageText(_ context.Context, i int) (string, error) {
	if d.delay != nil {
		d.delay(i)
	}
	if i == d.panicOn {
		panic("boom")
	}
	if i == d.failPage {
		return "", errors.New("page failed")
	}
	return d.texts[i], nil
}
func (d *fakeDoc) renderGray(context.Context, int, int) (*image.Gray, error) {
	return nil, errors.New("not implemented")
}
func (d *fakeDoc) close() { *d.closed++ }

type fakeEngine struct {
	doc     *fakeDoc
	openErr error
	closed  int
}

func (e *fakeEngine) open(context.Context, []byte) (pdfDoc, error) {
	if e.openErr != nil {
		return nil, e.openErr
	}
	e.doc.closed = &e.closed
	return e.doc, nil
}

func useFakePDF(t *testing.T, texts []string) *fakeEngine {
	t.Helper()
	e := &fakeEngine{doc: &fakeDoc{texts: texts, failPage: -1, panicOn: -1}}
	pdfState.test = e
	t.Cleanup(func() { pdfState.test = nil })
	return e
}

func TestFakeEngineMarkersFooterAndHyphen(t *testing.T) {
	long := "texto suficientemente longo para a pagina"
	e := useFakePDF(t, []string{
		long + " con-\r\ntrato e Rio-\r\nSão e 10 -\r\nmais e a-\r\n\r\nb e fim-\r\n",
		"  \r\n ", // blank
		"curto",   // < 20 non-blank runes
		long,
	})
	r, err := Extract(context.Background(), FormatPDF, nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	want := "[página 1]\n" + long + " con-\ntrato e Rio-\nSão e 10 -\nmais e a-\n\nb e fim-\n\n" +
		"[página 2]\n\n[página 3]\ncurto\n\n[página 4]\n" + long
	// the trailing "fim-\n" is trimmed by TrimSpace (page end), so it keeps the hyphen only
	want = strings.Replace(want, "fim-\n\n[página 2]", "fim-\n\n[página 2]", 1)
	if r.Text != want {
		t.Fatalf("texto =\n%q\nqueria\n%q", r.Text, want)
	}
	if r.Pages != 4 || r.PagesWithoutText != 2 {
		t.Fatalf("resultado = %+v", r)
	}
	if e.closed != 1 {
		t.Fatalf("close chamado %d vezes", e.closed)
	}
}

func TestFakeEngineHyphenNotAcrossPages(t *testing.T) {
	long := "texto suficientemente longo para a pagina"
	useFakePDF(t, []string{long + " con-", "trato " + long})
	r, err := Extract(context.Background(), FormatPDF, nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "con-\n\n[página 2]\ntrato") {
		t.Fatalf("hífen não deve atravessar páginas: %q", r.Text)
	}
}

func TestFakeEngineAllBlank(t *testing.T) {
	e := useFakePDF(t, []string{"", "abc", "   "})
	_, err := Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonNoText)
	if e.closed != 1 {
		t.Fatalf("close chamado %d vezes", e.closed)
	}
}

func TestFakeEngineErrors(t *testing.T) {
	long := "texto suficientemente longo para a pagina"
	// password
	e := useFakePDF(t, nil)
	e.openErr = errPDFPassword
	_, err := Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonPDFPassword)
	// generic open error
	e.openErr = errors.New("x")
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonMalformed)
	// zero pages
	e = useFakePDF(t, nil)
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonMalformed)
	// page error
	e = useFakePDF(t, []string{long, long})
	e.doc.failPage = 1
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonMalformed)
	if e.closed != 1 {
		t.Fatalf("close chamado %d vezes após erro", e.closed)
	}
	// panic inside the engine: refusal, document closed, slot released
	e = useFakePDF(t, []string{long, long})
	e.doc.panicOn = 1
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonMalformed)
	if e.closed != 1 {
		t.Fatalf("close chamado %d vezes após panic", e.closed)
	}
	e.doc.panicOn = -1
	if _, err = Extract(context.Background(), FormatPDF, nil, Limits{}); err != nil {
		t.Fatalf("o motor deveria seguir utilizável após um panic: %v", err)
	}
	// too long
	useFakePDF(t, []string{long, long, long})
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{MaxTextRunes: 80})
	wantReason(t, err, ReasonTooLong)
	// corrupt text (U+FFFD > 1%) through NormalizeText
	useFakePDF(t, []string{strings.Repeat("�", 30) + " " + long})
	_, err = Extract(context.Background(), FormatPDF, nil, Limits{})
	wantReason(t, err, ReasonCorruptText)
}

func TestFakeEngineContextBetweenPages(t *testing.T) {
	long := "texto suficientemente longo para a pagina"
	e := useFakePDF(t, []string{long, long, long})
	ctx, cancel := context.WithCancel(context.Background())
	e.doc.delay = func(i int) {
		if i == 0 {
			cancel()
		}
	}
	_, err := Extract(ctx, FormatPDF, nil, Limits{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if e.closed != 1 {
		t.Fatalf("close chamado %d vezes", e.closed)
	}
}

// A literal "-" at the end of a line (from an engine that does not mark it) is
// never joined nor dropped: the text keeps the hyphen and the line break.
func TestPlainHyphenAtLineEndIsKept(t *testing.T) {
	long := "texto suficientemente longo para a pagina"
	useFakePDF(t, []string{long + " obriga-\nse e con-\nto e ex-\nempregado"})
	r, err := Extract(context.Background(), FormatPDF, nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "obriga-\nse e con-\nto e ex-\nempregado") {
		t.Fatalf("hífen ou quebra alterados: %q", r.Text)
	}
}

// B6 (owner decision after 7.7a): the U+0002 of PDFium is the same for a real
// hyphen and a syllable break, so the hyphen always stays; the line is joined
// only before a lowercase letter.
func TestPDFiumSoftHyphens(t *testing.T) {
	cases := map[string]string{
		"obriga\x02se":        "obriga-se",
		"compromete\x02se":    "compromete-se",
		"ex\x02empregado":     "ex-empregado",
		"con\x02trato":        "con-trato",
		"Rio\x02São":          "Rio-\nSão",
		"ab\x021990":          "ab-\n1990",
		"fim\x02":             "fim-\n",
		"co\x02ação x\x02y":   "co-ação x-y",
		"prazo de 30\x02dias": "prazo de 30-dias",
		"sem marca":           "sem marca",
	}
	for in, want := range cases {
		if got := pdfiumSoftHyphens(in); got != want {
			t.Errorf("pdfiumSoftHyphens(%q) = %q, queria %q", in, got, want)
		}
	}
}
