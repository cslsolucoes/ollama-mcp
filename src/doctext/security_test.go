package doctext

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func wantReason(t *testing.T, err error, want Reason) {
	t.Helper()
	r, ok := ReasonOf(err)
	if !ok || r != want {
		t.Fatalf("razão = %v (ok=%v), esperado %v; err=%v", r, ok, want, err)
	}
}

func TestMinimalPackageOK(t *testing.T) {
	p, err := OpenPackage(minimalDOCX(t), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Has("word/document.xml") || p.Has("word/../word/document.xml") || p.Has("/word/document.xml") || p.Has("word/missing.xml") {
		t.Fatal("Has com resultado errado")
	}
	got := strings.Join(p.Names(), ",")
	if got != "[Content_Types].xml,word/_rels/document.xml.rels,word/document.xml,word/styles.xml" {
		t.Fatalf("Names fora de ordem/incorreto: %s", got)
	}
	b, err := p.Read("word/document.xml")
	if err != nil || !bytes.Contains(b, []byte("Olá")) {
		t.Fatalf("Read: %v", err)
	}
	if _, err := p.Read("word/missing.xml"); err == nil {
		t.Fatal("parte ausente deveria falhar")
	}
	rels, err := p.Relationships("word/document.xml")
	if err != nil || len(rels) != 1 || rels[0].Target != "word/styles.xml" || rels[0].ID != "rId1" {
		t.Fatalf("rels = %+v, %v", rels, err)
	}
	if r, err := p.Relationships("word/styles.xml"); r != nil || err != nil {
		t.Fatalf("parte sem .rels deveria dar nil,nil: %v %v", r, err)
	}
}

func TestNotAZip(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("PK\x03\x04"), []byte("%PDF-1.4 não é zip"), bytes.Repeat([]byte{0}, 100)} {
		_, err := OpenPackage(data, Limits{})
		wantReason(t, err, ReasonMalformed)
	}
}

// peakHeap samples HeapAlloc while f runs and returns the highest value seen
// above the baseline taken before.
func peakHeap(f func()) (peak, totalAlloc uint64) {
	runtime.GC()
	var base, end runtime.MemStats
	runtime.ReadMemStats(&base)
	var stop atomic.Bool
	done := make(chan uint64)
	go func() {
		var max uint64
		var ms runtime.MemStats
		for !stop.Load() {
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > max {
				max = ms.HeapAlloc
			}
			time.Sleep(time.Millisecond)
		}
		done <- max
	}()
	f()
	stop.Store(true)
	max := <-done
	runtime.ReadMemStats(&end)
	if max > base.HeapAlloc {
		peak = max - base.HeapAlloc
	}
	return peak, end.TotalAlloc - base.TotalAlloc
}

func zeros(tb testing.TB, name string, mib int) []byte {
	tb.Helper()
	chunk := make([]byte, 1<<20)
	var big bytes.Buffer
	for i := 0; i < mib; i++ {
		big.Write(chunk)
	}
	return buildZip(tb, entry{name: name, data: big.Bytes()})
}

