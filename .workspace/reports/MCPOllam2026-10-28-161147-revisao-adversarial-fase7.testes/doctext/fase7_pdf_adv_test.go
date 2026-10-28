package doctext

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// rawPDF writes objs as objects 1..n (bodies), with a classic xref.
func rawPDF(objs []string, trailerExtra string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offs := make([]int, len(objs))
	for i, b := range objs {
		offs[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, b)
	}
	x := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R %s >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, trailerExtra, x)
	return out.Bytes()
}

func pdfStream(dict string, data []byte) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

func pdfFlate(dict string, data []byte) string {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	w.Write(data)
	w.Close()
	return pdfStream(dict+" /Filter /FlateDecode", b.Bytes())
}

const pdfFont = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"

func pdfPageObj(contents string, extra string) string {
	return "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents " + contents + " " + extra + " >>"
}

func textOps(lines ...string) []byte {
	var b bytes.Buffer
	b.WriteString("BT /F1 12 Tf 72 720 Td 14 TL\n")
	for i, l := range lines {
		if i > 0 {
			b.WriteString("T*\n")
		}
		b.Write(pdfLit(l))
		b.WriteString(" Tj\n")
	}
	b.WriteString("ET\n")
	return b.Bytes()
}

func advPDF(data []byte, lim Limits) (*Result, error) {
	return Extract(context.Background(), FormatPDF, data, lim)
}

func pdfJSFormAttach() []byte {
	return rawPDF([]string{
		"<< /Type /Catalog /Pages 2 0 R /OpenAction 6 0 R /Names << /EmbeddedFiles 7 0 R /JavaScript 10 0 R >> /AcroForm << /Fields [9 0 R] /NeedAppearances true >> >>",
		"<< /Type /Pages /Count 1 /Kids [4 0 R] >>",
		pdfFont,
		pdfPageObj("5 0 R", "/Annots [9 0 R 11 0 R] /AA << /O 6 0 R >>"),
		pdfStream("", textOps("Texto visivel da pagina com mais de vinte letras")),
		"<< /S /JavaScript /JS (app.alert\\('JS-SENTINELA'\\); this.exportDataObject\\({cName: 'anexo.txt', nLaunch: 2}\\);) /Next << /S /Launch /F (cmd.exe) /Win << /F (cmd.exe) /P (/c echo LAUNCH-SENTINELA) >> >> >>",
		"<< /Names [(anexo.txt) 8 0 R] >>",
		"<< /Type /Filespec /F (anexo.txt) /UF (anexo.txt) /EF << /F 12 0 R >> >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (campo) /V (FORM-VALOR-SENTINELA) /Rect [72 600 300 620] /P 4 0 R /AP << /N 13 0 R >> /F 4 >>",
		"<< /Names [(js1) 6 0 R] >>",
		"<< /Type /Annot /Subtype /FreeText /Rect [72 500 300 520] /Contents (ANOTACAO-SENTINELA) /DA (/F1 12 Tf 0 g) /F 4 >>",
		pdfStream("/Type /EmbeddedFile", []byte("ANEXO-SENTINELA conteudo do arquivo anexado")),
		pdfStream("/Type /XObject /Subtype /Form /BBox [0 0 228 20] /Resources << /Font << /F1 3 0 R >> >>", []byte("BT /F1 10 Tf 2 5 Td (APARENCIA-SENTINELA) Tj ET")),
		"<< /Author (AUTOR-PDF-SENTINELA) /Title (TITULO-PDF-SENTINELA) /Subject (ASSUNTO-PDF-SENTINELA) >>",
	}, "/Info 14 0 R")
}

func pdfCycle(selfKid bool) []byte {
	if selfKid {
		return rawPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [2 0 R] >>", pdfFont}, "")
	}
	return rawPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [4 0 R] >>", pdfFont,
		"<< /Type /Pages /Count 1 /Kids [2 0 R] /Parent 2 0 R >>"}, "")
}

