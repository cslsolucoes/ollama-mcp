package doctext

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// Minimal PDF writers for the tests. Every fixture of the PDF tests is built
// here, in code, with the standard library only: no third-party or customer
// document ever enters the repository.

// pdfPage is one page: its text lines (empty slice = a blank page).
type pdfPage []string

// pdfLit encodes s as a PDF literal string in WinAnsi: runes up to U+00FF map
// to the same byte (true for the Latin-1 part of cp1252 that the tests use).
func pdfLit(s string) []byte {
	var b bytes.Buffer
	b.WriteByte('(')
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteByte(byte(r))
		case r > 0xFF:
			b.WriteByte('?')
		case r >= 0x80 || r < 0x20:
			fmt.Fprintf(&b, "\\%03o", r)
		default:
			b.WriteByte(byte(r))
		}
	}
	b.WriteByte(')')
	return b.Bytes()
}

// pdfContent builds the content stream of a page: one Tj per line, 14 pt apart.
func pdfContent(lines pdfPage) []byte {
	var b bytes.Buffer
	if len(lines) > 0 {
		b.WriteString("BT /F1 12 Tf 72 720 Td 14 TL\n")
		for i, l := range lines {
			if i > 0 {
				b.WriteString("T*\n")
			}
			b.Write(pdfLit(l))
			b.WriteString(" Tj\n")
		}
		b.WriteString("ET\n")
	}
	return b.Bytes()
}

// pdfEnc describes the standard security handler (RC4) of a generated file.
type pdfEnc struct {
	rev       int    // 2 (40 bits) or 3 (128 bits)
	userPW    string // empty = opens without a password (permission-only protection)
	ownerPW   string
	key       []byte // file key, derived
	oStr, uSt []byte
}

var pdfPad = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padPW(pw string) []byte {
	b := append([]byte(pw), pdfPad...)
	return b[:32]
}

