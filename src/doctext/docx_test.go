package doctext

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"
)

const dxRootAttrs = `xmlns:w="` + NSWordprocessing + `" xmlns:r="` + NSRelationships + `" xmlns:mc="` + NSMarkupCompat +
	`" xmlns:wps="urn:test:wps" xmlns:v="urn:test:v" xmlns:m="` + nsMath + `"`

func dxDocXML(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><w:document ` + dxRootAttrs + `><w:body>` + body + `</w:body></w:document>`)
}

func dxPartXML(root, body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><w:` + root + ` ` + dxRootAttrs + `>` + body + `</w:` + root + `>`)
}

// dxBuild builds a minimal .docx around body; extra entries are added as is.
func dxBuild(tb testing.TB, body string, extra ...entry) []byte {
	entries := []entry{
		{name: "[Content_Types].xml", data: []byte(`<Types/>`)},
		{name: "word/document.xml", data: dxDocXML(body)},
	}
	return buildZip(tb, append(entries, extra...)...)
}

func dxP(text string) string { return `<w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>` }

func dxExtract(tb testing.TB, data []byte) *Result {
	tb.Helper()
	r, err := Extract(context.Background(), FormatDOCX, data, Limits{})
	if err != nil {
		tb.Fatalf("Extract: %v", err)
	}
	return r
}

func dxWant(t *testing.T, data []byte, want string) {
	t.Helper()
	if got := dxExtract(t, data).Text; got != want {
		t.Errorf("texto:\n got %q\nwant %q", got, want)
	}
}

func TestDOCXParagraphsTabsBreaks(t *testing.T) {
	body := `<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr><w:r><w:t>Olá</w:t><w:tab/><w:t>mundo</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>a</w:t><w:br/><w:t>b</w:t><w:cr/><w:t>c</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>antes</w:t><w:br w:type="page"/><w:t>depois</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>co</w:t><w:noBreakHyphen/><w:t>op</w:t><w:softHyphen/><w:t>era</w:t><w:sym w:font="Wingdings" w:char="F0FC"/></w:r></w:p>`
	dxWant(t, dxBuild(t, body), "Olá\tmundo\na\nb\nc\nantes\n[quebra de página]\ndepois\nco-opera\n")
}

func TestDOCXPageBreakAlone(t *testing.T) {
	body := dxP("um") + `<w:p><w:r><w:br w:type="page"/></w:r></w:p>` + dxP("dois")
	dxWant(t, dxBuild(t, body), "um\n[quebra de página]\ndois\n")
}

func TestDOCXTable(t *testing.T) {
	tc := func(props, text string) string {
		paras := ""
		for _, s := range strings.Split(text, "+") {
			if s != "" {
				paras += dxP(s)
			} else {
				paras += `<w:p/>`
			}
		}
		return `<w:tc><w:tcPr>` + props + `</w:tcPr>` + paras + `</w:tc>`
	}
	tbl := `<w:tbl><w:tblPr/><w:tr>` + tc("", "A+B") + tc(`<w:gridSpan w:val="2"/>`, "C") + `</w:tr>` +
		`<w:tr>` + tc(`<w:vMerge w:val="restart"/>`, "D") + tc("", "E") + tc("", "F") + `</w:tr>` +
		`<w:tr>` + tc(`<w:vMerge/>`, "") + tc("", "G") + tc("", "H") + `</w:tr></w:tbl>`
	dxWant(t, dxBuild(t, dxP("antes")+tbl+dxP("depois")),
		"antes\n\nA B | C\nD | E | F\n | G | H\n\ndepois\n")
}