func TestAdvPDF(t *testing.T) {
	t.Cleanup(func() { _ = ClosePDF() })
	// 1. JavaScript, Launch, anexo, AcroForm, anotacao, metadados.
	{
		r, err := advPDF(pdfJSFormAttach(), Limits{})
		if err != nil {
			vec(t, "pdf", "js-anexo-form-anotacao", "ACHADO", reasonStr(err))
		} else {
			var leaked []string
			for _, s := range []string{"JS-SENTINELA", "LAUNCH-SENTINELA", "ANEXO-SENTINELA", "AUTOR-PDF-SENTINELA", "TITULO-PDF-SENTINELA", "ASSUNTO-PDF-SENTINELA"} {
				if strings.Contains(r.Text, s) {
					leaked = append(leaked, s)
				}
			}
			res := "BLOQUEADO"
			if len(leaked) > 0 {
				res = "ESCAPOU"
			}
			vec(t, "pdf", "js-launch-anexo-metadados", res, fmt.Sprintf("visivel=%v vazou=%v", strings.Contains(r.Text, "Texto visivel"), leaked))
			vec(t, "pdf", "acroform-valor-e-aparencia", "INFO", fmt.Sprintf("valor_campo=%v aparencia_widget=%v anotacao_freetext=%v (formulario preenchido nao vai para o modelo, como o brief decide; perda sem sinal no rodape)",
				strings.Contains(r.Text, "FORM-VALOR-SENTINELA"), strings.Contains(r.Text, "APARENCIA-SENTINELA"), strings.Contains(r.Text, "ANOTACAO-SENTINELA")))
		}
	}
	// 2. arvore de paginas ciclica.
	for _, self := range []bool{false, true} {
		name := map[bool]string{false: "ciclo-2-nos", true: "kid-de-si-mesmo"}[self]
		done := make(chan string, 1)
		go func() { _, err := advPDF(pdfCycle(self), Limits{}); done <- reasonStr(err) }()
		select {
		case r := <-done:
			vec(t, "pdf", "arvore-"+name, map[bool]string{true: "BLOQUEADO", false: "INFO"}[r != "ok"], r)
		case <-time.After(30 * time.Second):
			vec(t, "pdf", "arvore-"+name, "ESCAPOU", "travou > 30 s")
		}
	}
	// 3. stream com /F externo (dados num arquivo do disco).
	{
		dir := t.TempDir()
		ext := filepath.Join(dir, "externo.txt")
		os.WriteFile(ext, textOps("ARQUIVO-EXTERNO-LIDO pelo motor de PDF"), 0o644)
		spec := strings.ReplaceAll(filepath.ToSlash(ext), "(", "")
		data := rawPDF([]string{
			"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 2 /Kids [4 0 R 6 0 R] >>", pdfFont,
			pdfPageObj("5 0 R", ""), "<< /Length 0 /F (" + spec + ") >>\nstream\n\nendstream",
			pdfPageObj("7 0 R", ""), pdfStream("", textOps("Segunda pagina com texto normal e suficiente")),
		}, "")
		r, err := advPDF(data, Limits{})
		res := "BLOQUEADO"
		if r != nil && strings.Contains(r.Text, "ARQUIVO-EXTERNO-LIDO") {
			res = "ESCAPOU"
		}
		vec(t, "pdf", "stream-externo-F", res, reasonStr(err))
	}
	// 4. MaxPDFPages+1 (padrao 2000).
	{
		objs := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", pdfFont, pdfStream("", textOps("pagina repetida com texto suficiente"))}
		var kids []string
		for i := 0; i < 2001; i++ {
			objs = append(objs, pdfPageObj("4 0 R", ""))
			kids = append(kids, fmt.Sprintf("%d 0 R", len(objs)))
		}
		objs[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(kids), strings.Join(kids, " "))
		start := time.Now()
		_, err := advPDF(rawPDF(objs, ""), Limits{})
		r, _ := ReasonOf(err)
		vec(t, "pdf", "paginas-2001", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[r == ReasonMalformed], fmt.Sprintf("%s em %v", reasonStr(err), time.Since(start).Round(time.Millisecond)))
	}
	// 5. paginas mistas (texto + so imagem) e texto invisivel / branco.
	{
		img := pdfStream("/Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8", []byte{0x80})
		data := rawPDF([]string{
			"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 3 /Kids [4 0 R 6 0 R 9 0 R] >>", pdfFont,
			pdfPageObj("5 0 R", ""), pdfStream("", textOps("Pagina um com texto normal suficiente")),
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im1 8 0 R >> >> /Contents 7 0 R >>",
			pdfStream("", []byte("q 500 0 0 700 50 50 cm /Im1 Do Q")), img,
			pdfPageObj("10 0 R", ""), pdfStream("", []byte("BT /F1 12 Tf 72 720 Td 3 Tr (TEXTO-INVISIVEL-TR3 escondido) Tj 0 Tr 1 1 1 rg 0 -20 Td (TEXTO-BRANCO-NO-BRANCO escondido) Tj ET")),
		}, "")
		r, err := advPDF(data, Limits{})
		if err != nil {
			vec(t, "pdf", "paginas-mistas", "ACHADO", reasonStr(err))
		} else {
			vec(t, "pdf", "paginas-mistas", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[r.Pages == 3 && r.PagesWithoutText == 1 && strings.Contains(r.Text, "[página 2]")],
				fmt.Sprintf("paginas=%d sem_texto=%d", r.Pages, r.PagesWithoutText))
			vec(t, "pdf", "texto-invisivel", "INFO", fmt.Sprintf("Tr3=%v branco=%v (texto que o leitor nao ve chega ao modelo; necessario para PDF com OCR embutido)",
				strings.Contains(r.Text, "TEXTO-INVISIVEL-TR3"), strings.Contains(r.Text, "TEXTO-BRANCO-NO-BRANCO")))
		}
	}
	// 6. truncado no meio (3 paginas).
	{
		full := pdfThreePages(t)
		cut := full[:bytes.Index(full, []byte("8 0 obj"))]
		r, err := advPDF(cut, Limits{})
		pages := 0
		if r != nil {
			pages = r.Pages
		}
		vec(t, "pdf", "truncado-antes-pag3", "INFO", fmt.Sprintf("%s paginas=%d (original 3; o que um leitor de PDF tambem mostraria)", reasonStr(err), pages))
	}
	// 7. hifenizacao com encliticos (pt-BR) pelo motor real.
	{
		data := buildPDF(t, []pdfPage{{"A CONTRATADA obriga-", "se a entregar o objeto e o ex-", "empregado compromete-", "se tambem com o pre-", "contrato firmado"}}, false, nil, nil)
		r, err := advPDF(data, Limits{})
		if err != nil {
			vec(t, "pdf", "hifen-encliticos", "ACHADO", reasonStr(err))
		} else {
			flat := strings.ReplaceAll(r.Text, "\n", " ")
			lost := strings.Contains(flat, "obrigase") || strings.Contains(flat, "comprometese") || strings.Contains(flat, "exempregado") || strings.Contains(flat, "precontrato")
			kept := strings.Contains(flat, "obriga-se") && strings.Contains(flat, "compromete-se") && strings.Contains(flat, "ex-empregado") && strings.Contains(flat, "pre-contrato")
			res := "ACHADO"
			if !lost && kept {
				res = "FIEL" // passo 2 (B6): o hifen sempre fica
			}
			vec(t, "pdf", "hifen-encliticos", res, fmt.Sprintf("hifen_perdido=%v hifens_mantidos=%v", lost, kept))
		}
		vec(t, "pdf", "hifen-apos-digito-softhyphen", "INFO", fmt.Sprintf("pdfiumSoftHyphens(\"prazo 30\\x02dias\")=%q (o hifen fica; a linha so e juntada)", pdfiumSoftHyphens("prazo 30\x02dias")))
	}
	// 8. lixo antes do cabecalho (Detect aceita ate 1 KiB).
	{
		data := append(bytes.Repeat([]byte("J"), 1000), pdfASCII(t)...)
		f, derr := Detect("x.pdf", data[:min(len(data), 4096)])
		r, err := advPDF(data, Limits{})
		vec(t, "pdf", "lixo-antes-do-cabecalho", "INFO", fmt.Sprintf("detect=%v/%v extract=%s texto=%v", f, derr, reasonStr(err), r != nil && strings.Contains(r.Text, "Contrato")))
	}
}

