package doctext

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"
)

// Fixtures are generated in code: no third-party binary lives in the repo.

type entry struct {
	name   string
	data   []byte
	method uint16 // zero value means Deflate
}

func buildZip(tb testing.TB, entries ...entry) []byte {
	tb.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		m := e.method
		if m == 0 {
			m = zip.Deflate
		}
		if m == 255 { // test alias for Store
			m = zip.Store
		}
		fw, err := w.CreateHeader(&zip.FileHeader{Name: e.name, Method: m})
		if err != nil {
			tb.Fatalf("CreateHeader(%q): %v", e.name, err)
		}
		if _, err := fw.Write(e.data); err != nil {
			tb.Fatalf("Write(%q): %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		tb.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

// store is the method alias for a stored (uncompressed) entry in buildZip.
const store = 255

// offsets returns every offset of sig in data.
func offsets(data, sig []byte) []int {
	var out []int
	for i := 0; ; {
		j := bytes.Index(data[i:], sig)
		if j < 0 {
			return out
		}
		out = append(out, i+j)
		i += j + 1
	}
}

var (
	sigCentral = []byte("PK\x01\x02")
	sigLocal   = []byte("PK\x03\x04")
)

// patchCentral edits the n-th central directory header: flags at +8, method at
// +10, uncompressed size at +24.
func patchCentral(tb testing.TB, data []byte, n int, off int, v []byte) {
	tb.Helper()
	o := offsets(data, sigCentral)
	if n >= len(o) {
		tb.Fatalf("central header %d not found", n)
	}
	copy(data[o[n]+off:], v)
}

func le16(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
func le32(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }

const (
	nsW  = NSWordprocessing
	nsWS = NSWordprocessingStrict
)

func docXML(body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="` + nsW + `"><w:body>` + body + `</w:body></w:document>`)
}

func relsXML(rels ...string) []byte {
	s := `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="` + NSPackageRels + `">`
	for _, r := range rels {
		s += r
	}
	return []byte(s + `</Relationships>`)
}

func minimalDOCX(tb testing.TB) []byte {
	return buildZip(tb,
		entry{name: "[Content_Types].xml", data: []byte(`<Types/>`)},
		entry{name: "word/document.xml", data: docXML(`<w:p><w:r><w:t>Olá</w:t></w:r></w:p>`)},
		entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="t" Target="styles.xml"/>`)},
		entry{name: "word/styles.xml", data: []byte(`<w:styles xmlns:w="` + nsW + `"/>`)},
	)
}