func TestZipBomb(t *testing.T) {
	honest := zeros(t, "word/document.xml", 100) // 100 MiB of zeros, ~100 KiB compressed
	t.Logf("pacote de %d bytes comprimidos; descompactado real: 100 MiB", len(honest))

	// Case 1: the header is honest (100 MiB > 64 MiB), so the part is refused
	// before anything is decompressed: archive/zip could never deliver a size
	// other than the declared one. (Our own byte count, for a reader that
	// breaks that contract, is TestReadBoundedOwnCount.)
	p, err := OpenPackage(honest, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var rerr error
	peak, total := peakHeap(func() { _, rerr = p.Read("word/document.xml") })
	wantReason(t, rerr, ReasonMalformed)
	t.Logf("MEMORIA zip bomb 100 MiB (limite de entrada 64 MiB): pico de heap acima da base = %d MiB; total alocado = %d MiB",
		peak>>20, total>>20)
	if total > 1<<20 {
		t.Fatalf("a recusa pelo tamanho declarado alocou %d bytes (deveria recusar antes de descompactar)", total)
	}
	if p.total <= DefaultLimits().MaxEntryBytes {
		t.Fatalf("a recusa deveria cobrar o orçamento como uma leitura que estourou o limite (total=%d)", p.total)
	}

	// Case 2: small cap, honest header.
	p2, err := OpenPackage(honest, Limits{MaxEntryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	peak2, _ := peakHeap(func() { _, rerr = p2.Read("word/document.xml") })
	wantReason(t, rerr, ReasonMalformed)
	t.Logf("MEMORIA zip bomb com limite de entrada 1 MiB: pico = %d MiB", peak2>>20)
	if peak2 > 16<<20 {
		t.Fatalf("com teto de 1 MiB o pico deveria ser pequeno, foi %d MiB", peak2>>20)
	}

	// Case 3: the header lies (declares 1 KiB, central directory +24). The
	// standard reader may itself notice the mismatch; either way the package
	// must refuse and never return 100 MiB.
	lying := append([]byte(nil), honest...)
	patchCentral(t, lying, 0, 24, le32(1024))
	p3, err := OpenPackage(lying, Limits{})
	if err != nil {
		t.Fatalf("a abertura não deveria falhar (só o cabeçalho mente): %v", err)
	}
	var out []byte
	peak3, _ := peakHeap(func() { out, rerr = p3.Read("word/document.xml") })
	wantReason(t, rerr, ReasonMalformed)
	if len(out) != 0 {
		t.Fatalf("devolveu %d bytes de uma bomba", len(out))
	}
	t.Logf("MEMORIA zip bomb com cabeçalho mentiroso (1 KiB declarado): pico = %d MiB", peak3>>20)
}

func TestMaxTotalBytesAcrossEntries(t *testing.T) {
	chunk := bytes.Repeat([]byte("a"), 600<<10)
	data := buildZip(t,
		entry{name: "a.xml", data: chunk}, entry{name: "b.xml", data: chunk}, entry{name: "c.xml", data: chunk})
	p, err := OpenPackage(data, Limits{MaxEntryBytes: 1 << 20, MaxTotalBytes: 1500 << 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.xml", "b.xml"} {
		if _, err := p.Read(n); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	_, err = p.Read("c.xml") // 1,8 MiB acumulados > 1,5 MiB
	wantReason(t, err, ReasonMalformed)
	// the budget is spent for good: even a tiny read now fails.
	if _, err := p.Read("a.xml"); err == nil {
		t.Fatal("o orçamento total deveria continuar estourado")
	}
}

// A part right at the default per-entry limit (64 MiB) is read with a single
// allocation of its size. io.ReadAll allocated ~157 MiB and peaked at ~156 MiB
// for the same part (growing chunks plus the final copy).
func TestReadPresizedAtDefaultLimit(t *testing.T) {
	unit := []byte("<w:t>abcdefgh</w:t>")
	part := bytes.Repeat(unit, (64<<20)/len(unit)+1)[:64<<20]
	for _, m := range []uint16{0, store} {
		data := buildZip(t, entry{name: "word/document.xml", data: part, method: m})
		p, err := OpenPackage(data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var out []byte
		var rerr error
		start := time.Now()
		peak, total := peakHeap(func() { out, rerr = p.Read("word/document.xml") })
		el := time.Since(start)
		if rerr != nil || !bytes.Equal(out, part) {
			t.Fatalf("método %d: a parte de 64 MiB, no limite, deveria ser lida: %v (len %d)", m, rerr, len(out))
		}
		t.Logf("MEMORIA parte de 64 MiB (método %d): pico = %d MiB, total alocado = %d MiB, tempo = %v", m, peak>>20, total>>20, el)
		if total > 80<<20 {
			t.Fatalf("método %d: alocou %d MiB para ler 64 MiB (esperado ~64)", m, total>>20)
		}
		if el > 5*time.Second {
			t.Fatalf("método %d: leitura levou %v", m, el)
		}
		out = nil
	}
}

// A header that lies UPWARD (declares 1 MiB, holds 1 KiB) makes Read pre-size
// for nothing; the allocation is charged to MaxTotalBytes, so repeated reads of
// lying parts cannot allocate without bound.
func TestReadLyingUpwardChargesBudget(t *testing.T) {
	data := buildZip(t, entry{name: "a.xml", data: bytes.Repeat([]byte("a"), 1024)}, entry{name: "b.xml", data: []byte("b")})
	patchCentral(t, data, 0, 24, le32(1<<20))
	p, err := OpenPackage(data, Limits{MaxEntryBytes: 1 << 20, MaxTotalBytes: 3 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, err := p.Read("a.xml")
		wantReason(t, err, ReasonMalformed)
	}
	if _, err := p.Read("b.xml"); err == nil {
		t.Fatal("as pré-alocações dos cabeçalhos mentirosos deveriam ter gasto o orçamento")
	}
}

// readBounded keeps its own count: a reader that ignores any declared size
// (archive/zip's contract broken) still cannot go far past max.
func TestReadBoundedOwnCount(t *testing.T) {
	buf, err := readBounded(io.LimitReader(endlessZeros{}, 10<<20), 1024, 1<<20)
	if err != errTooBig {
		t.Fatalf("err = %v, esperado errTooBig", err)
	}
	if len(buf) > 2<<20 {
		t.Fatalf("leu %d bytes, bem além do teto de 1 MiB", len(buf))
	}
	// Control: exactly max bytes pass, with an exact hint and with none.
	for _, hint := range []int64{1 << 20, 0} {
		buf, err = readBounded(io.LimitReader(endlessZeros{}, 1<<20), hint, 1<<20)
		if err != nil || len(buf) != 1<<20 {
			t.Fatalf("hint %d: %d bytes, err %v; esperado 1 MiB sem erro", hint, len(buf), err)
		}
	}
}

type endlessZeros struct{}

func (endlessZeros) Read(b []byte) (int, error) { clear(b); return len(b), nil }

func TestTenThousandEntriesRefusedBeforeIterating(t *testing.T) {
	entries := make([]entry, 10000)
	for i := range entries {
		entries[i] = entry{name: fmt.Sprintf("f/%05d.xml", i), data: []byte("x"), method: store}
	}
	data := buildZip(t, entries...)
	var visited int
	testHookEntry = func() { visited++ }
	defer func() { testHookEntry = nil }()
	_, err := OpenPackage(data, Limits{})
	wantReason(t, err, ReasonMalformed)
	if visited != 0 {
		t.Fatalf("%d entradas visitadas antes de recusar; esperado 0", visited)
	}
	// Control: right at the limit it opens, and every entry is visited.
	visited = 0
	if _, err := OpenPackage(buildZip(t, entries[:2000]...), Limits{}); err != nil {
		t.Fatalf("2000 entradas deveriam abrir: %v", err)
	}
	if visited != 2000 {
		t.Fatalf("controle: visitadas %d, esperado 2000", visited)
	}
}

func TestUnsafeEntryNames(t *testing.T) {
	for _, name := range []string{
		"../../evil", "word/../../evil", "..", "word/..", `word\document.xml`, `..\evil`,
		"/etc/passwd", "/word/document.xml", "C:/evil.xml", "a\x00b",
	} {
		data := buildZip(t, entry{name: "word/document.xml", data: []byte("x")}, entry{name: name, data: []byte("y")})
		_, err := OpenPackage(data, Limits{})
		if err == nil {
			t.Errorf("nome %q deveria recusar o pacote", name)
			continue
		}
		wantReason(t, err, ReasonMalformed)
	}
}

func TestDuplicateEntry(t *testing.T) {
	for _, dup := range []string{"word/document.xml", "word/./document.xml", "word//document.xml", "WORD/Document.xml"} {
		data := buildZip(t, entry{name: "word/document.xml", data: []byte("real")}, entry{name: dup, data: []byte("fake")})
		_, err := OpenPackage(data, Limits{})
		if err == nil {
			t.Errorf("duplicata %q deveria recusar", dup)
			continue
		}
		wantReason(t, err, ReasonMalformed)
	}
}

func TestEncryptedEntry(t *testing.T) {
	data := buildZip(t, entry{name: "word/document.xml", data: []byte("x")})
	patchCentral(t, data, 0, 8, le16(0x0001|0x0008)) // bit 0 = encrypted (+ data descriptor kept)
	_, err := OpenPackage(data, Limits{})
	wantReason(t, err, ReasonMalformed)
}

func TestUnsupportedMethod(t *testing.T) {
	data := buildZip(t, entry{name: "word/document.xml", data: []byte("x"), method: store})
	patchCentral(t, data, 0, 10, le16(12)) // bzip2
	_, err := OpenPackage(data, Limits{})
	wantReason(t, err, ReasonMalformed)
}

// failingTransport fails the test if anything ever tries to use the network.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("acesso de rede inesperado: %s", r.URL.Scheme)
	return nil, fmt.Errorf("rede proibida")
}

func TestRelationships(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = failingTransport{t}
	defer func() { http.DefaultTransport = old }()

	pkg := func(rels ...string) *Package {
		data := buildZip(t,
			entry{name: "word/document.xml", data: docXML("")},
			entry{name: "word/_rels/document.xml.rels", data: relsXML(rels...)})
		p, err := OpenPackage(data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	// Valid targets, with External entries interleaved: skipped silently.
	p := pkg(
		`<Relationship Id="r1" Type="h" Target="header1.xml"/>`,
		`<Relationship Id="r2" Type="l" Target="http://example.invalid/x" TargetMode="External"/>`,
		`<Relationship Id="r3" Type="l" Target="file:///C:/Windows/win.ini" TargetMode="External"/>`,
		`<Relationship Id="r4" Type="c" Target="../customXml/item1.xml"/>`,
		`<Relationship Id="r5" Type="x" Target="media%2Fimg.png"/>`,
		`<Relationship Id="r6" Type="l" Target="https://example.invalid/y" TargetMode="external"/>`,
	)
	rels, err := p.Relationships("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rels {
		got = append(got, r.ID+"="+r.Target)
	}
	want := "r1=word/header1.xml,r4=customXml/item1.xml,r5=word/media/img.png"
	if strings.Join(got, ",") != want {
		t.Fatalf("rels = %v; esperado %s", got, want)
	}

	// Refused targets.
	for _, bad := range []string{
		"../../evil.xml", "../../../etc/passwd", "/etc/passwd", "/word/header1.xml", `..\evil`, `a\b.xml`,
		"http://example.invalid/x", "file:///C:/x", "C:/x.xml", "..%2F..%2Fevil.xml", "%2Fabs.xml", "..",
	} {
		_, err := pkg(`<Relationship Id="r" Type="t" Target="` + bad + `"/>`).Relationships("word/document.xml")
		if err == nil {
			t.Errorf("alvo %q deveria recusar (não é External)", bad)
			continue
		}
		wantReason(t, err, ReasonMalformed)
	}
}

func TestRelationshipsDoctypeRefused(t *testing.T) {
	data := buildZip(t,
		entry{name: "word/document.xml", data: docXML("")},
		entry{name: "word/_rels/document.xml.rels", data: []byte(`<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x "y">]><Relationships xmlns="` + NSPackageRels + `"/>`)})
	p, err := OpenPackage(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Relationships("word/document.xml")
	wantReason(t, err, ReasonMalformed)
}

func drain(d *Decoder) error {
	for {
		if _, err := d.Next(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func TestXMLDoctypeRefused(t *testing.T) {
	for _, doc := range []string{
		`<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///C:/Windows/win.ini">]><a>&xxe;</a>`,
		`<!DOCTYPE a><a/>`,
		`<!doctype a SYSTEM "http://example.invalid/a.dtd"><a/>`,
		`<a><!ENTITY x "y"></a>`,
	} {
		err := drain(NewDecoder(strings.NewReader(doc), Limits{}))
		if err == nil {
			t.Errorf("DTD deveria recusar: %.40s", doc)
			continue
		}
		wantReason(t, err, ReasonMalformed)
	}
	// Control: a plain document drains fine, comments and PIs included.
	if err := drain(NewDecoder(strings.NewReader(`<?xml version="1.0"?><!-- c --><a><b>t</b></a>`), Limits{})); err != nil {
		t.Fatal(err)
	}
}

func TestXMLDepth(t *testing.T) {
	deep := strings.Repeat("<a>", 10000) + strings.Repeat("</a>", 10000)
	d := NewDecoder(strings.NewReader(deep), Limits{})
	wantReason(t, drain(d), ReasonMalformed)
	if d.Depth() > DefaultLimits().MaxXMLDepth+1 {
		t.Fatalf("o decoder desceu até %d níveis", d.Depth())
	}
	// exactly at the limit is fine.
	ok := strings.Repeat("<a>", 256) + strings.Repeat("</a>", 256)
	if err := drain(NewDecoder(strings.NewReader(ok), Limits{})); err != nil {
		t.Fatalf("profundidade 256 deveria passar: %v", err)
	}
	over := strings.Repeat("<a>", 257) + strings.Repeat("</a>", 257)
	wantReason(t, drain(NewDecoder(strings.NewReader(over), Limits{})), ReasonMalformed)
}

func TestXMLTextRunes(t *testing.T) {
	doc := "<a>" + strings.Repeat("ç", 1000) + "</a>"
	d := NewDecoder(strings.NewReader(doc), Limits{MaxTextRunes: 100})
	wantReason(t, drain(d), ReasonTooLong)
	// the limit counts runes, not bytes: 1000 two-byte runes under 1000 passes.
	d = NewDecoder(strings.NewReader(doc), Limits{MaxTextRunes: 1000})
	if err := drain(d); err != nil || d.TextRunes() != 1000 {
		t.Fatalf("1000 runas no limite de 1000: err=%v runas=%d", err, d.TextRunes())
	}
	// Many small chunks accumulate.
	many := "<a>" + strings.Repeat("<b>xxxxxxxxxx</b>", 50) + "</a>"
	wantReason(t, drain(NewDecoder(strings.NewReader(many), Limits{MaxTextRunes: 400})), ReasonTooLong)
}

func TestXMLMalformedAndCharset(t *testing.T) {
	for _, doc := range []string{`<a><b></a>`, `<a`, `<a attr=></a>`, `<?xml version="1.0" encoding="ISO-8859-1"?><a/>`} {
		err := drain(NewDecoder(strings.NewReader(doc), Limits{}))
		if err == nil {
			t.Errorf("deveria recusar: %q", doc)
			continue
		}
		wantReason(t, err, ReasonMalformed)
	}
}

func firstStart(t *testing.T, doc string) xml.StartElement {
	t.Helper()
	d := NewDecoder(strings.NewReader(doc), Limits{})
	for {
		tok, err := d.Next()
		if err != nil {
			t.Fatal(err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se
		}
	}
}

func TestNamespacesByURI(t *testing.T) {
	// non standard prefix, transitional URI
	se := firstStart(t, `<x:p xmlns:x="`+NSWordprocessing+`" x:val="v"/>`)
	if !IsW(se.Name, "p") || IsW(se.Name, "r") {
		t.Fatal("prefixo não padrão com URI transicional deveria ser reconhecido")
	}
	if v, ok := AttrW(se, "val"); !ok || v != "v" {
		t.Fatal("AttrW com prefixo não padrão")
	}
	// Strict URIs
	se = firstStart(t, `<w:p xmlns:w="`+NSWordprocessingStrict+`" w:val="s"/>`)
	if !IsW(se.Name, "p") {
		t.Fatal("URI Strict de WordprocessingML não reconhecida")
	}
	if v, _ := AttrW(se, "val"); v != "s" {
		t.Fatal("AttrW Strict")
	}
	se = firstStart(t, `<a:t xmlns:a="`+NSDrawingStrict+`"/>`)
	if !IsA(se.Name, "t") {
		t.Fatal("DrawingML Strict")
	}
	se = firstStart(t, `<p:sld xmlns:p="`+NSPresentationStrict+`"/>`)
	if !IsP(se.Name, "sld") {
		t.Fatal("PresentationML Strict")
	}
	se = firstStart(t, `<q:sld xmlns:q="`+NSPresentation+`" xmlns:rr="`+NSRelationshipsStrict+`" rr:id="rId9" show="0"/>`)
	if !IsP(se.Name, "sld") {
		t.Fatal("PresentationML com prefixo q")
	}
	if v, ok := AttrR(se, "id"); !ok || v != "rId9" {
		t.Fatal("AttrR com URI Strict")
	}
	if v, ok := AttrPlain(se, "show"); !ok || v != "0" {
		t.Fatal("AttrPlain")
	}
	se = firstStart(t, `<mc:Choice xmlns:mc="`+NSMarkupCompat+`" Requires="wps"/>`)
	if !IsMC(se.Name, "Choice") {
		t.Fatal("markup-compatibility")
	}
	// A prefix "w" bound to another URI must NOT match: URI decides, not prefix.
	se = firstStart(t, `<w:p xmlns:w="http://example.invalid/other"/>`)
	if IsW(se.Name, "p") || IsA(se.Name, "p") || IsP(se.Name, "p") {
		t.Fatal("prefixo w com outra URI não pode casar")
	}
	// A namespaced attribute does not match AttrPlain, and vice versa.
	se = firstStart(t, `<a xmlns:w="`+NSWordprocessing+`" w:val="1" val="2"/>`)
	if v, _ := AttrW(se, "val"); v != "1" {
		t.Fatal("AttrW deveria ignorar o atributo sem namespace")
	}
	if v, _ := AttrPlain(se, "val"); v != "2" {
		t.Fatal("AttrPlain deveria ignorar o atributo com namespace")
	}
}

// The reader must be deterministic: same input, same Names and Relationships.
func TestPackageDeterminism(t *testing.T) {
	data := minimalDOCX(t)
	var first string
	for i := 0; i < 20; i++ {
		p, err := OpenPackage(data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		rels, _ := p.Relationships("word/document.xml")
		s := fmt.Sprint(p.Names(), rels)
		if i == 0 {
			first = s
		} else if s != first {
			t.Fatal("saída dependente de ordem de mapa")
		}
	}
}
