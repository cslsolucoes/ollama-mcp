package doctext

// Hidden text (owner decision after 7.7a): it is extracted on lines labelled
// "[oculto]" and the result says HiddenText, so the reader knows the passage
// is not in the printed document.

import (
	"strings"
	"testing"
)

func hid(text string) string {
	return `<w:r><w:rPr><w:vanish/></w:rPr><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}

func vis(text string) string { return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>` }

func TestDOCXHiddenTextDirect(t *testing.T) {
	r := dxExtract(t, dxBuild(t, `<w:p>`+vis("Valor de ")+hid("R$ 1,00 (nota interna)")+vis(" mil reais.")+`</w:p>`+dxP("Fim")))
	want := "Valor de\n[oculto] R$ 1,00 (nota interna)\nmil reais.\nFim\n"
	if r.Text != want || !r.HiddenText {
		t.Errorf("texto %q hidden=%v\nwant %q", r.Text, r.HiddenText, want)
	}
	// w:vanish w:val="0" is visible; no hidden text, no flag
	r = dxExtract(t, dxBuild(t, `<w:p><w:r><w:rPr><w:vanish w:val="0"/></w:rPr><w:t>visivel</w:t></w:r></w:p>`))
	if r.Text != "visivel\n" || r.HiddenText {
		t.Errorf("vanish=0: %q hidden=%v", r.Text, r.HiddenText)
	}
	// a paragraph mark that is hidden does not hide the text of its runs
	r = dxExtract(t, dxBuild(t, `<w:p><w:pPr><w:rPr><w:vanish/></w:rPr></w:pPr>`+vis("marca oculta, texto não")+`</w:p>`))
	if r.Text != "marca oculta, texto não\n" || r.HiddenText {
		t.Errorf("marca de parágrafo: %q hidden=%v", r.Text, r.HiddenText)
	}
}

func TestDOCXHiddenTextFromStyles(t *testing.T) {
	styles := `<w:docDefaults><w:rPrDefault><w:rPr/></w:rPrDefault></w:docDefaults>` +
		`<w:style w:type="character" w:styleId="Oculto"><w:rPr><w:vanish/></w:rPr></w:style>` +
		`<w:style w:type="character" w:styleId="Herdado"><w:basedOn w:val="Oculto"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="ParOculto"><w:rPr><w:vanish/></w:rPr></w:style>` +
		`<w:style w:type="table" w:styleId="Tab"><w:tblStylePr w:type="firstRow"><w:rPr><w:vanish/></w:rPr></w:tblStylePr></w:style>`
	body := `<w:p><w:r><w:rPr><w:rStyle w:val="Oculto"/></w:rPr><w:t>por estilo</w:t></w:r></w:p>` +
		`<w:p><w:r><w:rPr><w:rStyle w:val="Herdado"/></w:rPr><w:t>por basedOn</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="ParOculto"/></w:pPr>` + vis("paragrafo inteiro") + `</w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="ParOculto"/></w:pPr><w:r><w:rPr><w:vanish w:val="false"/></w:rPr><w:t>direto vence</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Tab"/></w:pPr>` + vis("estilo de tabela nao conta") + `</w:p>`
	r := dxExtract(t, nbDocx(t, "", styles, body))
	want := "[oculto] por estilo\n[oculto] por basedOn\n[oculto] paragrafo inteiro\ndireto vence\nestilo de tabela nao conta\n"
	if r.Text != want || !r.HiddenText {
		t.Errorf("texto %q hidden=%v\nwant %q", r.Text, r.HiddenText, want)
	}
	// document defaults
	def := `<w:docDefaults><w:rPrDefault><w:rPr><w:vanish/></w:rPr></w:rPrDefault></w:docDefaults>`
	if got := dxExtract(t, nbDocx(t, "", def, dxP("tudo oculto"))).Text; got != "[oculto] tudo oculto\n" {
		t.Errorf("docDefaults: %q", got)
	}
}

func TestDOCXHiddenNumberedParagraphAndCell(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", "")) + nbNum("1", "0", "")
	body := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>` + hid("cláusula retirada") + `</w:p>` +
		nbP("1", 0, "cláusula que vale") +
		`<w:tbl><w:tr><w:tc><w:p>` + vis("a") + hid("b") + `</w:p></w:tc><w:tc>` + dxP("c") + `</w:tc></w:tr></w:tbl>`
	r := dxExtract(t, nbDocx(t, num, "", body))
	for _, w := range []string{"[oculto] 1.\tcláusula retirada\n2.\tcláusula que vale\n", "a [oculto] b | c"} {
		if !strings.Contains(r.Text, w) {
			t.Errorf("falta %q em %q", w, r.Text)
		}
	}
}

func TestPPTXHiddenShapesAndSlides(t *testing.T) {
	hiddenShape := `<p:sp><p:nvSpPr><p:cNvPr id="5" name="h" hidden="1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/>` +
		`<a:p><a:r><a:t>forma oculta</a:t></a:r></a:p></p:txBody></p:sp>`
	hiddenTable := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="6" name="t" hidden="1"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<a:graphic><a:graphicData><a:tbl><a:tr><a:tc><a:txBody><a:p><a:r><a:t>x</a:t></a:r></a:p></a:txBody></a:tc>` +
		`<a:tc><a:txBody><a:p><a:r><a:t>y</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("antes")+hiddenShape+textShape("depois")+hiddenTable)})
	r := runPPTX(t, data)
	want := "[slide 1]\nantes\n[oculto] forma oculta\ndepois\n\n[oculto] x | y"
	if r.Text != want || !r.HiddenText {
		t.Errorf("texto %q hidden=%v\nwant %q", r.Text, r.HiddenText, want)
	}
	// a hidden slide flags it too; a plain presentation does not
	data = buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML(`show="0"`, textShape("s"))})
	if !runPPTX(t, data).HiddenText {
		t.Error("slide oculto deve marcar HiddenText")
	}
	if runPPTX(t, samplePPTX(t)).HiddenText {
		t.Error("apresentação sem nada oculto marcou HiddenText")
	}
}
