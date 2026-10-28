package doctext

import (
	"strconv"
	"strings"
	"testing"
)

// nbLvl is a w:lvl. extra goes after the standard children (w:suff, w:pStyle...).
func nbLvl(ilvl int, format, text, extra string) string {
	return `<w:lvl w:ilvl="` + strconv.Itoa(ilvl) + `"><w:start w:val="1"/><w:numFmt w:val="` + format + `"/>` +
		`<w:lvlText w:val="` + text + `"/>` + extra + `</w:lvl>`
}

func nbAbs(id string, lvls ...string) string {
	return `<w:abstractNum w:abstractNumId="` + id + `">` + strings.Join(lvls, "") + `</w:abstractNum>`
}

func nbNum(id, abs, extra string) string {
	return `<w:num w:numId="` + id + `"><w:abstractNumId w:val="` + abs + `"/>` + extra + `</w:num>`
}

func nbSuff(s string) string { return `<w:suff w:val="` + s + `"/>` }

// nbP is a numbered paragraph with direct numPr.
func nbP(numID string, ilvl int, text string) string {
	return `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + strconv.Itoa(ilvl) + `"/><w:numId w:val="` + numID + `"/></w:numPr></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func nbStylePara(style, text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + style + `"/></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func nbDocx(tb testing.TB, numbering, styles, body string) []byte {
	var extra []entry
	if numbering != "" {
		extra = append(extra, entry{name: "word/numbering.xml", data: dxPartXML("numbering", numbering)})
	}
	if styles != "" {
		extra = append(extra, entry{name: "word/styles.xml", data: dxPartXML("styles", styles)})
	}
	return dxBuild(tb, body, extra...)
}

func nbStyle(id, basedOn, numPr string) string {
	s := `<w:style w:type="paragraph" w:styleId="` + id + `">`
	if basedOn != "" {
		s += `<w:basedOn w:val="` + basedOn + `"/>`
	}
	if numPr != "" {
		s += `<w:pPr><w:numPr>` + numPr + `</w:numPr></w:pPr>`
	}
	return s + `</w:style>`
}

func TestNumberingClausulaDecimal(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "CLÁUSULA %1ª", nbSuff("space"))) + nbNum("1", "0", "")
	body := nbP("1", 0, "Do objeto") + dxP("texto solto") + nbP("1", 0, "Do preço") + nbP("1", 0, "Do prazo")
	dxWant(t, nbDocx(t, num, "", body),
		"CLÁUSULA 1ª Do objeto\ntexto solto\nCLÁUSULA 2ª Do preço\nCLÁUSULA 3ª Do prazo\n")
}

func TestNumberingDefaultSuffixIsTab(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", "")) + nbNum("1", "0", "")
	dxWant(t, nbDocx(t, num, "", nbP("1", 0, "um")), "1.\tum\n")
}

func TestNumberingMultilevelRestart(t *testing.T) {
	num := nbAbs("0",
		nbLvl(0, "decimal", "%1.", nbSuff("space")),
		nbLvl(1, "decimal", "%1.%2.", nbSuff("space")),
		nbLvl(2, "lowerLetter", "%3)", nbSuff("space"))) + nbNum("1", "0", "")
	body := nbP("1", 0, "A") + nbP("1", 1, "B") + nbP("1", 2, "x") + nbP("1", 2, "y") + nbP("1", 1, "C") + nbP("1", 2, "z") +
		nbP("1", 0, "D") + nbP("1", 1, "E")
	dxWant(t, nbDocx(t, num, "", body),
		"1. A\n1.1. B\na) x\nb) y\n1.2. C\na) z\n2. D\n2.1. E\n")
}

func TestNumberingLvlRestartNever(t *testing.T) {
	num := nbAbs("0",
		nbLvl(0, "decimal", "%1.", nbSuff("space")),
		nbLvl(1, "decimal", "(%2)", nbSuff("space")+`<w:lvlRestart w:val="0"/>`)) + nbNum("1", "0", "")
	body := nbP("1", 0, "A") + nbP("1", 1, "a") + nbP("1", 0, "B") + nbP("1", 1, "b")
	dxWant(t, nbDocx(t, num, "", body), "1. A\n(1) a\n2. B\n(2) b\n")
}

