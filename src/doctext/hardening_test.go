package doctext

// Regression tests for the fixes of the Phase 7 adversarial review (7.7a,
// report MCPOllam2026-10-28-161147) and of P7.7-3 (acceptance round 2).

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// P7.7-3: an empty revision marker is not a pending revision; a revision with
// runs or text is; property changes count as before.
func TestDOCXPendingRevisionsNeedContent(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"ins vazio em numPr (modelo da Microsoft)", `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/><w:ins w:id="1" w:author="a" w:date="2020-01-01T00:00:00Z"/></w:numPr></w:pPr><w:r><w:t>Título</w:t></w:r></w:p>`, false},
		{"ins vazio entre runs", `<w:p><w:r><w:t>a</w:t></w:r><w:ins w:id="1" w:author="a"/><w:r><w:t>b</w:t></w:r></w:p>`, false},
		{"del vazio na marca de paragrafo", `<w:p><w:pPr><w:rPr><w:del w:id="2" w:author="a"/></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`, false},
		{"ins vazio na marca de paragrafo", `<w:p><w:pPr><w:rPr><w:ins w:id="2" w:author="a"/></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`, false},
		{"moveFrom e moveTo vazios", `<w:p><w:moveFrom w:id="3" w:author="a"/><w:moveTo w:id="4" w:author="a"/><w:r><w:t>x</w:t></w:r></w:p>`, false},
		{"ins com run", `<w:p><w:ins w:id="1" w:author="a"><w:r><w:t>novo</w:t></w:r></w:ins></w:p>`, true},
		{"del com delText", `<w:p><w:del w:id="1" w:author="a"><w:r><w:delText>velho</w:delText></w:r></w:del></w:p>`, true},
		{"moveFrom com run", `<w:p><w:moveFrom w:id="1" w:author="a"><w:r><w:t>m</w:t></w:r></w:moveFrom></w:p>`, true},
		{"moveTo com run", `<w:p><w:moveTo w:id="1" w:author="a"><w:r><w:t>m</w:t></w:r></w:moveTo></w:p>`, true},
		{"del de paragrafo inteiro", `<w:del w:id="1" w:author="a"><w:p><w:r><w:delText>x</w:delText></w:r></w:p></w:del>` + dxP("y"), true},
		{"rPrChange (formatacao) continua contando", `<w:p><w:r><w:rPr><w:b/><w:rPrChange w:id="1" w:author="a"><w:rPr/></w:rPrChange></w:rPr><w:t>x</w:t></w:r></w:p>`, true},
		{"pPrChange continua contando", `<w:p><w:pPr><w:pPrChange w:id="1" w:author="a"><w:pPr/></w:pPrChange></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`, true},
		{"sem revisao", dxP("x"), false},
	}
	for _, c := range cases {
		r := dxExtract(t, dxBuild(t, c.body))
		if r.PendingRevisions != c.want {
			t.Errorf("%s: PendingRevisions = %v, want %v", c.name, r.PendingRevisions, c.want)
		}
	}
	// the empty marker does not change the text either
	if got := dxExtract(t, dxBuild(t, cases[0].body)).Text; got != "Título\n" {
		t.Errorf("texto = %q", got)
	}
}

// A1: w:lvlText is copied into every numbered paragraph; an absurd one falls
// back to "- ", and the text of a table cell counts against the ceiling while
// the row is still open.
func TestDOCXLvlTextAmplificationBounded(t *testing.T) {
	long := strings.Repeat("%2", 64<<10)
	num := `<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="` + long + `"/></w:lvl>` +
		`<w:lvl w:ilvl="1"><w:start w:val="1664"/><w:numFmt w:val="upperLetter"/><w:lvlText w:val="x"/></w:lvl></w:abstractNum>` + nbNum("1", "0", "")
	body := `<w:tbl><w:tr><w:tc>` + strings.Repeat(nbP("1", 0, "item"), 32) + `</w:tc></w:tr></w:tbl>`
	data := nbDocx(t, num, "", body)
	var r *Result
	var err error
	peak, _ := peakHeap(func() { r, err = Extract(context.Background(), FormatDOCX, data, Limits{}) })
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.HasPrefix(r.Text, "- item - item") {
		t.Errorf("texto = %.80q", r.Text)
	}
	if peak > 32<<20 {
		t.Errorf("pico de heap %d MiB para um docx de %d bytes", peak>>20, len(data))
	}
	// a real lvlText is untouched
	ok := nbAbs("0", nbLvl(0, "decimal", "CLÁUSULA %1ª", nbSuff("space"))) + nbNum("1", "0", "")
	dxWant(t, nbDocx(t, ok, "", nbP("1", 0, "Do objeto")), "CLÁUSULA 1ª Do objeto\n")
}

func TestDOCXCellTextCountsAgainstCeiling(t *testing.T) {
	cell := strings.Repeat(dxP(strings.Repeat("a", 100)), 50) // 50 lines of 100 runes in ONE cell
	data := dxBuild(t, `<w:tbl><w:tr><w:tc>`+cell+`</w:tc><w:tc>`+dxP("b")+`</w:tc></w:tr></w:tbl>`)
	r := dxExtract(t, data)
	n := len([]rune(r.Text))
	// the ceiling at the size of the text is enough: a row is not charged twice
	if _, err := Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: n}); err != nil {
		t.Fatalf("teto = tamanho do texto (%d): %v", n, err)
	}
	// below it, the cell is refused before the row ends
	_, err := Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: 1000})
	if r, _ := ReasonOf(err); r != ReasonTooLong {
		t.Fatalf("err = %v, want ReasonTooLong", err)
	}
}

