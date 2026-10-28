package doctext

import (
	"bytes"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	zipHead := []byte("PK\x03\x04rest")
	oleHead := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 16)...)
	pdfHead := []byte("%PDF-1.7\n%....")
	junk := func(n int) []byte { return bytes.Repeat([]byte("x"), n) }

	cases := []struct {
		name   string
		file   string
		head   []byte
		want   Format
		reason Reason // 0 = no error
	}{
		{"docx ok", "contrato.docx", zipHead, FormatDOCX, 0},
		{"pptx ok", "deck.pptx", zipHead, FormatPPTX, 0},
		{"pdf ok", "a.pdf", pdfHead, FormatPDF, 0},
		{"pdf junk before header within 1024", "a.pdf", append(junk(1019), pdfHead...), FormatPDF, 0},
		{"pdf header starting at 1019 (ends inside window)", "a.pdf", append(junk(1019), []byte("%PDF-1")...), FormatPDF, 0},
		{"pdf junk past 1024", "a.pdf", append(junk(1020), pdfHead...), FormatText, ReasonMismatch},
		{"docx with pdf content", "a.docx", pdfHead, FormatText, ReasonMismatch},
		{"pdf with zip content", "a.pdf", zipHead, FormatText, ReasonMismatch},
		{"pptx with pdf content", "a.pptx", pdfHead, FormatText, ReasonMismatch},
		{"docx empty", "a.docx", nil, FormatText, ReasonMismatch},
		{"docx OLE is password protected", "a.docx", oleHead, FormatText, ReasonPassworded},
		{"pptx OLE is password protected", "a.pptx", oleHead, FormatText, ReasonPassworded},
		{"pdf with OLE content", "a.pdf", oleHead, FormatText, ReasonMismatch},
		{".doc OLE is legacy", "a.doc", oleHead, FormatText, ReasonLegacy},
		{".ppt OLE is legacy", "a.ppt", oleHead, FormatText, ReasonLegacy},
		{".xls OLE is legacy excel", "a.xls", oleHead, FormatText, ReasonLegacyExcel},
		{".doc not OLE follows the text path", "a.doc", []byte("texto"), FormatText, 0},
		{"upper case extension", "CONTRATO.DOCX", zipHead, FormatDOCX, 0},
		{"mixed case pdf", "Doc.PdF", pdfHead, FormatPDF, 0},
		{"upper case OLE doc", "A.DOC", oleHead, FormatText, ReasonLegacy},
		{"no extension with PK", "arquivo", zipHead, FormatText, 0},
		{"no extension with pdf", "arquivo", pdfHead, FormatText, 0},
		{"txt with pdf bytes", "a.txt", pdfHead, FormatText, 0},
		{"docm out of scope", "a.docm", zipHead, FormatText, 0},
		{"pptm out of scope", "a.pptm", zipHead, FormatText, 0},
		{"dotx out of scope", "a.dotx", zipHead, FormatText, 0},
		{"potx out of scope", "a.potx", zipHead, FormatText, 0},
		{"windows path", `C:\dir\a.docx`, zipHead, FormatDOCX, 0},
		{"dot in directory only", "dir.docx/arquivo", zipHead, FormatText, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Detect(c.file, c.head)
			if c.reason == 0 {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
			} else {
				r, ok := ReasonOf(err)
				if !ok || r != c.reason {
					t.Fatalf("razão = %v (ok=%v), esperado %v; err=%v", r, ok, c.reason, err)
				}
			}
			if got != c.want {
				t.Fatalf("formato = %v, esperado %v", got, c.want)
			}
		})
	}
}

// The exact wording of section 6 is part of the contract.
func TestErrorMessages(t *testing.T) {
	want := map[Reason]string{
		ReasonMismatch:    "o conteúdo não corresponde à extensão do arquivo (.docx/.pptx/.pdf); o arquivo pode estar corrompido ou renomeado",
		ReasonPassworded:  "documento do Office protegido por senha não é suportado; salve uma cópia sem senha",
		ReasonLegacy:      "formato Word/PowerPoint 97-2003 não é suportado; salve como .docx ou .pptx",
		ReasonPDFPassword: "PDF protegido por senha não é suportado; gere uma cópia sem senha",
		ReasonNoText:      "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado",
		ReasonMalformed:   "documento corrompido, malformado ou acima dos limites de segurança",
		ReasonTooLong:     "documento extenso demais: o texto extraído passa do limite; divida o documento",
		ReasonCorruptText: "o texto extraído do PDF está corrompido (fontes sem mapeamento de caracteres); não é possível resumi-lo com segurança",
		ReasonTimeout:     "a extração do texto excedeu o tempo limite (--timeout)",
	}
	for r, msg := range want {
		if got := (&Error{Reason: r}).Error(); got != msg {
			t.Errorf("razão %d: %q", r, got)
		}
	}
	if m := (&Error{Reason: ReasonLegacyExcel}).Error(); !strings.Contains(m, "Excel 97-2003") {
		t.Errorf("mensagem do .xls: %q", m)
	}
}

func TestFormatString(t *testing.T) {
	for f, s := range map[Format]string{FormatText: "txt", FormatDOCX: "docx", FormatPPTX: "pptx", FormatPDF: "pdf"} {
		if f.String() != s {
			t.Errorf("%d -> %q", f, f.String())
		}
	}
}

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.MaxEntries != 2000 || l.MaxEntryBytes != 64<<20 || l.MaxTotalBytes != 256<<20 || l.MaxXMLDepth != 256 || l.MaxPDFPages != 2000 || l.MaxTextRunes <= 0 {
		t.Fatalf("defaults fora do brief: %+v", l)
	}
	// Zero means default, never unlimited.
	if (Limits{}).withDefaults() != l {
		t.Fatal("Limits{} deve virar DefaultLimits")
	}
}