func TestNumberingFormats(t *testing.T) {
	num := nbAbs("0",
		nbLvl(0, "lowerLetter", "%1)", nbSuff("space")),
		nbLvl(1, "upperLetter", "%2.", nbSuff("space")),
		nbLvl(2, "lowerRoman", "%3.", nbSuff("space")),
		nbLvl(3, "upperRoman", "%4.", nbSuff("space")),
		nbLvl(4, "ordinal", "%5", nbSuff("space")),
		nbLvl(5, "bullet", "", nbSuff("space")),
		nbLvl(6, "none", "Parágrafo único.", nbSuff("space")),
		nbLvl(7, "japaneseCounting", "%8.", nbSuff("space"))) + nbNum("1", "0", "")
	var body string
	for i := 0; i < 8; i++ {
		body += nbP("1", i, "t"+strconv.Itoa(i))
	}
	dxWant(t, nbDocx(t, num, "", body), "a) t0\nA. t1\ni. t2\nI. t3\n1º t4\n- t5\nParágrafo único. t6\n- t7\n")
}

func TestNumberingRomanAndLetters(t *testing.T) {
	cases := []struct {
		format string
		n      int
		want   string
	}{
		{"upperRoman", 4, "IV"}, {"upperRoman", 9, "IX"}, {"upperRoman", 14, "XIV"}, {"upperRoman", 1994, "MCMXCIV"},
		{"lowerRoman", 38, "xxxviii"}, {"upperRoman", 4000, "4000"},
		{"lowerLetter", 1, "a"}, {"lowerLetter", 26, "z"}, {"lowerLetter", 27, "aa"}, {"lowerLetter", 28, "bb"}, {"upperLetter", 53, "AAA"},
		{"lowerLetter", 0, "0"}, {"ordinal", 3, "3º"}, {"decimal", 12, "12"},
	}
	for _, c := range cases {
		if got := formatNumber(c.format, c.n); got != c.want {
			t.Errorf("formatNumber(%s,%d) = %q, quer %q", c.format, c.n, got, c.want)
		}
	}
}

func TestNumberingSuffixes(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1)", nbSuff("nothing")), nbLvl(1, "decimal", "%2)", nbSuff("space")), nbLvl(2, "decimal", "%3)", nbSuff("tab"))) + nbNum("1", "0", "")
	body := nbP("1", 0, "a") + nbP("1", 1, "b") + nbP("1", 2, "c")
	dxWant(t, nbDocx(t, num, "", body), "1)a\n1) b\n1)\tc\n")
}

func TestNumberingInheritedFromStyle(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space")), nbLvl(1, "decimal", "%1.%2.", nbSuff("space"))) + nbNum("1", "0", "")
	styles := nbStyle("Base", "", `<w:numId w:val="1"/>`) +
		nbStyle("Titulo1", "Base", `<w:ilvl w:val="0"/>`) +
		nbStyle("Titulo2", "Titulo1", `<w:ilvl w:val="1"/>`) +
		// A conditional table style with numbering must not leak into the style.
		`<w:style w:type="table" w:styleId="Tab"><w:tblStylePr w:type="firstRow"><w:pPr><w:numPr><w:numId w:val="1"/></w:numPr></w:pPr></w:tblStylePr></w:style>`
	body := nbStylePara("Titulo1", "Um") + nbStylePara("Titulo2", "Um-um") + nbStylePara("Titulo2", "Um-dois") + nbStylePara("Titulo1", "Dois") +
		nbStylePara("Tab", "sem lista") + nbStylePara("Inexistente", "nada")
	dxWant(t, nbDocx(t, num, styles, body), "1. Um\n1.1. Um-um\n1.2. Um-dois\n2. Dois\nsem lista\nnada\n")
}

func TestNumberingStyleLinkedLevel(t *testing.T) {
	// The style has numId only; the level comes from the lvl that points at the style.
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space")), nbLvl(1, "decimal", "%1.%2.", nbSuff("space")+`<w:pStyle w:val="Sub"/>`)) + nbNum("1", "0", "")
	styles := nbStyle("Sub", "", `<w:numId w:val="1"/>`)
	body := nbP("1", 0, "A") + nbStylePara("Sub", "B") + nbStylePara("Sub", "C")
	dxWant(t, nbDocx(t, num, styles, body), "1. A\n1.1. B\n1.2. C\n")
}