// M1: a field still in its code part at the end of a part hid the rest of
// the document: refused, never truncated in silence.
func TestDOCXUnbalancedFieldRefused(t *testing.T) {
	for name, body := range map[string]string{
		"begin sem fim": dxP("um") + `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r></w:p>` + dxP("tres"),
		"separate e end em w:del": `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> REF x </w:instrText></w:r>` +
			`<w:del w:id="1" w:author="a"><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:del>` +
			`<w:r><w:t>depois</w:t></w:r></w:p>` + dxP("seguinte"),
	} {
		_, err := Extract(context.Background(), FormatDOCX, dxBuild(t, body), Limits{})
		if r, _ := ReasonOf(err); r != ReasonMalformed {
			t.Errorf("%s: err = %v, want ReasonMalformed", name, err)
		}
	}
	// balanced and nested fields still work, and an orphan end is harmless
	dxWant(t, dxBuild(t, `<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>ok</w:t></w:r></w:p>`), "ok\n")
}

// B1 + B2: part names are case-insensitive (OPC); a header rel that points at
// the body or a notes part is ignored instead of duplicating it.
func TestDOCXRelTargetsCaseAndNoDuplicate(t *testing.T) {
	sect := `<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`
	data := dxBuild(t, dxP("corpo")+sect,
		entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="Header1.xml"/>`)},
		entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("timbre"))})
	dxWant(t, data, labelHeader+"\ntimbre\n\ncorpo\n")
	for _, target := range []string{"document.xml", "Document.XML", "footnotes.xml"} {
		data := dxBuild(t, `<w:p><w:r><w:t>corpo-unico</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`+sect,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="` + target + `"/>`)},
			entry{name: "word/footnotes.xml", data: dxPartXML("footnotes", `<w:footnote w:id="1">`+dxP("nota-unica")+`</w:footnote>`)})
		txt := dxExtract(t, data).Text
		if strings.Count(txt, "corpo-unico") != 1 || strings.Count(txt, "nota-unica") != 1 || strings.Contains(txt, labelHeader) {
			t.Errorf("alvo %s: texto duplicado: %q", target, txt)
		}
	}
}

// B1 (PPTX), B3, B4, B5.
func TestPPTXHardening(t *testing.T) {
	// B1: notes target with another case
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("S1"))},
		entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(notesRel("../notesSlides/NotesSlide1.xml"))},
		entry{name: "ppt/notesSlides/notesSlide1.xml", data: notesXML(phShape("body", "nota"))})
	if got := runPPTX(t, data).Text; !strings.Contains(got, "[notas do slide 1]\nnota") {
		t.Errorf("notas com caixa diferente: %q", got)
	}
	// B3: the same slide listed twice is refused
	wantMalformed(t, buildPPTX(t, presXML("rId1", "rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("S1"))}))
	// B4: an AlternateContent with only a Fallback keeps its text; with a
	// Choice the Fallback is still skipped
	only := `<mc:AlternateContent xmlns:mc="` + NSMarkupCompat + `"><mc:Fallback>` + textShape("so-fallback") + `</mc:Fallback></mc:AlternateContent>`
	both := `<mc:AlternateContent xmlns:mc="` + NSMarkupCompat + `"><mc:Choice Requires="p14">` + textShape("escolha") + `</mc:Choice><mc:Fallback>` + textShape("escolha") + `</mc:Fallback></mc:AlternateContent>`
	data = buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", only+both)})
	if got := runPPTX(t, data).Text; got != "[slide 1]\nso-fallback\nescolha" {
		t.Errorf("AlternateContent: %q", got)
	}
	// B5: the deadline is the typed refusal, like DOCX and PDF
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := extractPPTX(ctx, data, DefaultLimits())
	if r, _ := ReasonOf(err); r != ReasonTimeout {
		t.Errorf("deadline: %v, want ReasonTimeout", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := extractPPTX(ctx2, data, DefaultLimits()); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel: %v, want context.Canceled", err)
	}
}

// A2: the deadline cuts a single long page (the WebAssembly call is aborted),
// and the engine keeps working afterwards.
func TestPDFDeadlineCutsALongPage(t *testing.T) {
	content := append([]byte("BT /F1 12 Tf 72 720 Td\n"), bytes.Repeat([]byte("0 0 m 1 1 l S\n"), 1<<20)...)
	content = append(content, "ET\n"...)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(content)
	zw.Close()
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 1 /Kids [4 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents 5 0 R >>",
		fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", z.Len(), z.Bytes()),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
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
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)

	mustPDF(t, pdfASCII(t)) // engine warm, outside the measure
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Extract(ctx, FormatPDF, out.Bytes(), Limits{})
	el := time.Since(start)
	if r, _ := ReasonOf(err); r != ReasonTimeout {
		t.Fatalf("err = %v, want ReasonTimeout", err)
	}
	if el > 3*time.Second {
		t.Fatalf("a pagina rodou %v com timeout de 300 ms", el)
	}
	t.Logf("cortado em %v (antes da correcao: 3,7-5,6 s com timeout de 2 s)", el.Round(time.Millisecond))
	mustPDF(t, pdfThreePages(t)) // the killed instance was replaced
}
