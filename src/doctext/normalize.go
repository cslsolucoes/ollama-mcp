package doctext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeText applies the text contract of files.File.Text to extracted
// text: CRLF and lone CR become LF, a leading BOM is dropped, invalid UTF-8 is
// replaced by U+FFFD (and counted), and control characters other than \n and
// \t are removed. If more than 1% of the resulting runes are U+FFFD the text
// is refused with ReasonCorruptText, whose message is the PDF one of the brief
// ("fontes sem mapeamento de caracteres"): that symptom comes from PDF fonts
// without ToUnicode, and the wording is kept exactly as specified.
func NormalizeText(s string) (string, error) {
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	var b strings.Builder
	b.Grow(len(s))
	total, bad := 0, 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == utf8.RuneError && size == 1:
			// Invalid byte sequence.
			bad++
			total++
			b.WriteRune(utf8.RuneError)
			continue
		case r == '\r':
			if i < len(s) && s[i] == '\n' {
				i++
			}
			r = '\n'
		case r != '\n' && r != '\t' && unicode.IsControl(r):
			continue
		}
		if r == utf8.RuneError { // a genuine U+FFFD in the input
			bad++
		}
		total++
		b.WriteRune(r)
	}
	if total > 0 && bad*100 > total {
		return "", refuse(ReasonCorruptText)
	}
	return b.String(), nil
}