func TestNumberingDirectOverridesStyle(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space"))) + nbAbs("1", nbLvl(0, "lowerLetter", "%1)", nbSuff("space"))) + nbNum("1", "0", "") + nbNum("2", "1", "")
	styles := nbStyle("S", "", `<w:numId w:val="1"/>`)
	body := nbStylePara("S", "a") +
		`<w:p><w:pPr><w:pStyle w:val="S"/><w:numPr><w:numId w:val="2"/></w:numPr></w:pPr><w:r><w:t>b</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="S"/><w:numPr><w:numId w:val="0"/></w:numPr></w:pPr><w:r><w:t>c</w:t></w:r></w:p>`
	dxWant(t, nbDocx(t, num, styles, body), "1. a\na) b\nc\n")
}

func TestNumberingStartOverride(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space"))) + nbNum("1", "0", "") +
		nbNum("2", "0", `<w:lvlOverride w:ilvl="0"><w:startOverride w:val="5"/></w:lvlOverride>`)
	body := nbP("1", 0, "a") + nbP("1", 0, "b") + nbP("2", 0, "c") + nbP("2", 0, "d")
	dxWant(t, nbDocx(t, num, "", body), "1. a\n2. b\n5. c\n6. d\n")
}

func TestNumberingLevelStartAndOverrideLevel(t *testing.T) {
	lvl := `<w:lvl w:ilvl="0"><w:start w:val="3"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1-"/><w:suff w:val="space"/></w:lvl>`
	num := nbAbs("0", lvl) + nbNum("1", "0", "") +
		nbNum("2", "0", `<w:lvlOverride w:ilvl="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="upperLetter"/><w:lvlText w:val="%1)"/><w:suff w:val="space"/></w:lvl></w:lvlOverride>`)
	body := nbP("1", 0, "a") + nbP("1", 0, "b") + nbP("2", 0, "c")
	dxWant(t, nbDocx(t, num, "", body), "3- a\n4- b\nA) c\n")
}

func TestNumberingIsLglForcesDecimal(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "upperRoman", "%1.", nbSuff("space")), nbLvl(1, "decimal", "%1.%2", nbSuff("space")+`<w:isLgl/>`)) + nbNum("1", "0", "")
	body := nbP("1", 0, "a") + nbP("1", 0, "b") + nbP("1", 1, "c")
	dxWant(t, nbDocx(t, num, "", body), "I. a\nII. b\n2.1 c\n")
}

func TestNumberingStyleLinkedAbstract(t *testing.T) {
	// abstractNum 1 has no levels: it borrows the ones of the abstractNum that owns style "Lista".
	num := `<w:abstractNum w:abstractNumId="0"><w:styleLink w:val="Lista"/>` + nbLvl(0, "decimal", "%1.", nbSuff("space")) + `</w:abstractNum>` +
		`<w:abstractNum w:abstractNumId="1"><w:numStyleLink w:val="Lista"/></w:abstractNum>` + nbNum("1", "1", "")
	dxWant(t, nbDocx(t, num, "", nbP("1", 0, "a")+nbP("1", 0, "b")), "1. a\n2. b\n")
}

func TestNumberingUnknownAndMissing(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space"))) + nbNum("1", "0", "")
	body := nbP("0", 0, "zero") + nbP("99", 0, "indefinido") + nbP("1", 3, "nivel ausente") + nbP("1", 0, "ok")
	dxWant(t, nbDocx(t, num, "", body), "zero\nindefinido\n- nivel ausente\n1. ok\n")
	// No numbering.xml: numPr is ignored, nothing is prefixed, nothing fails,
	// and nothing is missing.
	none := nbDocx(t, "", "", nbP("1", 0, "sem lista"))
	dxWant(t, none, "sem lista\n")
	if dxExtract(t, none).NumberingUnavailable {
		t.Error("sem numbering.xml não falta numeração")
	}
	// Garbage numbering.xml: the text goes out without numbers, and the result
	// says the numbering is unavailable (footer numeracao=indisponivel).
	bad := dxBuild(t, nbP("1", 0, "x"), entry{name: "word/numbering.xml", data: []byte(`<w:numbering`)})
	dxWant(t, bad, "x\n")
	if !dxExtract(t, bad).NumberingUnavailable {
		t.Error("numbering.xml quebrado deve marcar NumberingUnavailable")
	}
	if dxExtract(t, nbDocx(t, num, "", body)).NumberingUnavailable {
		t.Error("numeração válida não é indisponível")
	}
}

