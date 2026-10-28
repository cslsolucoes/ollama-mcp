package doctext

import (
	"bytes"
	"html"
	"path"
	"strings"
	"unicode/utf8"
)

// w:altChunk (decision of the owner, 7.7a M2): content imported into a DOCX
// (HTML, plain text or another DOCX) that Word renders in place of the chunk
// but that is not WordprocessingML. TXT, HTML and DOCX (one level deep) are
// extracted at the place of the chunk and the result says ImportedContent;
// anything else (MHT, RTF, a broken chunk, a DOCX inside an imported DOCX)
// is not read and the result says UnreadContent. Nothing is silently lost.

// altChunk writes the text of the chunk r:id at the current position.
func (h *docxWalker) altChunk(id string) {
	text, ok, err := h.doc.readAltChunk(h.part, id)
	if err != nil {
		h.err = err
		return
	}
	if !ok {
		h.doc.unread = true
		return
	}
	if p := h.cur(); p != nil { // a chunk is block level; keep the order if it is not
		h.flushAll(p, false)
		p.emitted = true
	}
	text = strings.Trim(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	for _, l := range strings.Split(text, "\n") {
		h.emit(l)
		if h.err != nil {
			return
		}
	}
	h.doc.imported = true
}

// relsOf returns the relationships of a part by id, read once.
func (d *docxDoc) relsOf(part string) (map[string]Relationship, error) {
	if m, ok := d.rels[part]; ok {
		return m, nil
	}
	rels, err := d.pkg.Relationships(part)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Relationship, len(rels))
	for _, r := range rels {
		if _, dup := m[r.ID]; !dup {
			m[r.ID] = r
		}
	}
	if d.rels == nil {
		d.rels = map[string]map[string]Relationship{}
	}
	d.rels[part] = m
	return m, nil
}

// readAltChunk returns the text of a chunk. ok is false when the chunk exists
// in the document but cannot be read (unsupported type, missing or broken
// part): the caller flags UnreadContent. err is a refusal that must stop the
// whole extraction (a cancelled context, the text ceiling, a malformed .rels).
func (d *docxDoc) readAltChunk(part, id string) (text string, ok bool, err error) {
	rels, err := d.relsOf(part)
	if err != nil {
		return "", false, err
	}
	r, found := rels[id]
	if !found || !d.pkg.Has(r.Target) {
		return "", false, nil
	}
	data, rerr := d.pkg.Read(r.Target)
	if rerr != nil {
		return "", false, nil
	}
	switch strings.ToLower(path.Ext(r.Target)) {
	case ".txt":
		data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
		if !utf8.Valid(data) {
			return "", false, nil
		}
		return string(data), true, nil
	case ".htm", ".html", ".xhtml":
		t, good := htmlText(data)
		return t, good, nil
	case ".docx":
		return d.importDOCX(data)
	}
	return "", false, nil
}

// importDOCX extracts a DOCX imported by w:altChunk. It shares the budgets of
// the document: the decompressed bytes still available and the runes of text
// still allowed. A DOCX inside it is not followed (depth 1).
func (d *docxDoc) importDOCX(data []byte) (string, bool, error) {
	if d.depth >= 1 {
		return "", false, nil
	}
	sub := d.lim
	sub.MaxTotalBytes = d.pkg.remaining()
	if sub.MaxTotalBytes <= 0 {
		return "", false, nil
	}
	sub.MaxTextRunes = d.lim.MaxTextRunes - *d.total
	if sub.MaxTextRunes <= 0 {
		return "", false, refuse(ReasonTooLong)
	}
	pkg, err := OpenPackage(data, sub)
	if err != nil {
		return "", false, nil
	}
	total := 0
	child := &docxDoc{ctx: d.ctx, pkg: pkg, lim: sub, total: &total, depth: d.depth + 1}
	res, err := extractDOCXDoc(child)
	d.pkg.charge(pkg.total)
	if err != nil {
		if cerr := ctxErr(d.ctx); cerr != nil {
			return "", false, cerr
		}
		if r, ok := ReasonOf(err); ok && r == ReasonTooLong {
			return "", false, err
		}
		return "", false, nil
	}
	d.importPending = d.importPending || res.PendingRevisions
	d.importNumUnavailable = d.importNumUnavailable || res.NumberingUnavailable
	d.unread = d.unread || res.UnreadContent
	d.hidden = d.hidden || res.HiddenText
	d.imported = d.imported || res.ImportedContent
	return res.Text, true, nil
}

// htmlSkipped are elements whose content is never text of the document.
var htmlSkipped = map[string]bool{"script": true, "style": true, "head": true, "title": true, "noscript": true, "template": true}

