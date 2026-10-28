package main

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
)

// The adapter carries every flag of doctext.Result into files.Extracted (they
// feed the footer: numeracao, conteudo_importado, texto_oculto, [aviso]).
func TestDocExtractorCarriesFlags(t *testing.T) {
	w := `xmlns:w="` + doctext.NSWordprocessing + `" xmlns:r="` + doctext.NSRelationships + `"`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range [][2]string{
		{"[Content_Types].xml", `<Types/>`},
		{"word/document.xml", `<w:document ` + w + `><w:body><w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>x</w:t></w:r></w:p>` +
			`<w:altChunk r:id="r1"/><w:altChunk r:id="r2"/></w:body></w:document>`},
		{"word/_rels/document.xml.rels", `<Relationships xmlns="` + doctext.NSPackageRels + `"><Relationship Id="r1" Type="t" Target="a.txt"/>` +
			`<Relationship Id="r2" Type="t" Target="b.rtf"/></Relationships>`},
		{"word/a.txt", "importado"},
		{"word/b.rtf", `{\rtf1 y}`},
		{"word/numbering.xml", `<w:numbering`},
	} {
		f, _ := zw.Create(e[0])
		f.Write([]byte(e[1]))
	}
	zw.Close()
	e := newDocExtractor(Config{MaxPromptChars: 128000, MaxChunks: 40, Timeout: time.Minute}, nil)
	r, err := e.Extract(context.Background(), "a.docx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !r.ImportedContent || !r.UnreadContent || !r.NumberingUnavailable || !r.HiddenText {
		t.Fatalf("flags: %+v", r)
	}
}
