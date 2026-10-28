package doctext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func advPptx(data []byte) (string, error) {
	r, err := Extract(context.Background(), FormatPPTX, data, Limits{})
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

// ---------------- PPTX ----------------

func TestAdvPPTX(t *testing.T) {
	// 1. sldId duplicado -> mesmo slide duas vezes.
	{
		data := buildPPTX(t, presXML("rId1", "rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("SLIDE-UNICO-TXT"))})
		r, err := Extract(context.Background(), FormatPPTX, data, Limits{})
		if err == nil && count(r.Text, "SLIDE-UNICO-TXT") > 1 {
			vec(t, "pptx", "sldid-duplicado", "SILENCIOSO", fmt.Sprintf("slide repetido %dx como slides distintos; slides=%d (o arquivo so tem 1)", count(r.Text, "SLIDE-UNICO-TXT"), r.Slides))
		} else {
			vec(t, "pptx", "sldid-duplicado", "FIEL", reasonStr(err))
		}
	}
	// 2. rel faltando.
	{
		data := buildPPTX(t, presXML("rId1", "rId2"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("S1"))})
		_, err := advPptx(data)
		vec(t, "pptx", "rel-faltando", map[bool]string{true: "BLOQUEADO", false: "SILENCIOSO"}[err != nil], "recusa ruidosa: "+reasonStr(err))
	}
	// 3. AlternateContent sem Choice (so Fallback) e aninhado em Choice.
	{
		only := `<mc:AlternateContent xmlns:mc="` + NSMarkupCompat + `"><mc:Fallback>` + textShape("PPTX-SO-FALLBACK") + `</mc:Fallback></mc:AlternateContent>`
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", only+textShape("DEPOIS"))})
		txt, err := advPptx(data)
		if err == nil && !strings.Contains(txt, "PPTX-SO-FALLBACK") {
			vec(t, "pptx", "ac-sem-choice", "SILENCIOSO", "texto do Fallback sem Choice sumiu (o DOCX le esse Fallback; o PPTX sempre o pula)")
		} else {
			vec(t, "pptx", "ac-sem-choice", "FIEL", reasonStr(err))
		}
		nested := `<mc:AlternateContent xmlns:mc="` + NSMarkupCompat + `"><mc:Choice Requires="p14"><mc:AlternateContent><mc:Choice Requires="a14">` + textShape("PPTX-AC-INTERNO") +
			`</mc:Choice><mc:Fallback>` + textShape("PPTX-AC-INTERNO") + `</mc:Fallback></mc:AlternateContent></mc:Choice><mc:Fallback>` + textShape("PPTX-AC-INTERNO") + `</mc:Fallback></mc:AlternateContent>`
		data = buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", nested)})
		txt, err = advPptx(data)
		vec(t, "pptx", "ac-aninhado", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[err == nil && count(txt, "PPTX-AC-INTERNO") == 1], fmt.Sprintf("ocorrencias=%d %s", count(txt, "PPTX-AC-INTERNO"), reasonStr(err)))
	}
	// 4. celula hMerge com texto.
	{
		tbl := `<p:graphicFrame><a:graphic><a:graphicData><a:tbl><a:tr><a:tc gridSpan="2">` + `<a:txBody><a:p><a:r><a:t>CEL-SPAN</a:t></a:r></a:p></a:txBody></a:tc>` +
			`<a:tc hMerge="1"><a:txBody><a:p><a:r><a:t>CEL-HMERGE-TEXTO</a:t></a:r></a:p></a:txBody></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")}, entry{name: "ppt/slides/slide1.xml", data: sldXML("", tbl)})
		txt, _ := advPptx(data)
		vec(t, "pptx", "hmerge-com-texto", "INFO", fmt.Sprintf("span=%v hmerge_aparece=%v (celula absorvida descartada, como o PowerPoint exibe)", strings.Contains(txt, "CEL-SPAN"), strings.Contains(txt, "CEL-HMERGE-TEXTO")))
	}
	// 5. notas: alvo com caixa diferente.
	{
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("S1"))},
			entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(notesRel("../notesSlides/NotesSlide1.xml"))},
			entry{name: "ppt/notesSlides/notesSlide1.xml", data: notesXML(phShape("body", "NOTA-CASE-DIFERENTE"))})
		txt, err := advPptx(data)
		if err == nil && !strings.Contains(txt, "NOTA-CASE-DIFERENTE") {
			vec(t, "pptx", "notas-alvo-caixa-diferente", "SILENCIOSO", "notas do apresentador puladas sem sinal (nome de parte OPC e case-insensitive)")
		} else {
			vec(t, "pptx", "notas-alvo-caixa-diferente", "FIEL", reasonStr(err))
		}
		data = buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "Slides/Slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("S1"))})
		_, err = advPptx(data)
		vec(t, "pptx", "slide-alvo-caixa-diferente", "INFO", "recusa ruidosa: "+reasonStr(err))
	}
	// 6. SmartArt / grafico: texto fora do slide.
	{
		gf := `<p:graphicFrame><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/diagram"><dgm:relIds xmlns:dgm="urn:dgm" r:dm="rId5"/></a:graphicData></a:graphic></p:graphicFrame>`
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("TITULO")+gf)},
			entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(`<Relationship Id="rId5" Type="`+relT+`diagramData" Target="../diagrams/data1.xml"/>`)},
			entry{name: "ppt/diagrams/data1.xml", data: []byte(`<dgm:dataModel xmlns:dgm="urn:dgm" xmlns:a="` + nsPA + `"><dgm:ptLst><dgm:pt><dgm:t><a:p><a:r><a:t>TEXTO-SMARTART</a:t></a:r></a:p></dgm:t></dgm:pt></dgm:ptLst></dgm:dataModel>`)})
		txt, _ := advPptx(data)
		vec(t, "pptx", "smartart", "INFO", fmt.Sprintf("smartart_extraido=%v (fora do escopo do brief; perda sem sinal no rodape)", strings.Contains(txt, "TEXTO-SMARTART")))
	}
	// 7. forma oculta.
	{
		hidden := `<p:sp><p:nvSpPr><p:cNvPr id="9" name="h" hidden="1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:p><a:r><a:t>FORMA-OCULTA</a:t></a:r></a:p></p:txBody></p:sp>`
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")}, entry{name: "ppt/slides/slide1.xml", data: sldXML("", hidden)})
		txt, _ := advPptx(data)
		vec(t, "pptx", "forma-oculta", "INFO", fmt.Sprintf("extraida=%v (sem rotulo; slides ocultos sao rotulados, formas ocultas nao)", strings.Contains(txt, "FORMA-OCULTA")))
	}
	// 8. deadline no meio da extracao: erro nao tipado.
	{
		data := buildPPTX(t, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")}, entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("x"))})
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, err := extractPPTX(ctx, data, DefaultLimits())
		_, typed := ReasonOf(err)
		res := "INFO"
		if !typed && errors.Is(err, context.DeadlineExceeded) {
			res = "ACHADO"
		}
		vec(t, "pptx", "deadline-nao-tipado", res, fmt.Sprintf("extractPPTX devolve %v (tipado=%v); DOCX e PDF devolvem ReasonTimeout", err, typed))
	}
	// 9. 100 000 sldId para o mesmo slide: custo de CPU.
	{
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><p:presentation xmlns:a="` + nsPA + `" xmlns:p="` + nsPP + `" xmlns:r="` + nsPR + `"><p:sldIdLst>`)
		for i := 0; i < 100000; i++ {
			sb.WriteString(`<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="rId1"/>`)
		}
		sb.WriteString(`</p:sldIdLst></p:presentation>`)
		data := buildPPTX(t, []byte(sb.String()), []string{slideRel("rId1", "slides/slide1.xml")},
			entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("AMPLIFICADO"))},
			entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(notesRel("../notesSlides/notesSlide1.xml"))},
			entry{name: "ppt/notesSlides/notesSlide1.xml", data: notesXML(phShape("body", "nota"))})
		start := time.Now()
		var r *Result
		var err error
		peak, _ := peakHeap(func() { r, err = Extract(context.Background(), FormatPPTX, data, Limits{}) })
		n := 0
		if r != nil {
			n = r.Slides
		}
		vec(t, "rec", "pptx-100k-sldid-mesmo-slide", "INFO", fmt.Sprintf("pptx=%d B; %v; pico heap=%s; slides=%d; %s (CPU limitada pelo --timeout, checado por slide)", len(data), time.Since(start).Round(time.Millisecond), mib(peak), n, reasonStr(err)))
	}
}

func TestAdvPPTXDeterminism(t *testing.T) {
	data := samplePPTX(t)
	h := map[[32]byte]bool{}
	for i := 0; i < 50; i++ {
		txt, err := advPptx(data)
		if err != nil {
			t.Fatal(err)
		}
		h[sha256.Sum256([]byte(txt))] = true
	}
	vec(t, "det", "pptx-50x", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[len(h) == 1], fmt.Sprintf("%d hash(es) em 50 extracoes", len(h)))
}

// ---------------- ZIP ----------------

func TestAdvZip(t *testing.T) {
	// 1. bomba sobreposta: 12 cabecalhos (nomes distintos) apontam para UMA entrada local de 30 MiB.
	{
		pad := bytes.Repeat([]byte("<!-- "+strings.Repeat("x", 1000)+" -->"), 30<<10)
		hdr := append(append([]byte(`<?xml version="1.0"?><w:hdr xmlns:w="`+NSWordprocessing+`">`), pad...), []byte(`<w:p><w:r><w:t>H</w:t></w:r></w:p></w:hdr>`)...)
		var refs, rels []string
		var shared []string
		for i := 1; i <= 12; i++ {
			refs = append(refs, fmt.Sprintf(`<w:headerReference w:type="default" r:id="rId%d"/>`, i))
			rels = append(rels, fmt.Sprintf(`<Relationship Id="rId%d" Type="t" Target="header%d.xml"/>`, i, i))
			shared = append(shared, fmt.Sprintf("word/header%d.xml", i))
		}
		data := overlapZip(t, []zipPart{
			{"[Content_Types].xml", []byte(`<Types/>`)},
			{"word/document.xml", dxDocXML(dxP("CORPO") + `<w:sectPr>` + strings.Join(refs, "") + `</w:sectPr>`)},
			{"word/_rels/document.xml.rels", relsXML(rels...)},
		}, shared, hdr)
		var err error
		start := time.Now()
		peak, total := peakHeap(func() { _, err = advDocx(data) })
		r, _ := ReasonOf(err)
		vec(t, "zip", "bomba-sobreposta-12x30MiB", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[r == ReasonMalformed],
			fmt.Sprintf("arquivo=%d B, %d MiB por entrada, %s; pico heap=%s; alocado=%s; %v", len(data), len(hdr)>>20, reasonStr(err), mib(peak), mib(total), time.Since(start).Round(time.Millisecond)))
	}
	// 2. muitas entradas pequenas que somam > MaxTotalBytes (limite reduzido para o teste).
	{
		var refs, rels []string
		ents := []entry{}
		for i := 1; i <= 40; i++ {
			refs = append(refs, fmt.Sprintf(`<w:headerReference w:type="default" r:id="rId%d"/>`, i))
			rels = append(rels, fmt.Sprintf(`<Relationship Id="rId%d" Type="t" Target="header%d.xml"/>`, i, i))
			ents = append(ents, entry{name: fmt.Sprintf("word/header%d.xml", i), data: dxPartXML("hdr", "<!--"+strings.Repeat("y", 300<<10)+"-->"+dxP("h"))})
		}
		ents = append(ents, entry{name: "word/_rels/document.xml.rels", data: relsXML(rels...)})
		data := dxBuild(t, dxP("C")+`<w:sectPr>`+strings.Join(refs, "")+`</w:sectPr>`, ents...)
		_, err := Extract(context.Background(), FormatDOCX, data, Limits{MaxTotalBytes: 4 << 20})
		r, _ := ReasonOf(err)
		vec(t, "zip", "muitas-pequenas-somam-acima-total", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[r == ReasonMalformed], fmt.Sprintf("40 x 300 KiB com MaxTotalBytes=4 MiB: %s", reasonStr(err)))
	}
	// 3. entrada declarando 0 bytes descompactados (mas com dados).
	{
		good := buildZip(t, entry{name: "[Content_Types].xml", data: []byte(`<Types/>`)}, entry{name: "word/document.xml", data: dxDocXML(dxP(strings.Repeat("Z", 5000)))})
		bad := append([]byte(nil), good...)
		// zera o tamanho descompactado no diretorio central da 2a entrada
		cds := offsets(bad, []byte("PK\x01\x02"))
		if len(cds) == 2 {
			copy(bad[cds[1]+24:cds[1]+28], []byte{0, 0, 0, 0})
		}
		_, err := advDocx(bad)
		r, _ := ReasonOf(err)
		vec(t, "zip", "entrada-declara-0", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[r == ReasonMalformed], reasonStr(err))
	}
	// 4. zip aninhado: nunca aberto.
	{
		inner := zeros(t, "word/document.xml", 50)
		data := dxBuild(t, dxP("CORPO-COM-ZIP"), entry{name: "word/embeddings/oleObject1.bin", data: inner}, entry{name: "word/embeddings/x.zip", data: inner})
		var err error
		peak, _ := peakHeap(func() { _, err = advDocx(data) })
		vec(t, "zip", "zip-aninhado", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[err == nil && peak < 16<<20], fmt.Sprintf("entradas aninhadas nunca lidas; %s; pico=%s", reasonStr(err), mib(peak)))
	}
	// 5. nomes: caixa, NFKC, duplicatas disfarcadas.
	for _, c := range []struct {
		name  string
		names []string
	}{
		{"dup-caixa", []string{"word/document.xml", "WORD/DOCUMENT.XML"}},
		{"dup-barra-dupla", []string{"word/document.xml", "word//document.xml"}},
		{"dup-ponto", []string{"word/document.xml", "./word/document.xml"}},
		{"dup-ponto-meio", []string{"word/document.xml", "word/./document.xml"}},
		{"fullwidth-nfkc", []string{"word/document.xml", "ｗｏｒｄ/document.xml"}},
		{"barra-final", []string{"word/document.xml", "word/document.xml/"}},
		{"backslash", []string{"word\\document.xml"}},
		{"drive", []string{"C:/word/document.xml"}},
		{"ponto-ponto-meio", []string{"word/../word/document.xml"}},
	} {
		ents := []entry{{name: "[Content_Types].xml", data: []byte(`<Types/>`)}}
		for i, n := range c.names {
			d := dxDocXML(dxP(fmt.Sprintf("VARIANTE-%d", i)))
			if strings.HasSuffix(n, "/") {
				d = nil // marcador de diretorio com o mesmo nome normalizado
			}
			ents = append(ents, entry{name: n, data: d})
		}
		txt, err := advDocx(buildZip(t, ents...))
		res := "BLOQUEADO"
		if err == nil && len(c.names) > 1 && strings.Contains(txt, "VARIANTE-1") {
			res = "ESCAPOU"
		}
		if err == nil && c.name == "fullwidth-nfkc" {
			res = map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[strings.Contains(txt, "VARIANTE-0") && !strings.Contains(txt, "VARIANTE-1")]
		}
		vec(t, "zip", "nome-"+c.name, res, fmt.Sprintf("%s texto=%q", reasonStr(err), strings.TrimSpace(txt)))
	}
	// 6. alvos maliciosos em .rels.
	for _, tg := range []string{"%2e%2e/%2e%2e/evil.xml", "..\\evil.xml", "C:/evil.xml", "C:evil.xml", "file:///C:/evil.xml", `\\server\share\x.xml`,
		"%5c%5cserver%5cx.xml", "%00x.xml", "http://example.invalid/x.xml", "/../evil.xml", "%2F..%2F..%2Fevil.xml", "a/%2e%2e/%2e%2e/%2e%2e/evil.xml", "%252e%252e/evil.xml", "header1.xml%00.png"} {
		data := dxBuild(t, dxP("C")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="t" Target="` + strings.ReplaceAll(tg, `&`, `&amp;`) + `"/>`)},
			entry{name: "evil.xml", data: dxPartXML("hdr", dxP("ALVO-MALICIOSO-LIDO"))},
			entry{name: "word/%2e%2e/evil.xml", data: dxPartXML("hdr", dxP("ALVO-DUPLO-ENCODE"))})
		txt, err := advDocx(data)
		res := "BLOQUEADO"
		if strings.Contains(txt, "ALVO-MALICIOSO-LIDO") {
			res = "ESCAPOU"
		}
		vec(t, "zip", "rels-alvo-"+tg, res, fmt.Sprintf("%s; alvo_duplo_encode_lido=%v (dentro do pacote)", reasonStr(err), strings.Contains(txt, "ALVO-DUPLO-ENCODE")))
	}
	// 7. External para file:// e http:// nunca seguido.
	{
		data := dxBuild(t, dxP("C")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="t" Target="file:///C:/Windows/win.ini" TargetMode="External"/>`)})
		txt, err := advDocx(data)
		vec(t, "zip", "rels-external-file", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[err == nil && !strings.Contains(strings.ToLower(txt), "fonts")], reasonStr(err))
	}
}

// ---------------- XML ----------------

func TestAdvXML(t *testing.T) {
	cases := []struct {
		name string
		doc  []byte
	}{
		{"entidade-sem-dtd", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + NSWordprocessing + `"><w:body><w:p><w:r><w:t>&ent;</w:t></w:r></w:p></w:body></w:document>`)},
		{"doctype-entidade", []byte(`<?xml version="1.0"?><!DOCTYPE d [<!ENTITY a "AAAA"><!ENTITY b "&a;&a;&a;">]><w:document xmlns:w="` + NSWordprocessing + `"><w:body><w:p><w:r><w:t>&b;</w:t></w:r></w:p></w:body></w:document>`)},
		{"doctype-depois-raiz", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + NSWordprocessing + `"><!DOCTYPE x><w:body/></w:document>`)},
		{"char-ref-nul", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + NSWordprocessing + `"><w:body><w:p><w:r><w:t>a&#0;b</w:t></w:r></w:p></w:body></w:document>`)},
		{"utf16-bom", append([]byte{0xFF, 0xFE}, utf16le(`<?xml version="1.0" encoding="UTF-16"?><w:document xmlns:w="`+NSWordprocessing+`"><w:body><w:p><w:r><w:t>U16</w:t></w:r></w:p></w:body></w:document>`)...)},
		{"encoding-latin1", []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><w:document xmlns:w="` + NSWordprocessing + `"><w:body/></w:document>`)},
		{"profundidade-10000", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + NSWordprocessing + `"><w:body>` + strings.Repeat("<w:sdt>", 10000) + strings.Repeat("</w:sdt>", 10000) + `</w:body></w:document>`)},
		{"texto-gigante-20M", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + NSWordprocessing + `"><w:body><w:p><w:r><w:t>` + strings.Repeat("a", 20<<20) + `</w:t></w:r></w:p></w:body></w:document>`)},
	}
	for _, c := range cases {
		data := buildZip(t, entry{name: "[Content_Types].xml", data: []byte(`<Types/>`)}, entry{name: "word/document.xml", data: c.doc})
		txt, err := advDocx(data)
		res := "BLOQUEADO"
		if err == nil {
			res = "INFO"
		}
		if c.name == "doctype-entidade" && strings.Contains(txt, "AAAA") {
			res = "ESCAPOU"
		}
		if c.name == "utf16-bom" {
			res = "INFO"
		}
		vec(t, "xml", c.name, res, fmt.Sprintf("%s len=%d", reasonStr(err), len(txt)))
	}
	// CDATA dentro de w:t: e texto do documento.
	{
		data := dxBuild(t, `<w:p><w:r><w:t><![CDATA[CDATA-TXT <w:t>nao-e-tag</w:t>]]></w:t></w:r></w:p>`)
		txt, err := advDocx(data)
		vec(t, "xml", "cdata", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[err == nil && strings.Contains(txt, "CDATA-TXT <w:t>nao-e-tag</w:t>")], reasonStr(err))
	}
	// Atributo enorme (nao conta no teto de texto).
	{
		data := dxBuild(t, `<w:p><w:pPr><w:pStyle w:val="`+strings.Repeat("s", 30<<20)+`"/></w:pPr><w:r><w:t>ATTR</w:t></w:r></w:p>`)
		var err error
		peak, _ := peakHeap(func() { _, err = advDocx(data) })
		vec(t, "xml", "atributo-30MiB", "INFO", fmt.Sprintf("arquivo=%d B; %s; pico heap=%s (limitado por MaxEntryBytes=64 MiB)", len(data), reasonStr(err), mib(peak)))
	}
	// Comentarios XML enormes: nao contam em MaxTextRunes.
	{
		data := dxBuild(t, `<!--`+strings.Repeat("c", 40<<20)+`-->`+dxP("APOS-COMENTARIO"))
		var err error
		start := time.Now()
		peak, _ := peakHeap(func() { _, err = advDocx(data) })
		vec(t, "xml", "comentario-40MiB", "INFO", fmt.Sprintf("arquivo=%d B; %s; pico heap=%s; %v", len(data), reasonStr(err), mib(peak), time.Since(start).Round(time.Millisecond)))
	}
}

func utf16le(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}