func TestDOCXTableNestedAndBreaksInCell(t *testing.T) {
	inner := `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>x</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>y</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	tbl := `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>L1</w:t><w:br/><w:t>L2</w:t></w:r></w:p>` + inner + `<w:p/></w:tc><w:tc>` + dxP("Z") + `</w:tc></w:tr></w:tbl>`
	dxWant(t, dxBuild(t, tbl), "L1 L2 x | y | Z\n")
}

func TestDOCXRevisions(t *testing.T) {
	body := `<w:p><w:r><w:t>base </w:t></w:r>` +
		`<w:ins w:id="1" w:author="a"><w:r><w:t>novo </w:t></w:r></w:ins>` +
		`<w:del w:id="2" w:author="a"><w:r><w:delText>velho </w:delText><w:t>escondido </w:t></w:r></w:del>` +
		`<w:moveFrom w:id="3"><w:r><w:t>saiu </w:t></w:r></w:moveFrom>` +
		`<w:moveTo w:id="4"><w:r><w:t>entrou</w:t></w:r></w:moveTo></w:p>`
	r := dxExtract(t, dxBuild(t, body))
	if r.Text != "base novo entrou\n" {
		t.Errorf("texto = %q", r.Text)
	}
	if !r.PendingRevisions {
		t.Error("PendingRevisions deveria ser true")
	}
	if r := dxExtract(t, dxBuild(t, dxP("limpo"))); r.PendingRevisions {
		t.Error("PendingRevisions deveria ser false sem revisões")
	}
	// A property change is a pending revision too, and its old numbering is not read.
	pc := `<w:p><w:pPr><w:pPrChange w:id="5"><w:pPr><w:numPr><w:numId w:val="9"/></w:numPr></w:pPr></w:pPrChange></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`
	if r := dxExtract(t, dxBuild(t, pc)); !r.PendingRevisions || r.Text != "x\n" {
		t.Errorf("pPrChange: %+v", r)
	}
}

func TestDOCXAlternateContentOnce(t *testing.T) {
	ac := `<mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wps:txbx><w:txbxContent>` + dxP("Caixa") +
		`</w:txbxContent></wps:txbx></w:drawing></mc:Choice><mc:Fallback><w:pict><v:textbox><w:txbxContent>` + dxP("Caixa") +
		`</w:txbxContent></v:textbox></w:pict></mc:Fallback></mc:AlternateContent>`
	body := `<w:p><w:r><w:t>Antes</w:t></w:r><w:r>` + ac + `</w:r><w:r><w:t>Depois</w:t></w:r></w:p>`
	r := dxExtract(t, dxBuild(t, body))
	if n := strings.Count(r.Text, "Caixa"); n != 1 {
		t.Errorf("Caixa apareceu %d vezes: %q", n, r.Text)
	}
	if r.Text != "Antes\nCaixa\nDepois\n" {
		t.Errorf("texto = %q", r.Text)
	}
	// Fallback alone (no Choice) is still read.
	only := `<w:p><w:r><mc:AlternateContent><mc:Fallback><w:t>fb</w:t></mc:Fallback></mc:AlternateContent></w:r></w:p>`
	dxWant(t, dxBuild(t, only), "fb\n")
}

func TestDOCXFields(t *testing.T) {
	fc := func(typ string) string { return `<w:r><w:fldChar w:fldCharType="` + typ + `"/></w:r>` }
	instr := func(s string) string { return `<w:r><w:instrText xml:space="preserve"> ` + s + ` </w:instrText></w:r>` }
	res := func(s string) string { return `<w:r><w:t>` + s + `</w:t></w:r>` }
	body := `<w:p><w:r><w:t>Página </w:t></w:r>` + fc("begin") + instr("PAGE") + fc("separate") + res("7") + fc("end") +
		res(" link ") + fc("begin") + instr(`HYPERLINK "http://x"`) + fc("separate") +
		`<w:hyperlink r:id="rId9">` + res("clique") + `</w:hyperlink>` + fc("end") +
		`<w:fldSimple w:instr=" DATE "><w:r><w:t> hoje</w:t></w:r></w:fldSimple>` +
		// A field with no result at all (index entry) must not swallow the rest.
		fc("begin") + instr("XE sumido") + fc("end") + res(" fim") + `</w:p>`
	dxWant(t, dxBuild(t, body), "Página 7 link clique hoje fim\n")
	// Nested: the code of the inner field is hidden, the outer result is kept.
	nested := `<w:p>` + fc("begin") + instr("IF") + fc("separate") + res("A") + fc("begin") + instr("PAGEREF x") + fc("separate") + res("3") + fc("end") + res("B") + fc("end") + `</w:p>`
	dxWant(t, dxBuild(t, nested), "A3B\n")
}

func TestDOCXHeadersFootersOnce(t *testing.T) {
	sect := func(h, f string) string {
		return `<w:sectPr><w:headerReference w:type="default" r:id="` + h + `"/><w:footerReference w:type="default" r:id="` + f + `"/></w:sectPr>`
	}
	body := dxP("Seção um") + `<w:p><w:pPr>` + sect("rId10", "rId11") + `</w:pPr></w:p>` + dxP("Seção dois") +
		`<w:sectPr><w:headerReference w:type="default" r:id="rId10"/><w:headerReference w:type="first" r:id="rId12"/><w:footerReference w:type="default" r:id="rId11"/></w:sectPr>`
	rels := relsXML(
		`<Relationship Id="rId10" Type="t" Target="header1.xml"/>`,
		`<Relationship Id="rId11" Type="t" Target="footer1.xml"/>`,
		`<Relationship Id="rId12" Type="t" Target="header2.xml"/>`,
		`<Relationship Id="rId13" Type="t" Target="https://evil.example/h.xml" TargetMode="External"/>`)
	data := dxBuild(t, body,
		entry{name: "word/_rels/document.xml.rels", data: rels},
		entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("Timbre"))},
		entry{name: "word/header2.xml", data: dxPartXML("hdr", `<w:p><w:r><w:drawing/></w:r></w:p>`)},
		entry{name: "word/footer1.xml", data: dxPartXML("ftr", dxP("Pé de página"))})
	r := dxExtract(t, data)
	want := "[cabeçalho]\nTimbre\n\nSeção um\n\nSeção dois\n\n[rodapé]\nPé de página\n"
	if r.Text != want {
		t.Errorf("texto:\n got %q\nwant %q", r.Text, want)
	}
	if strings.Count(r.Text, "Timbre") != 1 {
		t.Error("cabeçalho repetido")
	}
}

func TestDOCXNotesAndComments(t *testing.T) {
	body := `<w:p><w:r><w:t>Veja</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r><w:r><w:t> e</w:t></w:r><w:r><w:endnoteReference w:id="1"/></w:r></w:p>`
	foot := dxPartXML("footnotes",
		`<w:footnote w:type="separator" w:id="-1">`+dxP("")+`</w:footnote>`+
			`<w:footnote w:type="continuationSeparator" w:id="0">`+dxP("SEP")+`</w:footnote>`+
			`<w:footnote w:id="1"><w:p><w:r><w:footnoteRef/></w:r><w:r><w:t xml:space="preserve"> Texto da nota</w:t></w:r></w:p>`+dxP("segundo parágrafo")+`</w:footnote>`)
	end := dxPartXML("endnotes", `<w:endnote w:id="1">`+dxP("Fim da obra")+`</w:endnote>`)
	com := dxPartXML("comments",
		`<w:comment w:id="0" w:author="Maria Silva" w:date="2026-01-02T03:04:05Z" w:initials="MS">`+dxP("Revisar esta cláusula")+`</w:comment>`+
			`<w:comment w:id="1" w:date="2026-01-02T03:04:05Z">`+dxP("sem autor")+`</w:comment>`)
	r := dxExtract(t, dxBuild(t, body,
		entry{name: "word/footnotes.xml", data: foot},
		entry{name: "word/endnotes.xml", data: end},
		entry{name: "word/comments.xml", data: com}))
	want := "Veja[1] e[fim 1]\n\n" +
		"[nota de rodapé 1] Texto da nota segundo parágrafo\n[nota de fim 1] Fim da obra\n[comentário] Maria Silva: Revisar esta cláusula\n[comentário] sem autor\n"
	if r.Text != want {
		t.Errorf("texto:\n got %q\nwant %q", r.Text, want)
	}
	if strings.Contains(r.Text, "SEP") || strings.Contains(r.Text, "2026") {
		t.Error("separador ou data vazou")
	}
}

func TestDOCXContentControlsTextBoxesMath(t *testing.T) {
	body := `<w:sdt><w:sdtPr><w:alias w:val="Campo"/></w:sdtPr><w:sdtContent>` + dxP("dentro do controle") + `</w:sdtContent></w:sdt>` +
		`<w:p><w:r><w:pict><v:shape><v:textbox><w:txbxContent>` + dxP("caixa vml") + `</w:txbxContent></v:textbox></v:shape></w:pict></w:r></w:p>` +
		`<w:p><w:hyperlink w:anchor="a"><w:r><w:t>âncora</w:t></w:r></w:hyperlink><m:oMath><m:r><m:t>x2</m:t></m:r></m:oMath></w:p>`
	dxWant(t, dxBuild(t, body), "dentro do controle\ncaixa vml\nâncorax2\n")
}

func TestDOCXStrictOOXML(t *testing.T) {
	doc := []byte(`<w:document xmlns:w="` + NSWordprocessingStrict + `"><w:body><w:p><w:r><w:t>estrito</w:t></w:r></w:p></w:body></w:document>`)
	data := buildZip(t, entry{name: "word/document.xml", data: doc})
	dxWant(t, data, "estrito\n")
}

func TestDOCXNonStandardPrefix(t *testing.T) {
	doc := []byte(`<q:document xmlns:q="` + NSWordprocessing + `"><q:body><q:p><q:r><q:t>prefixo</q:t></q:r></q:p>` +
		`<q:p><q:r><q:t>x</q:t></q:r><q:ins><q:r><q:t>y</q:t></q:r></q:ins><q:del><q:r><q:delText>z</q:delText></q:r></q:del></q:p></q:body></q:document>`)
	r := dxExtract(t, buildZip(t, entry{name: "word/document.xml", data: doc}))
	if r.Text != "prefixo\nxy\n" || !r.PendingRevisions {
		t.Errorf("r = %+v", r)
	}
}

func TestDOCXIgnoredParts(t *testing.T) {
	data := dxBuild(t, dxP("corpo"),
		entry{name: "docProps/core.xml", data: []byte(`<cp:coreProperties>AUTOR-SECRETO</cp:coreProperties>`)},
		entry{name: "customXml/item1.xml", data: []byte(`<x>CUSTOM-SECRETO</x>`)},
		entry{name: "word/glossary/document.xml", data: dxDocXML(dxP("GLOSSARIO"))},
		entry{name: "word/vbaProject.bin", data: []byte("MACRO")},
		entry{name: "word/media/image1.png", data: []byte("PNG")})
	dxWant(t, data, "corpo\n")
}

func TestDOCXBlankLinesCollapsed(t *testing.T) {
	body := dxP("a") + strings.Repeat(`<w:p/>`, 7) + dxP("b") + `<w:p/>` + dxP("c") + strings.Repeat(`<w:p/>`, 3)
	dxWant(t, dxBuild(t, body), "a\n\n\nb\n\nc\n")
}

func TestDOCXMissingDocumentXML(t *testing.T) {
	data := buildZip(t, entry{name: "word/styles.xml", data: []byte(`<s/>`)})
	_, err := Extract(context.Background(), FormatDOCX, data, Limits{})
	wantReason(t, err, ReasonMalformed)
}

func TestDOCXMalformedXMLAndDTD(t *testing.T) {
	bad := buildZip(t, entry{name: "word/document.xml", data: []byte(`<w:document xmlns:w="` + nsW + `"><w:body><w:p>`)})
	_, err := Extract(context.Background(), FormatDOCX, bad, Limits{})
	wantReason(t, err, ReasonMalformed)
	dtd := buildZip(t, entry{name: "word/document.xml", data: []byte(`<!DOCTYPE a [<!ENTITY x "y">]><w:document xmlns:w="` + nsW + `"/>`)})
	_, err = Extract(context.Background(), FormatDOCX, dtd, Limits{})
	wantReason(t, err, ReasonMalformed)
}

func TestDOCXTextCeiling(t *testing.T) {
	data := dxBuild(t, strings.Repeat(dxP("0123456789"), 50))
	_, err := Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: 200})
	wantReason(t, err, ReasonTooLong)
	// The ceiling sums every part: body and a footnote together pass it.
	foot := dxPartXML("footnotes", `<w:footnote w:id="1">`+dxP(strings.Repeat("n", 150))+`</w:footnote>`)
	data = dxBuild(t, dxP(strings.Repeat("c", 150)), entry{name: "word/footnotes.xml", data: foot})
	_, err = Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: 300})
	wantReason(t, err, ReasonTooLong)
}

func TestExtractContext(t *testing.T) {
	data := dxBuild(t, dxP("x"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, FormatDOCX, data, Limits{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelado: err = %v", err)
	}
	ctx2, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel2()
	_, err := Extract(ctx2, FormatDOCX, data, Limits{})
	wantReason(t, err, ReasonTimeout)
}

func TestExtractRecoversPanic(t *testing.T) {
	testHookExtract = func(Format) { panic("boom") }
	defer func() { testHookExtract = nil }()
	res, err := Extract(context.Background(), FormatDOCX, dxBuild(t, dxP("x")), Limits{})
	if res != nil {
		t.Error("resultado deveria ser nil")
	}
	wantReason(t, err, ReasonMalformed)
}

func TestExtractFormatText(t *testing.T) {
	_, err := Extract(context.Background(), FormatText, []byte("x"), Limits{})
	if err == nil {
		t.Fatal("FormatText deveria ser erro de uso")
	}
	if _, typed := ReasonOf(err); typed {
		t.Error("erro de uso não deveria ser uma recusa tipada")
	}
}

func TestExtractNormalizesAndSetsFormat(t *testing.T) {
	r := dxExtract(t, dxBuild(t, dxP("a")+dxP("b")))
	if r.Format != FormatDOCX {
		t.Errorf("Format = %v", r.Format)
	}
}

func TestDOCXDeterminism(t *testing.T) {
	tc := func(s string) string { return `<w:tc>` + dxP(s) + `</w:tc>` }
	body := dxP("um") + `<w:tbl><w:tr>` + tc("a") + tc("b") + `</w:tr></w:tbl>` + dxP("dois")
	foot := dxPartXML("footnotes", `<w:footnote w:id="2">`+dxP("n2")+`</w:footnote><w:footnote w:id="1">`+dxP("n1")+`</w:footnote>`)
	data := dxBuild(t, body, entry{name: "word/footnotes.xml", data: foot})
	var first [32]byte
	for i := 0; i < 20; i++ {
		sum := sha256.Sum256([]byte(dxExtract(t, data).Text))
		if i == 0 {
			first = sum
		} else if sum != first {
			t.Fatalf("extração %d difere da primeira", i)
		}
	}
}