func rc4Do(key, data []byte) []byte {
	c, err := rc4.NewCipher(key)
	if err != nil {
		panic(err)
	}
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

func xorKey(key []byte, i byte) []byte {
	k := make([]byte, len(key))
	for j := range key {
		k[j] = key[j] ^ i
	}
	return k
}

// newPDFEnc derives O, U and the file key as in PDF 1.4 section 3.5
// (algorithms 3.2 to 3.5). id is the first element of the trailer /ID.
func newPDFEnc(rev int, userPW, ownerPW string, id []byte, perms int32) *pdfEnc {
	n := 5
	if rev == 3 {
		n = 16
	}
	e := &pdfEnc{rev: rev, userPW: userPW, ownerPW: ownerPW}
	// Algorithm 3.3: the O entry.
	h := md5.Sum(padPW(ownerPW))
	sum := h[:]
	if rev == 3 {
		for i := 0; i < 50; i++ {
			h = md5.Sum(sum)
			sum = h[:]
		}
	}
	okey := sum[:n]
	o := rc4Do(okey, padPW(userPW))
	if rev == 3 {
		for i := byte(1); i <= 19; i++ {
			o = rc4Do(xorKey(okey, i), o)
		}
	}
	e.oStr = o
	// Algorithm 3.2: the file key.
	var in bytes.Buffer
	in.Write(padPW(userPW))
	in.Write(o)
	_ = binary.Write(&in, binary.LittleEndian, uint32(perms))
	in.Write(id)
	h = md5.Sum(in.Bytes())
	sum = h[:]
	if rev == 3 {
		for i := 0; i < 50; i++ {
			h = md5.Sum(sum[:n])
			sum = h[:]
		}
	}
	e.key = append([]byte(nil), sum[:n]...)
	// Algorithms 3.4 / 3.5: the U entry.
	if rev == 2 {
		e.uSt = rc4Do(e.key, pdfPad)
	} else {
		var u bytes.Buffer
		u.Write(pdfPad)
		u.Write(id)
		h = md5.Sum(u.Bytes())
		x := rc4Do(e.key, h[:])
		for i := byte(1); i <= 19; i++ {
			x = rc4Do(xorKey(e.key, i), x)
		}
		e.uSt = append(x, make([]byte, 16)...)
	}
	return e
}

// objKey is the per-object RC4 key (algorithm 3.1).
func (e *pdfEnc) objKey(num int) []byte {
	b := append([]byte(nil), e.key...)
	b = append(b, byte(num), byte(num>>8), byte(num>>16), 0, 0)
	h := md5.Sum(b)
	n := len(e.key) + 5
	if n > 16 {
		n = 16
	}
	return h[:n]
}

// buildPDF builds a complete PDF: objects 1 (catalog), 2 (pages), 3 (font), then
// a page object and a content object per page. A nil enc leaves it in the clear.
func buildPDF(tb testing.TB, pages []pdfPage, winAnsi bool, enc *pdfEnc, id []byte) []byte {
	tb.Helper()
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	var offs []int
	obj := func(body []byte) {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n", len(offs))
		out.Write(body)
		out.WriteString("\nendobj\n")
	}
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 4+2*i)
	}
	obj([]byte("<< /Type /Catalog /Pages 2 0 R >>"))
	obj([]byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(pages), strings.Join(kids, " "))))
	font := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica"
	if winAnsi {
		font += " /Encoding /WinAnsiEncoding"
	}
	obj([]byte(font + " >>"))
	for i, p := range pages {
		pageNum := 4 + 2*i
		obj([]byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", pageNum+1)))
		c := pdfContent(p)
		if enc != nil {
			c = rc4Do(enc.objKey(pageNum+1), c)
		}
		body := append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(c))), c...)
		body = append(body, "\nendstream"...)
		obj(body)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	trailer := fmt.Sprintf("<< /Size %d /Root 1 0 R", len(offs)+1)
	if id != nil {
		trailer += fmt.Sprintf(" /ID [<%s> <%s>]", hex.EncodeToString(id), hex.EncodeToString(id))
	}
	if enc != nil {
		length := 40
		v := 1
		if enc.rev == 3 {
			length, v = 128, 2
		}
		trailer += fmt.Sprintf(" /Encrypt << /Filter /Standard /V %d /Length %d /R %d /O <%s> /U <%s> /P -4 >>",
			v, length, enc.rev, hex.EncodeToString(enc.oStr), hex.EncodeToString(enc.uSt))
	}
	fmt.Fprintf(&out, "trailer\n%s >>\nstartxref\n%d\n%%%%EOF\n", trailer, xref)
	return out.Bytes()
}

var pdfTestID = []byte("0123456789abcdef")

// encPDF builds an encrypted PDF (RC4, rev 2 or 3) with the given passwords.
func encPDF(tb testing.TB, rev int, userPW, ownerPW string, pages []pdfPage) []byte {
	tb.Helper()
	enc := newPDFEnc(rev, userPW, ownerPW, pdfTestID, -4)
	return buildPDF(tb, pages, false, enc, pdfTestID)
}

const (
	pdfLine1 = "Contrato de prestacao de servicos numero 123"
	pdfLine2 = "Clausula primeira: objeto do contrato"
)

// Fixtures.
func pdfASCII(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{{pdfLine1, pdfLine2}}, false, nil, nil)
}

func pdfAccents(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{{"Ação de cobrança, concessão e opinião é válida", "Título: coração, órgão, lição"}}, true, nil, nil)
}

func pdfThreePages(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{
		{"Primeira pagina com texto suficiente para contar"},
		{"Segunda pagina com texto suficiente para contar"},
		{"Terceira pagina com texto suficiente para contar"},
	}, false, nil, nil)
}

func pdfBlankMiddle(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{
		{"Primeira pagina com texto suficiente para contar"},
		{},
		{"Terceira pagina com texto suficiente para contar"},
	}, false, nil, nil)
}

func pdfAllBlank(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{{}, {"1"}, {}}, false, nil, nil)
}

func pdfHyphen(tb testing.TB) []byte {
	return buildPDF(tb, []pdfPage{{
		"Este contra-",
		"to termina aqui e continua",
		"Rio-",
		"Sao nao junta pois e maiuscula",
		"valor 10 -",
		"mais nao junta",
	}}, false, nil, nil)
}
