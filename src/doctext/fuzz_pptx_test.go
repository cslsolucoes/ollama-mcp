package doctext

import (
	"context"
	"testing"
)

func FuzzPPTX(f *testing.F) {
	f.Add(samplePPTX(f))
	f.Add(buildPPTX(f, presXML("rId1"), []string{slideRel("rId1", "slides/slide1.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML(`show="0"`, textShape("a")+`<a:tbl><a:tr>`+cell(`gridSpan="2"`, "x")+`</a:tr></a:tbl>`)}))
	f.Add(buildZip(f, entry{name: "ppt/presentation.xml", data: []byte("<p/>")}))
	f.Add([]byte("PK\x03\x04"))
	lim := Limits{MaxEntries: 50, MaxEntryBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxXMLDepth: 32, MaxTextRunes: 1 << 16}
	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := extractPPTX(context.Background(), data, lim)
		if err != nil {
			if _, ok := ReasonOf(err); !ok {
				t.Fatalf("erro não tipado: %v", err)
			}
			return
		}
		if r.Format != FormatPPTX {
			t.Fatal("formato errado")
		}
	})
}