// Conteudo de pagina enorme (bomba Flate): tempo, memoria do processo e se o
// --timeout interrompe DENTRO de uma pagina.
func TestAdvPDFContentBomb(t *testing.T) {
	// Sobe a memoria do processo em ate ~5 GiB sem a correcao: so roda quando pedido.
	if os.Getenv("MCPO_ADV_BOMB") == "" {
		t.Skip("defina MCPO_ADV_BOMB=1 para rodar (pico de varios GiB)")
	}
	t.Cleanup(func() { _ = ClosePDF() })
	// aquece o motor fora da medicao
	if _, err := advPDF(pdfASCII(t), Limits{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		ops  []byte
		n    int
	}{
		{"caminhos-1M", []byte("0 0 m 1 1 l S\n"), 1 << 20},
		{"caminhos-3M", []byte("0 0 m 1 1 l S\n"), 3 << 20},
		{"texto-1M-tj", []byte("(abcdefghij) Tj\n"), 1 << 20},
	} {
		content := append([]byte("BT /F1 12 Tf 72 720 Td\n"), bytes.Repeat(c.ops, c.n)...)
		content = append(content, "ET\n"...)
		data := rawPDF([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [4 0 R] >>", pdfFont,
			pdfPageObj("5 0 R", ""), pdfFlate("", content)}, "")
		var err error
		var el time.Duration
		runtime.GC()
		debug.FreeOSMemory()
		before := privateBytes()
		residual := func() {
			runtime.GC()
			debug.FreeOSMemory()
			time.Sleep(500 * time.Millisecond)
			runtime.GC()
			debug.FreeOSMemory()
			resid := int64(privateBytes()) - int64(before)
			vec(t, "rec", "pdf-bomba-residuo-"+c.name, "INFO", fmt.Sprintf("memoria privada apos GC+FreeOSMemory: %+.1f MiB em relacao a antes", float64(resid)/(1<<20)))
		}
		priv := peakPrivate(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			s := time.Now()
			_, err = Extract(ctx, FormatPDF, data, Limits{})
			el = time.Since(s)
		})
		res := "INFO"
		if el > 4*time.Second {
			res = "ACHADO"
		}
		vec(t, "rec", "pdf-bomba-conteudo-"+c.name, res, fmt.Sprintf("pdf=%d B, conteudo=%d MiB; timeout=2s, durou %v; %s; pico de memoria privada do processo=+%s",
			len(data), len(content)>>20, el.Round(time.Millisecond), reasonStr(err), mib(priv)))
		residual()
	}
}

