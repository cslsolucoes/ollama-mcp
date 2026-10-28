package doctext

// w:altChunk (7.7a M2, owner decision): TXT, HTML and DOCX (depth 1) are
// extracted in place and flagged ImportedContent; anything else is flagged
// UnreadContent. Nothing imported is lost in silence.

import (
	"context"
	"strings"
	"testing"
)

// chunkDocx builds a .docx whose body is body and whose rels map rId to the
// given targets; parts adds the chunk parts themselves.
func chunkDocx(t *testing.T, body string, rels map[string]string, parts ...entry) []byte {
	t.Helper()
	var rs []string
	for id, target := range rels {
		rs = append(rs, `<Relationship Id="`+id+`" Type="`+relT+`aFChunk" Target="`+target+`"/>`)
	}
	extra := append([]entry{{name: "word/_rels/document.xml.rels", data: relsXML(rs...)}}, parts...)
	return dxBuild(t, body, extra...)
}

func alt(id string) string { return `<w:altChunk r:id="` + id + `"/>` }

func TestAltChunkHTML(t *testing.T) {
	page := `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0//EN" "x.dtd"><html><head><title>TITULO-NAO</title>` +
		`<style>p{color:red} .x:before{content:"CSS-NAO"}</style><script>var s = "<p>SCRIPT-NAO</p>";</script></head>` +
		`<body><!-- COMENTARIO-NAO --><h1>CLÁUSULA 9ª &ndash; Do foro</h1><p>Fica eleito o foro de <b>São&nbsp;Paulo</b> &amp; região.</p>` +
		`<ul><li>item um</li><li>item dois</li></ul><table><tr><th>Parte</th><th>Valor</th></tr><tr><td>A</td><td>R$ 10</td></tr></table>` +
		`<p>a &lt; b</p></body></html>`
	data := chunkDocx(t, dxP("ANTES")+alt("rIdA")+dxP("DEPOIS"), map[string]string{"rIdA": "afchunk.htm"},
		entry{name: "word/afchunk.htm", data: []byte(page)})
	r := dxExtract(t, data)
	want := "ANTES\nCLÁUSULA 9ª – Do foro\nFica eleito o foro de São Paulo & região.\n- item um\n- item dois\nParte | Valor\nA | R$ 10\na < b\nDEPOIS\n"
	if r.Text != want {
		t.Errorf("texto:\n got %q\nwant %q", r.Text, want)
	}
	if !r.ImportedContent || r.UnreadContent {
		t.Errorf("flags: imported=%v unread=%v", r.ImportedContent, r.UnreadContent)
	}
	for _, no := range []string{"TITULO-NAO", "CSS-NAO", "SCRIPT-NAO", "COMENTARIO-NAO", "x.dtd"} {
		if strings.Contains(r.Text, no) {
			t.Errorf("%s vazou para o texto", no)
		}
	}
}

func TestAltChunkTXTAndInCell(t *testing.T) {
	data := chunkDocx(t, `<w:tbl><w:tr><w:tc>`+alt("rIdT")+`</w:tc><w:tc>`+dxP("B")+`</w:tc></w:tr></w:tbl>`,
		map[string]string{"rIdT": "chunk.txt"},
		entry{name: "word/chunk.txt", data: []byte("\xEF\xBB\xBFlinha um\r\nlinha dois")})
	r := dxExtract(t, data)
	if !strings.Contains(r.Text, "linha um linha dois | B") || !r.ImportedContent {
		t.Errorf("texto %q imported=%v", r.Text, r.ImportedContent)
	}
}

func TestAltChunkImportedDOCX(t *testing.T) {
	inner := dxBuild(t, dxP("CORPO-IMPORTADO")+`<w:p><w:ins w:id="1" w:author="a"><w:r><w:t>inserido</w:t></w:r></w:ins></w:p>`+
		`<w:p><w:r><w:t>com nota</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`+
		`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
		entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="header1.xml"/>`)},
		entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("CABECALHO-IMPORTADO-NAO"))},
		entry{name: "word/footnotes.xml", data: dxPartXML("footnotes", `<w:footnote w:id="1">`+dxP("NOTA-IMPORTADA")+`</w:footnote>`)})
	data := chunkDocx(t, dxP("ANTES")+alt("rIdD")+dxP("DEPOIS"), map[string]string{"rIdD": "chunk.docx"},
		entry{name: "word/chunk.docx", data: inner, method: store})
	r := dxExtract(t, data)
	for _, w := range []string{"ANTES\nCORPO-IMPORTADO\ninserido\ncom nota[1]", "NOTA-IMPORTADA", "DEPOIS"} {
		if !strings.Contains(r.Text, w) {
			t.Errorf("falta %q em %q", w, r.Text)
		}
	}
	if strings.Contains(r.Text, "CABECALHO-IMPORTADO-NAO") {
		t.Error("cabeçalho do DOCX importado não entra (o Word importa o conteúdo)")
	}
	if !r.ImportedContent || r.UnreadContent || !r.PendingRevisions {
		t.Errorf("flags: imported=%v unread=%v pending=%v", r.ImportedContent, r.UnreadContent, r.PendingRevisions)
	}
	if strings.Index(r.Text, "CORPO-IMPORTADO") > strings.Index(r.Text, "DEPOIS") {
		t.Error("o conteúdo importado saiu fora do lugar")
	}
}

