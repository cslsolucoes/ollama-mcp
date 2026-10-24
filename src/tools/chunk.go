package tools

import (
	"strings"
	"unicode/utf8"
)

// splitChunks cuts text into parts of at most max runes, on line boundaries
// (FILE-009). Each part keeps its trailing newline, so strings.Join(parts, "")
// is the original text: nothing is dropped and nothing is added.
//
// A single line longer than max cannot be cut on a boundary; it is split by
// runes (never inside a UTF-8 sequence). That is the only case where a part
// ends mid-line.
func splitChunks(text string, max int) []string {
	if max < 1 {
		max = 1
	}
	if utf8.RuneCountInString(text) <= max {
		return []string{text}
	}
	var (
		parts []string
		cur   strings.Builder
		curN  int
	)
	flush := func() {
		if curN > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
			curN = 0
		}
	}
	for len(text) > 0 {
		line := text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line = text[:i+1]
		}
		text = text[len(line):]
		n := utf8.RuneCountInString(line)
		switch {
		case n > max:
			flush()
			rs := []rune(line)
			for len(rs) > 0 {
				k := max
				if k > len(rs) {
					k = len(rs)
				}
				parts = append(parts, string(rs[:k]))
				rs = rs[k:]
			}
		case curN+n > max:
			flush()
			cur.WriteString(line)
			curN = n
		default:
			cur.WriteString(line)
			curN += n
		}
	}
	flush()
	return parts
}