// O WASM nao escreve em stdout/stderr (stdout e o JSON-RPC), nem durante a
// inicializacao do motor.
func TestAdvPDFNoStdoutStderr(t *testing.T) {
	_ = ClosePDF()
	t.Cleanup(func() { _ = ClosePDF() })
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	got := make(chan [2]int, 1)
	var outB, errB bytes.Buffer
	go func() {
		d1 := make(chan struct{})
		go func() { io.Copy(&outB, ro); close(d1) }()
		io.Copy(&errB, re)
		<-d1
		got <- [2]int{outB.Len(), errB.Len()}
	}()
	os.Stdout, os.Stderr = wo, we
	results := []string{}
	for _, d := range [][]byte{pdfJSFormAttach(), pdfCycle(false), pdfThreePages(t)[:300], []byte("%PDF-1.7\n" + strings.Repeat("garbage obj stream endobj ", 2000)), pdfAccents(t)} {
		_, err := advPDF(d, Limits{})
		results = append(results, reasonStr(err))
	}
	_, _ = pdfRenderPage(context.Background(), pdfASCII(t), Limits{}, 0, 72)
	os.Stdout, os.Stderr = oldOut, oldErr
	wo.Close()
	we.Close()
	n := <-got
	res := "BLOQUEADO"
	if n[0] > 0 || n[1] > 0 {
		res = "ESCAPOU"
	}
	vec(t, "pdf", "wasm-stdout-stderr", res, fmt.Sprintf("init do motor + 5 PDFs (%v) + render: stdout=%d B stderr=%d B", results, n[0], n[1]))
}

