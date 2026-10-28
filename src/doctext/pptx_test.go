package doctext

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
)

func runPPTX(tb testing.TB, data []byte) *Result {
	tb.Helper()
	r, err := extractPPTX(context.Background(), data, Limits{})
	if err != nil {
		tb.Fatalf("extractPPTX: %v", err)
	}
	return r
}

func wantMalformed(t *testing.T, data []byte) {
	t.Helper()
	_, err := extractPPTX(context.Background(), data, Limits{})
	if r, ok := ReasonOf(err); !ok || r != ReasonMalformed {
		t.Fatalf("err=%v, esperava ReasonMalformed", err)
	}
}

func TestPPTXOrderBySldIdLst(t *testing.T) {
	r := runPPTX(t, samplePPTX(t))
	want := "[slide 1]\nDécimo, mas exibido primeiro\n\n" +
		"[slide 2]\nPrimeiro\n\n[notas do slide 2]\nNota do apresentador\n\n" +
		"[slide 3]\nSegundo"
	if r.Text != want {
		t.Fatalf("texto:\n%q\nesperado:\n%q", r.Text, want)
	}
	if r.Slides != 3 || r.Format != FormatPPTX {
		t.Fatalf("Slides=%d Format=%v", r.Slides, r.Format)
	}
}

func TestPPTXLayoutMasterAndSldNumIgnored(t *testing.T) {
	r := runPPTX(t, samplePPTX(t))
	for _, bad := range []string{"Clique para editar", "mestre"} {
		if strings.Contains(r.Text, bad) {
			t.Fatalf("texto de layout/master apareceu: %q", bad)
		}
	}
	if strings.Contains(r.Text, "\n7") {
		t.Fatalf("sldNum das notas apareceu: %q", r.Text)
	}
}

func TestPPTXNestedGroup(t *testing.T) {
	g := `<p:grpSp><p:nvGrpSpPr/><p:grpSpPr/>` + textShape("A") +
		`<p:grpSp><p:nvGrpSpPr/><p:grpSpPr/>` + textShape("B") + `</p:grpSp>` + textShape("C") + `</p:grpSp>` + textShape("D")
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("Z")+g)})
	if got, want := runPPTX(t, data).Text, "[slide 1]\nZ\nA\nB\nC\nD"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPPTXBreaksAndRuns(t *testing.T) {
	sh := `<p:sp><p:txBody><a:p><a:r><a:t>um</a:t></a:r><a:br/><a:r><a:t>dois</a:t></a:r><a:r><a:t> três</a:t></a:r></a:p><a:p/></p:txBody></p:sp>`
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", sh)})
	if got, want := runPPTX(t, data).Text, "[slide 1]\num\ndois três"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func cell(attrs string, paras ...string) string {
	s := `<a:tc ` + attrs + `><a:txBody><a:bodyPr/>`
	for _, p := range paras {
		s += `<a:p><a:r><a:t>` + p + `</a:t></a:r></a:p>`
	}
	return s + `</a:txBody></a:tc>`
}

func TestPPTXTableWithMerge(t *testing.T) {
	tbl := `<p:graphicFrame><a:graphic><a:graphicData><a:tbl><a:tblGrid/>` +
		`<a:tr>` + cell(`gridSpan="2"`, "Título") + cell(`hMerge="1"`) + `</a:tr>` +
		`<a:tr>` + cell(`rowSpan="2"`, "a", "b") + cell("", "c") + `</a:tr>` +
		`<a:tr>` + cell(`vMerge="1"`) + cell("", "d") + `</a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("Antes")+tbl+textShape("Depois"))})
	want := "[slide 1]\nAntes\n\nTítulo\na b | c\n | d\n\nDepois"
	if got := runPPTX(t, data).Text; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPPTXHiddenSlide(t *testing.T) {
	data := buildPPTX(t, presXML("rId1", "rId2"),
		[]string{slideRel("rId1", "slides/slide1.xml"), slideRel("rId2", "slides/slide2.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("Visível"))},
		entry{name: "ppt/slides/slide2.xml", data: sldXML(`show="0"`, textShape("Escondido"))})
	if got, want := runPPTX(t, data).Text, "[slide 1]\nVisível\n\n[slide 2 — oculto]\nEscondido"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPPTXAlternateContentNoDuplicate(t *testing.T) {
	ac := `<mc:AlternateContent xmlns:mc="` + NSMarkupCompat + `"><mc:Choice Requires="x">` + textShape("Novo") +
		`</mc:Choice><mc:Fallback>` + textShape("Antigo") + `</mc:Fallback></mc:AlternateContent>`
	data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", ac+textShape("Fim"))})
	if got, want := runPPTX(t, data).Text, "[slide 1]\nNovo\nFim"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPPTXNonStandardPrefixAndStrict(t *testing.T) {
	slide := []byte(`<?xml version="1.0"?><x:sld xmlns:x="` + NSPresentationStrict + `" xmlns:y="` + NSDrawingStrict + `"><x:cSld><x:spTree><x:sp><x:txBody>` +
		`<y:p><y:r><y:t>Estrito</y:t></y:r></y:p></x:txBody></x:sp></x:spTree></x:cSld></x:sld>`)
	pres := []byte(`<?xml version="1.0"?><q:presentation xmlns:q="` + NSPresentationStrict + `" xmlns:z="` + NSRelationshipsStrict + `"><q:sldIdLst><q:sldId id="256" z:id="rId1"/></q:sldIdLst></q:presentation>`)
	data := buildPPTX(t, pres, []string{slideRel("rId1", "slides/slide1.xml")}, entry{name: "ppt/slides/slide1.xml", data: slide})
	if got, want := runPPTX(t, data).Text, "[slide 1]\nEstrito"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPPTXMissingPresentation(t *testing.T) {
	wantMalformed(t, buildZip(t, entry{name: "[Content_Types].xml", data: []byte(`<Types/>`)}))
}

func TestPPTXRelOutsidePackage(t *testing.T) {
	wantMalformed(t, buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "../../evil.xml")}))
	// A slide whose own rels point outside the package is refused too.
	wantMalformed(t, buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("x"))},
		entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(notesRel("../../../x.xml"))}))
}

func TestPPTXMissingSlidePart(t *testing.T) {
	wantMalformed(t, buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")}))
}

func TestPPTXTooLong(t *testing.T) {
	_, err := extractPPTX(context.Background(), samplePPTX(t), Limits{MaxTextRunes: 20})
	if r, ok := ReasonOf(err); !ok || r != ReasonTooLong {
		t.Fatalf("err=%v", err)
	}
}

func TestPPTXDeterminism(t *testing.T) {
	data := samplePPTX(t)
	var first [32]byte
	for i := 0; i < 20; i++ {
		h := sha256.Sum256([]byte(runPPTX(t, data).Text))
		if i == 0 {
			first = h
		} else if h != first {
			t.Fatalf("extração %d difere", i)
		}
	}
}

func TestPPTXContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extractPPTX(ctx, samplePPTX(t), Limits{}); err == nil {
		t.Fatal("esperava erro com ctx cancelado")
	}
}

func TestPPTXGarbageNoPanic(t *testing.T) {
	for _, d := range [][]byte{nil, []byte("PK\x03\x04"), []byte("lixo")} {
		if _, err := extractPPTX(context.Background(), d, Limits{}); err == nil {
			t.Fatal("esperava erro")
		}
	}
}