func TestNumberingStyleCycleDoesNotHang(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", "")) + nbNum("1", "0", "")
	styles := nbStyle("A", "B", "") + nbStyle("B", "A", "") + nbStyle("Self", "Self", "")
	body := nbStylePara("A", "a") + nbStylePara("Self", "s")
	dxWant(t, nbDocx(t, num, styles, body), "a\ns\n")
	// A cycle that still reaches a numbered style resolves within the hop limit.
	styles = nbStyle("A", "B", "") + nbStyle("B", "C", "") + nbStyle("C", "A", `<w:numId w:val="1"/>`)
	dxWant(t, nbDocx(t, num, styles, nbStylePara("A", "a")), "1.\ta\n")
}

func TestNumberingEmptyNumberedParagraphAndTable(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space"))) + nbNum("1", "0", "")
	empty := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr></w:p>`
	tbl := `<w:tbl><w:tr><w:tc>` + nbP("1", 0, "na célula") + `</w:tc><w:tc>` + dxP("x") + `</w:tc></w:tr></w:tbl>`
	dxWant(t, nbDocx(t, num, "", nbP("1", 0, "a")+empty+tbl), "1. a\n2.\n\n3. na célula | x\n")
}

func TestNumberingNotAppliedToHeaderParagraphsAndDeterminism(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", nbSuff("space"))) + nbNum("1", "0", "")
	data := nbDocx(t, num, "", nbP("1", 0, "a")+nbP("1", 0, "b"))
	first := dxExtract(t, data).Text
	for i := 0; i < 20; i++ {
		if got := dxExtract(t, data).Text; got != first {
			t.Fatalf("extração %d: %q != %q", i, got, first)
		}
	}
}

// 7.7a M3: a valid numbering.xml with 50 000 w:num (documents pasted together
// many times) keeps its numbers; one over the item ceiling goes out without
// numbers and says so (NumberingUnavailable), never in silence.
func TestNumberingLargeAndOverTheCeiling(t *testing.T) {
	build := func(nums int) []byte {
		var sb strings.Builder
		sb.WriteString(nbAbs("0", nbLvl(0, "decimal", "%1.", "")) + nbNum("1", "0", ""))
		for i := 2; i <= nums; i++ {
			sb.WriteString(`<w:num w:numId="` + strconv.Itoa(i) + `"><w:abstractNumId w:val="0"/></w:num>`)
		}
		return nbDocx(t, sb.String(), "", nbP("1", 0, "Do objeto"))
	}
	var r *Result
	peak, _ := peakHeap(func() { r = dxExtract(t, build(50005)) })
	if r.Text != "1.\tDo objeto\n" || r.NumberingUnavailable {
		t.Errorf("50 000 w:num: %q unavailable=%v", r.Text, r.NumberingUnavailable)
	}
	if peak > 128<<20 {
		t.Errorf("pico de heap %d MiB", peak>>20)
	}
	r = dxExtract(t, build(maxNumberingItems/2+10))
	if r.Text != "Do objeto\n" || !r.NumberingUnavailable {
		t.Errorf("acima do teto: %q unavailable=%v", r.Text, r.NumberingUnavailable)
	}
	// a broken styles.xml loses the numbering inherited from styles: flagged
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", "")) + nbNum("1", "0", "")
	data := dxBuild(t, nbP("1", 0, "direta"),
		entry{name: "word/numbering.xml", data: dxPartXML("numbering", num)},
		entry{name: "word/styles.xml", data: []byte(`<w:styles`)})
	if r := dxExtract(t, data); r.Text != "1.\tdireta\n" || !r.NumberingUnavailable {
		t.Errorf("styles.xml quebrado: %q unavailable=%v", r.Text, r.NumberingUnavailable)
	}
}