func TestAltChunkUnreadable(t *testing.T) {
	innerInner := dxBuild(t, dxP("NIVEL-DOIS"))
	level1 := chunkDocx(t, dxP("NIVEL-UM")+alt("rIdX"), map[string]string{"rIdX": "deep.docx"},
		entry{name: "word/deep.docx", data: innerInner, method: store})
	cases := map[string]struct {
		body  string
		rels  map[string]string
		parts []entry
	}{
		"mht":                                  {alt("r1"), map[string]string{"r1": "a.mht"}, []entry{{name: "word/a.mht", data: []byte("MIME-Version: 1.0")}}},
		"rtf":                                  {alt("r1"), map[string]string{"r1": "a.rtf"}, []entry{{name: "word/a.rtf", data: []byte(`{\rtf1 TEXTO-RTF}`)}}},
		"html em latin1":                       {alt("r1"), map[string]string{"r1": "a.htm"}, []entry{{name: "word/a.htm", data: []byte("<p>S\xe3o</p>")}}},
		"alvo ausente":                         {alt("r1"), map[string]string{"r1": "nao-existe.htm"}, nil},
		"rel ausente":                          {alt("r9"), map[string]string{}, nil},
		"sem r:id":                             {`<w:altChunk/>`, map[string]string{}, nil},
		"docx quebrado":                        {alt("r1"), map[string]string{"r1": "a.docx"}, []entry{{name: "word/a.docx", data: []byte("PK\x03\x04 lixo")}}},
		"docx dentro de docx (profundidade 2)": {alt("r1"), map[string]string{"r1": "a.docx"}, []entry{{name: "word/a.docx", data: level1, method: store}}},
	}
	for name, c := range cases {
		r := dxExtract(t, chunkDocx(t, dxP("VISIVEL")+c.body+dxP("FIM"), c.rels, c.parts...))
		if !r.UnreadContent || !strings.Contains(r.Text, "VISIVEL\n") || !strings.Contains(r.Text, "FIM\n") {
			t.Errorf("%s: unread=%v texto=%q", name, r.UnreadContent, r.Text)
		}
		if strings.Contains(r.Text, "TEXTO-RTF") || strings.Contains(r.Text, "NIVEL-DOIS") {
			t.Errorf("%s: conteúdo não suportado entrou no texto: %q", name, r.Text)
		}
	}
	// depth 2: level 1 is read, its own chunk is not
	r := dxExtract(t, chunkDocx(t, alt("r1"), map[string]string{"r1": "a.docx"}, entry{name: "word/a.docx", data: level1, method: store}))
	if !strings.Contains(r.Text, "NIVEL-UM") || !r.ImportedContent || !r.UnreadContent {
		t.Errorf("profundidade: %q imported=%v unread=%v", r.Text, r.ImportedContent, r.UnreadContent)
	}
}

// The imported content shares the ceilings of the document: its text counts
// against MaxTextRunes and its bytes against MaxTotalBytes.
func TestAltChunkSharesTheLimits(t *testing.T) {
	big := strings.Repeat("palavra ", 2000)
	data := chunkDocx(t, dxP("x")+alt("r1"), map[string]string{"r1": "a.txt"}, entry{name: "word/a.txt", data: []byte(big)})
	_, err := Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: 5000})
	if r, _ := ReasonOf(err); r != ReasonTooLong {
		t.Errorf("txt acima do teto: %v", err)
	}
	inner := dxBuild(t, dxP(big))
	data = chunkDocx(t, dxP("x")+alt("r1"), map[string]string{"r1": "a.docx"}, entry{name: "word/a.docx", data: inner, method: store})
	_, err = Extract(context.Background(), FormatDOCX, data, Limits{MaxTextRunes: 5000})
	if r, _ := ReasonOf(err); r != ReasonTooLong {
		t.Errorf("docx importado acima do teto: %v", err)
	}
	// a nested bomb: 30 MiB inside, budget of 4 MiB for the whole document
	bomb := zeros(t, "word/document.xml", 30)
	data = chunkDocx(t, dxP("x")+alt("r1")+dxP("y"), map[string]string{"r1": "a.docx"}, entry{name: "word/a.docx", data: bomb, method: store})
	var r *Result
	peak, _ := peakHeap(func() { r, err = Extract(context.Background(), FormatDOCX, data, Limits{MaxTotalBytes: 4 << 20}) })
	if err != nil || !r.UnreadContent || peak > 16<<20 {
		t.Errorf("bomba aninhada: err=%v unread=%v pico=%d MiB", err, r != nil && r.UnreadContent, peak>>20)
	}
}

func TestHTMLTextNeverLongerThanInput(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("<td>", 10000), strings.Repeat("<li>", 10000), strings.Repeat("<br>", 10000),
		strings.Repeat("&amp;", 10000), strings.Repeat("<p>a</p>", 10000), "<", "<<<>>>", "<!--", "<script>", "<a href='>'>x</a>",
	} {
		out, ok := htmlText([]byte(in))
		if !ok || len(out) > len(in) {
			t.Errorf("htmlText(%.20q...): ok=%v %d > %d", in, ok, len(out), len(in))
		}
	}
}
