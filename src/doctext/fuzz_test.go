package doctext

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzOpenPackage(f *testing.F) {
	f.Add(minimalDOCX(f))
	f.Add(buildZip(f, entry{name: "ppt/presentation.xml", data: []byte("<p/>"), method: store}))
	f.Add(buildZip(f, entry{name: "../x", data: []byte("y")}))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{})
	lim := Limits{MaxEntries: 50, MaxEntryBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxXMLDepth: 32, MaxTextRunes: 1 << 16}
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := OpenPackage(data, lim)
		if err != nil {
			return
		}
		for _, n := range p.Names() {
			p.Has(n)
			_, _ = p.Read(n)
			_, _ = p.Relationships(n)
		}
	})
}

func FuzzXMLSafe(f *testing.F) {
	f.Add(docXML(`<w:p><w:r><w:t>Olá</w:t></w:r></w:p>`))
	f.Add(relsXML(`<Relationship Id="a" Type="t" Target="x.xml"/>`))
	f.Add([]byte(`<!DOCTYPE a [<!ENTITY x "y">]><a>&x;</a>`))
	f.Add([]byte(strings.Repeat("<a>", 100)))
	f.Add([]byte(`<?xml version="1.0" encoding="UTF-16"?><a/>`))
	f.Add([]byte(`<a xmlns:w="` + NSWordprocessingStrict + `" w:val="1">&amp;&#x41;</a>`))
	lim := Limits{MaxXMLDepth: 16, MaxTextRunes: 4096}
	f.Fuzz(func(t *testing.T, data []byte) {
		d := NewDecoder(bytes.NewReader(data), lim)
		for i := 0; i < 1<<20; i++ {
			tok, err := d.Next()
			if err != nil {
				break
			}
			_ = tok
			if d.Depth() > lim.MaxXMLDepth {
				t.Fatalf("profundidade %d acima do limite", d.Depth())
			}
			if d.TextRunes() > lim.MaxTextRunes {
				t.Fatalf("texto %d acima do limite", d.TextRunes())
			}
		}
	})
}

func FuzzDOCX(f *testing.F) {
	f.Add(minimalDOCX(f))
	f.Add(dxBuild(f, dxP("Olá")+`<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr>`+dxP("x")+`</w:tc></w:tr></w:tbl>`))
	f.Add(dxBuild(f, `<w:p><w:r><w:fldChar w:fldCharType="begin"/><w:instrText>X</w:instrText><w:fldChar w:fldCharType="end"/></w:r><w:ins><w:r><w:t>a</w:t></w:r></w:ins><w:del><w:r><w:delText>b</w:delText></w:r></w:del></w:p>`+
		`<w:p><w:r><mc:AlternateContent><mc:Choice><w:txbxContent>`+dxP("c")+`</w:txbxContent></mc:Choice><mc:Fallback/></mc:AlternateContent></w:r></w:p>`))
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", ""), nbLvl(1, "upperRoman", "%1.%2", nbSuff("space"))) + nbNum("1", "0", `<w:lvlOverride w:ilvl="0"><w:startOverride w:val="3"/></w:lvlOverride>`)
	styles := nbStyle("A", "B", "") + nbStyle("B", "A", `<w:numId w:val="1"/>`)
	f.Add(nbDocx(f, num, styles, nbP("1", 0, "a")+nbP("1", 1, "b")+nbStylePara("A", "c")))
	f.Add(dxBuild(f, dxP("x")+`<w:sectPr><w:headerReference r:id="rId1"/></w:sectPr>`,
		entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="t" Target="header1.xml"/>`)},
		entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("h"))},
		entry{name: "word/footnotes.xml", data: dxPartXML("footnotes", `<w:footnote w:id="1">`+dxP("n")+`</w:footnote>`)},
		entry{name: "word/comments.xml", data: dxPartXML("comments", `<w:comment w:id="1" w:author="a">`+dxP("c")+`</w:comment>`)}))
	f.Add([]byte("PK\x03\x04"))
	lim := Limits{MaxEntries: 50, MaxEntryBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxXMLDepth: 32, MaxTextRunes: 1 << 16}
	f.Fuzz(func(t *testing.T, data []byte) {
		// extractDOCX, not Extract: Extract would swallow a panic and hide it.
		r, err := extractDOCX(context.Background(), data, lim)
		if err != nil {
			if _, ok := ReasonOf(err); !ok {
				t.Fatalf("erro não tipado: %v", err)
			}
			return
		}
		if _, err := NormalizeText(r.Text); err != nil {
			if _, ok := ReasonOf(err); !ok {
				t.Fatalf("erro não tipado: %v", err)
			}
		}
		if n := utf8.RuneCountInString(r.Text); n > lim.MaxTextRunes+1 {
			t.Fatalf("texto com %d runas passa do teto", n)
		}
	})
}
