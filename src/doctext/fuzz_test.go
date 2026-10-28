package doctext

import (
	"bytes"
	"strings"
	"testing"
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