// Panic no motor: vira recusa e libera o slot (o PDF seguinte funciona).
type panicEngine struct{}

func (panicEngine) open(context.Context, []byte) (pdfDoc, error) { return panicDoc{}, nil }

type panicDoc struct{}

func (panicDoc) pages() int                                       { return 1 }
func (panicDoc) pageText(context.Context, int) (string, error)    { panic("explodiu com TEXTO-DO-CLIENTE") }
func (panicDoc) renderGray(context.Context, int, int) (*image.Gray, error) { panic("x") }
func (panicDoc) pageSize(int) (float64, float64, error)              { panic("x") } // 7.9: pdfDoc ganhou pageSize
func (panicDoc) close()                                           {}

func TestAdvPDFPanicReleasesSlot(t *testing.T) {
	t.Cleanup(func() { _ = ClosePDF() })
	pdfState.test = panicEngine{}
	_, err := advPDF(pdfASCII(t), Limits{})
	pdfState.test = nil
	r, _ := ReasonOf(err)
	leak := err != nil && strings.Contains(err.Error(), "TEXTO-DO-CLIENTE")
	done := make(chan error, 1)
	go func() { _, e := advPDF(pdfASCII(t), Limits{}); done <- e }()
	select {
	case e := <-done:
		res := "BLOQUEADO"
		if r != ReasonMalformed || leak || e != nil {
			res = "ESCAPOU"
		}
		vec(t, "pdf", "panic-no-motor-libera-slot", res, fmt.Sprintf("panic->%s vazou_msg=%v; PDF seguinte: %s", reasonStr(err), leak, reasonStr(e)))
	case <-time.After(60 * time.Second):
		vec(t, "pdf", "panic-no-motor-libera-slot", "ESCAPOU", "o PDF seguinte travou: slot nao liberado")
	}
	// panic em cada extrator via hook
	for _, f := range []Format{FormatDOCX, FormatPPTX, FormatPDF} {
		testHookExtract = func(Format) { panic("hook") }
		_, err := Extract(context.Background(), f, []byte("x"), Limits{})
		testHookExtract = nil
		r, _ := ReasonOf(err)
		vec(t, "panic", "hook-"+f.String(), map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[r == ReasonMalformed], reasonStr(err))
	}
}

func TestAdvPDFDeterminism(t *testing.T) {
	t.Cleanup(func() { _ = ClosePDF() })
	for name, data := range map[string][]byte{"tres-paginas": pdfThreePages(t), "acentos": pdfAccents(t), "js-form": pdfJSFormAttach()} {
		h := map[[32]byte]bool{}
		for i := 0; i < 20; i++ {
			r, err := advPDF(data, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			h[sha256.Sum256([]byte(r.Text))] = true
		}
		vec(t, "det", "pdf-20x-"+name, map[bool]string{true: "FIEL", false: "SILENCIOSO"}[len(h) == 1], fmt.Sprintf("%d hash(es)", len(h)))
	}
}
