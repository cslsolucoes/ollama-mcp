package tools

// The footer flags added after the 7.7a review (owner decisions): they appear
// only when they apply, and an unread part adds an [aviso] line right after
// the footer.

import (
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
)

const flagsW = `xmlns:w="` + doctext.NSWordprocessing + `" xmlns:r="` + doctext.NSRelationships + `"`

func flagsDocx(t *testing.T, body string, extra map[string]string) []byte {
	parts := map[string]string{
		"[Content_Types].xml": `<Types/>`,
		"word/document.xml":   `<w:document ` + flagsW + `><w:body>` + body + `</w:body></w:document>`,
	}
	order := []string{"[Content_Types].xml", "word/document.xml"}
	for n, d := range extra {
		parts[n] = d
		order = append(order, n)
	}
	return zipOf(t, parts, order...)
}

func flagsRels(rels string) string {
	return `<Relationships xmlns="` + doctext.NSPackageRels + `">` + rels + `</Relationships>`
}

func TestFooterFlags(t *testing.T) {
	e := newFileEnv(t)
	// every flag at once: imported HTML, hidden text, broken numbering
	e.putBytes(t, "todos.docx", flagsDocx(t,
		wp("visivel")+`<w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>escondido</w:t></w:r></w:p><w:altChunk r:id="r1"/>`,
		map[string]string{
			"word/_rels/document.xml.rels": flagsRels(`<Relationship Id="r1" Type="t" Target="a.htm"/>`),
			"word/a.htm":                   `<p>importado</p>`,
			"word/numbering.xml":           `<w:numbering`,
		}))
	// an unsupported chunk
	e.putBytes(t, "rtf.docx", flagsDocx(t, wp("texto")+`<w:altChunk r:id="r1"/>`,
		map[string]string{
			"word/_rels/document.xml.rels": flagsRels(`<Relationship Id="r1" Type="t" Target="a.rtf"/>`),
			"word/a.rtf":                   `{\rtf1 x}`,
		}))
	// nothing special
	e.putBytes(t, "limpo.docx", docx(t, wp("texto limpo")))

	d := e.docDeps(t, newMock(t, generateOK), 128000, 40)
	for name, want := range map[string]struct {
		suffix string
		aviso  bool
	}{
		"todos.docx": {" formato=docx revisoes_pendentes=nao numeracao=indisponivel conteudo_importado=sim texto_oculto=sim", false},
		"rtf.docx":   {" formato=docx revisoes_pendentes=nao", true},
		"limpo.docx": {" formato=docx revisoes_pendentes=nao", false},
	} {
		res, err := call(t, d, "summarize_file", `{"path":"`+name+`"}`)
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", name, err, res)
		}
		out := text(t, res)
		fl := fileLine(out)
		if !strings.HasSuffix(fl, want.suffix) {
			t.Errorf("%s: rodapé %q, queria o sufixo %q", name, fl, want.suffix)
		}
		hasAviso := strings.Contains(out, fl+"\n[aviso] parte do conteúdo não pôde ser lida")
		if hasAviso != want.aviso {
			t.Errorf("%s: aviso=%v, queria %v\n%s", name, hasAviso, want.aviso, out)
		}
	}
}

// docFooter: the flags never show for a text file, and for a PPTX only
// texto_oculto applies.
func TestDocFooterFlagsOnlyWhenTheyApply(t *testing.T) {
	if got := docFooter(&files.File{Format: "txt"}); got != " formato=txt" {
		t.Errorf("txt: %q", got)
	}
	if got := docFooter(&files.File{Format: "pptx", Slides: 3, HiddenText: true}); got != " formato=pptx slides=3 texto_oculto=sim" {
		t.Errorf("pptx: %q", got)
	}
	if got := docFooter(&files.File{Format: "pdf", Pages: 2}); got != " formato=pdf paginas=2 paginas_sem_texto=0" {
		t.Errorf("pdf: %q", got)
	}
}