// htmlBlocks end a line.
var htmlBlocks = map[string]bool{
	"p": true, "div": true, "br": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "tr": true, "ul": true, "ol": true, "blockquote": true, "pre": true, "section": true,
	"article": true, "header": true, "footer": true, "hr": true, "dt": true, "dd": true, "address": true, "body": true,
}

// htmlText turns an HTML chunk into lines of text. It is a flat scanner, not
// a parser: no tree, no DTD (a <!DOCTYPE> is skipped, never read), no entity
// definitions (only the fixed table of html.UnescapeString), no recursion. The
// output is never longer than the input, so the chunk (already capped by
// MaxEntryBytes) bounds it; its runes are charged by the walker as it emits
// them. ok is false for a chunk that is not valid UTF-8 (another charset).
func htmlText(data []byte) (string, bool) {
	if !utf8.Valid(data) {
		return "", false
	}
	s := string(bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF")))
	var out strings.Builder
	cells := 0 // cells already written in the current table row
	text := func(t string) {
		t = html.UnescapeString(t)
		t = strings.Join(strings.Fields(t), " ")
		if t == "" {
			return
		}
		if out.Len() > 0 {
			last := out.String()[out.Len()-1]
			if last != '\n' && last != ' ' {
				out.WriteByte(' ')
			}
		}
		out.WriteString(t)
	}
	// raw text is gathered until a real markup construct (tag, comment,
	// declaration) ends it, so a lone "<" never adds a separator: the output
	// stays shorter than the input.
	var raw strings.Builder
	flush := func() {
		if raw.Len() > 0 {
			text(raw.String())
			raw.Reset()
		}
	}
	newline := func() {
		flush()
		if out.Len() > 0 && out.String()[out.Len()-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	for i := 0; i < len(s); {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			raw.WriteString(s[i:])
			break
		}
		raw.WriteString(s[i : i+lt])
		i += lt
		rest := s[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest[4:], "-->")
			if end < 0 {
				i = len(s)
			} else {
				i += 4 + end + 3
			}
			continue
		case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				i = len(s)
			} else {
				i += end + 1
			}
			continue
		}
		end := tagEnd(rest)
		if end < 0 {
			raw.WriteString(rest) // a lone "<" is text
			break
		}
		closing, name := tagName(rest[1:end])
		i += end + 1
		if name == "" {
			raw.WriteString(rest[:end+1])
			continue
		}
		if !closing && htmlSkipped[name] {
			i += skipToEndTag(s[i:], name)
			continue
		}
		switch name {
		case "td", "th":
			if !closing {
				flush()
				if cells > 0 {
					out.WriteString(" | ")
				}
				cells++
			}
		case "li":
			if !closing {
				newline()
				out.WriteString("- ")
			}
		default:
			if htmlBlocks[name] {
				newline()
				if name == "tr" {
					cells = 0
				}
			}
			// any other tag is inline ("S<b>ão</b>" is "São"): no separator
		}
	}
	flush()
	var lines []string
	for _, l := range strings.Split(out.String(), "\n") {
		if l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), "|")); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n"), true
}

// skipToEndTag returns how many bytes of s to skip to pass the end tag of name
// ("</script>"), compared case-insensitively in place (no copy of the rest of
// the document, so many such elements stay linear); all of s when none.
func skipToEndTag(s, name string) int {
	for i := 0; ; {
		j := strings.Index(s[i:], "</")
		if j < 0 {
			return len(s)
		}
		i += j + 2
		if len(s)-i >= len(name) && strings.EqualFold(s[i:i+len(name)], name) {
			k := strings.IndexByte(s[i:], '>')
			if k < 0 {
				return len(s)
			}
			return i + k + 1
		}
	}
}

// tagEnd finds the ">" that closes the tag starting at s[0] ("<"), skipping
// quoted attribute values; -1 when there is none.
func tagEnd(s string) int {
	var quote byte
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i
		}
	}
	return -1
}

// tagName returns whether the tag is a closing one and its lower-case name;
// "" when the text after "<" is not a tag name.
func tagName(inner string) (closing bool, name string) {
	if strings.HasPrefix(inner, "/") {
		closing, inner = true, inner[1:]
	}
	n := 0
	for n < len(inner) {
		c := inner[n]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9' && n > 0) || (c == ':' && n > 0) || (c == '-' && n > 0) {
			n++
			continue
		}
		break
	}
	if n == 0 {
		return false, ""
	}
	return closing, strings.ToLower(inner[:n])
}
