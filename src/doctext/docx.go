package doctext

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf8"
)

// DOCX extraction (brief sections 1 and 4.1). The XML is read as a stream of
// tokens and never built into a tree. The walker keeps small stacks (open
// paragraphs, table cells, fields, markup-compatibility choices) and writes
// lines into a sink: the document text at the root, or the cell of a table.
//
// List numbering (w:numPr -> numbering.xml) is NOT in this file: it is behind
// paragraphNumberer, installed by docx_numbering.go. Without it (or without a
// numbering.xml) no paragraph gets a prefix.

const (
	nsMath       = "http://schemas.openxmlformats.org/officeDocument/2006/math"
	nsMathStrict = "http://purl.oclc.org/ooxml/officeDocument/math"

	labelHeader   = "[cabeçalho]"
	labelFooter   = "[rodapé]"
	labelPageDiv  = "[quebra de página]"
	labelFootnote = "nota de rodapé"
	labelEndnote  = "nota de fim"
	labelComment  = "[comentário]"
)

// paragraphNumberer turns the numbering properties of a paragraph into the
// list label that Word would draw in front of it ("5.2.\t", "a) ", "- "...).
// It must never fail: a paragraph it cannot resolve gets "". styleID is the
// w:pStyle of the paragraph, numID and ilvl are the direct w:numPr values
// (empty when absent). Counters advance on every call, so it is called exactly
// once per paragraph, in document order.
type paragraphNumberer interface {
	Prefix(styleID, numID, ilvl string) string
}

// numbererLoader builds the numberer of a package; nil when the package has no
// numbering. unavailable is true when the package HAS numbering that could not
// be read (broken or over the limit): the text then goes out without list
// numbers and the footer says so. docx_numbering.go installs it in init().
var numbererLoader func(pkg *Package, lim Limits) (num paragraphNumberer, unavailable bool)

// docxDoc is the state shared by every part of one document, and by the DOCX
// imported through w:altChunk (depth 1).
type docxDoc struct {
	ctx    context.Context
	pkg    *Package
	lim    Limits
	total  *int
	styles *docxHiddenStyles // nil: no styles.xml, or it could not be read
	depth  int               // 0: the document; 1: a DOCX imported by w:altChunk
	rels   map[string]map[string]Relationship

	imported, unread, hidden bool
	// flags of an imported DOCX that the parts of this one do not carry
	importPending, importNumUnavailable bool
}

// isWNS reports whether space is a WordprocessingML namespace (either dialect).
func isWNS(space string) bool { return inNS(space, NSWordprocessing, NSWordprocessingStrict) }

// docxRevisionSkip are the revision elements whose whole subtree is dropped
// (the "accepted" state keeps insertions and drops deletions and old
// properties). Every element here marks the document as having pending
// revisions, except w:del and w:moveFrom, which do so only when they hold
// content (see docxRevisionContent).
var docxRevisionSkip = map[string]bool{
	"del": true, "moveFrom": true,
	"pPrChange": true, "rPrChange": true, "sectPrChange": true, "tblPrChange": true,
	"tblPrExChange": true, "trPrChange": true, "tcPrChange": true, "tblGridChange": true,
	"numberingChange": true, "cellDel": true, "cellIns": true, "cellMerge": true,
}

// docxRevisionContent are the elements that make a w:ins, w:del, w:moveFrom
// or w:moveTo a real revision (runs or text). An empty marker, such as the
// <w:ins/> Word leaves inside w:numPr of a template paragraph, is not counted:
// Word itself reports 0 revisions for it (P7.7-3).
var docxRevisionContent = map[string]bool{
	"r": true, "t": true, "delText": true, "instrText": true, "delInstrText": true,
}

type docxPara struct {
	buf                  strings.Builder
	hid                  strings.Builder // hidden text (w:vanish) not yet written
	inHid                bool            // the text being gathered is hidden
	styleID, numID, ilvl string
	prefix               string
	prefixDone           bool
	emitted              bool
}

// docxRun is the run property state that decides whether its text is hidden.
type docxRun struct {
	vanish *bool  // direct w:rPr/w:vanish, nil when absent
	style  string // w:rStyle
}

type docxCell struct {
	parts []string
	cont  bool // vMerge continuation: content is not repeated
	drop  bool // hMerge continuation: the cell is absorbed by its left neighbour
}

type docxTable struct{ row []string }

type docxNote struct {
	kind, id, author string
	skip             bool
}

// docxPart is what one XML part yields.
type docxPart struct {
	text                 string
	pending              bool
	headerIDs, footerIDs []string
}

type docxWalker struct {
	doc      *docxDoc
	part     string // name of the part being walked (its .rels resolve w:altChunk)
	dec      *Decoder
	total    *int // runes written so far, shared by every part of the document
	maxRunes int
	num      paragraphNumberer
	notes    bool // notes/comments part: w:footnote, w:endnote, w:comment are blocks

	runs    []docxRun // open w:r, innermost last
	inRunPr bool      // inside the w:rPr of a run

	out       strings.Builder
	lastBlank bool
	wrote     bool
	cellRunes int // runes held by open cells/notes, already charged against maxRunes

	skip    int   // >0 while inside a dropped subtree (counts open elements)
	skipRev bool  // the dropped subtree is a w:del/w:moveFrom: content in it is a pending revision
	ins     int   // open w:ins/w:moveTo: content in them is a pending revision
	ac      []int // per open mc:AlternateContent: branches already taken
	fields  []bool
	code    int // fields currently in their code part
	paras   []*docxPara
	cells   []*docxCell
	tables  []*docxTable
	note    *docxNote

	inPPr, inRun int
	inNumPr      bool
	inT          bool

	pending              bool
	headerIDs, footerIDs []string
	err                  error
}

func (h *docxWalker) cur() *docxPara {
	if n := len(h.paras); n > 0 {
		return h.paras[n-1]
	}
	return nil
}

func (h *docxWalker) appendText(s string) {
	if h.code > 0 {
		return
	}
	p := h.cur()
	if p == nil {
		return
	}
	if h.runHidden(p) {
		if !p.inHid {
			h.flushPara(p, false) // the visible text before it keeps its place
			p.inHid = true
		}
		p.hid.WriteString(s)
		return
	}
	if p.inHid {
		h.flushHidden(p)
	}
	p.buf.WriteString(s)
}

// runHidden reports whether the innermost open run is hidden: its direct
// w:vanish, else its character style, else the paragraph style, else the
// document defaults (styles follow w:basedOn).
func (h *docxWalker) runHidden(p *docxPara) bool {
	if len(h.runs) == 0 {
		return false
	}
	r := h.runs[len(h.runs)-1]
	if r.vanish != nil {
		return *r.vanish
	}
	st := h.doc.styles
	if st == nil {
		return false
	}
	if r.style != "" {
		if v, ok := st.vanish(r.style); ok {
			return v
		}
	}
	if p.styleID != "" {
		if v, ok := st.vanish(p.styleID); ok {
			return v
		}
	}
	return st.def
}

// flushHidden writes the hidden text gathered so far as "[oculto] ..." lines,
// one per line of the text. A numbered paragraph whose text is all hidden
// carries its number on the hidden line.
func (h *docxWalker) flushHidden(p *docxPara) {
	text := strings.TrimSpace(p.hid.String())
	p.hid.Reset()
	p.inHid = false
	if text == "" {
		return
	}
	if p.prefix != "" && !p.prefixDone && !p.emitted {
		text = p.prefix + text
		p.prefixDone = true
	}
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			h.emit(labelHidden + " " + l)
		}
	}
	p.emitted = true
	h.doc.hidden = true
}

// flushAll ends what a paragraph gathered: hidden text first (it always comes
// after the visible text already written), then the visible text.
func (h *docxWalker) flushAll(p *docxPara, final bool) {
	if p.inHid {
		h.flushHidden(p)
	}
	h.flushPara(p, final)
}

// emit writes one line to the innermost sink: a table cell (or note block)
// collects it, the root appends it to the text.
func (h *docxWalker) emit(s string) {
	if n := len(h.cells); n > 0 {
		if f := strings.Join(strings.Fields(s), " "); f != "" {
			// A cell (or a note block) only reaches the text when its row
			// ends: charge it now, or a single row could grow without bound.
			h.cellRunes += utf8.RuneCountInString(f) + 1
			if *h.total+h.cellRunes > h.maxRunes {
				h.err = refuse(ReasonTooLong)
				return
			}
			c := h.cells[n-1]
			c.parts = append(c.parts, f)
		}
		return
	}
	if h.err != nil {
		return
	}
	// What the cells already charged is part of this line.
	if d := utf8.RuneCountInString(s) + 1 - h.cellRunes; d > 0 {
		*h.total += d
	}
	h.cellRunes = 0
	if *h.total > h.maxRunes {
		h.err = refuse(ReasonTooLong)
		return
	}
	h.out.WriteString(s)
	h.out.WriteByte('\n')
	h.lastBlank = s == ""
	h.wrote = true
}

// flushPara writes the text gathered so far. final is the end of the paragraph:
// then an empty paragraph becomes an empty line (unless it already wrote one).
func (h *docxWalker) flushPara(p *docxPara, final bool) {
	text := strings.TrimRight(p.buf.String(), " \t\n")
	if p.emitted {
		// a continuation line (after hidden text, a page break or a text
		// box) does not start with the space that separated it
		text = strings.TrimLeft(text, " ")
	}
	p.buf.Reset()
	if p.prefix != "" && !p.prefixDone && (text != "" || final) {
		text = strings.TrimRight(p.prefix+text, " \t")
		p.prefixDone = true
	}
	if text != "" {
		h.emit(text)
		p.emitted = true
		return
	}
	if final && !p.emitted {
		h.emit("")
		p.emitted = true
	}
}

// pageBreak is a manual page break: the paragraph text before it is flushed,
// the marker is its own line, and the rest of the paragraph follows.
func (h *docxWalker) pageBreak() {
	if p := h.cur(); p != nil {
		h.flushAll(p, false)
		p.emitted = true
	}
	h.emit(labelPageDiv)
}

func (h *docxWalker) start(se xml.StartElement) {
	if h.skip > 0 {
		if h.skipRev && isWNS(se.Name.Space) && docxRevisionContent[se.Name.Local] {
			h.pending = true // a deletion or move-from that removes real content
		}
		h.skip++
		return
	}
	n := se.Name
	switch {
	case isWNS(n.Space):
		h.startW(se)
	case n.Space == NSMarkupCompat:
		switch n.Local {
		case "AlternateContent":
			h.ac = append(h.ac, 0)
		case "Choice", "Fallback":
			// Only the first branch is read (Choice, or Fallback when there is
			// no Choice): both carry the same text.
			if k := len(h.ac); k > 0 {
				if h.ac[k-1] > 0 {
					h.skip = 1
				} else {
					h.ac[k-1]++
				}
			}
		}
	case n.Local == "t" && inNS(n.Space, nsMath, nsMathStrict):
		h.inT = true
	}
}

func (h *docxWalker) startW(se xml.StartElement) {
	local := se.Name.Local
	if docxRevisionSkip[local] {
		if local == "del" || local == "moveFrom" {
			h.skipRev = true // pending only if the subtree holds content
		} else {
			h.pending = true // property changes count as they are
		}
		h.skip = 1
		return
	}
	if h.ins > 0 && docxRevisionContent[local] {
		h.pending = true // an insertion or move-to that adds real content
	}
	switch local {
	case "ins", "moveTo":
		h.ins++
	case "p":
		h.paras = append(h.paras, &docxPara{})
	case "pPr":
		h.inPPr++
	case "pStyle":
		if p := h.cur(); p != nil && h.inPPr > 0 {
			p.styleID, _ = AttrW(se, "val")
		}
	case "numPr":
		if h.inPPr > 0 {
			h.inNumPr = true
		}
	case "ilvl":
		if p := h.cur(); p != nil && h.inNumPr {
			p.ilvl, _ = AttrW(se, "val")
		}
	case "numId":
		if p := h.cur(); p != nil && h.inNumPr {
			p.numID, _ = AttrW(se, "val")
		}
	case "r":
		h.inRun++
		h.runs = append(h.runs, docxRun{})
	case "rPr":
		if h.inRun > 0 && h.inPPr == 0 {
			h.inRunPr = true
		}
	case "vanish":
		if h.inRunPr && len(h.runs) > 0 {
			v := onOff(se)
			h.runs[len(h.runs)-1].vanish = &v
		}
	case "rStyle":
		if h.inRunPr && len(h.runs) > 0 {
			h.runs[len(h.runs)-1].style, _ = AttrW(se, "val")
		}
	case "altChunk":
		if id, ok := AttrR(se, "id"); ok {
			h.altChunk(id)
		} else {
			h.doc.unread = true
		}
	case "t":
		h.inT = true
	case "tab", "ptab":
		if h.inRun > 0 && h.inPPr == 0 {
			h.appendText("\t")
		}
	case "br":
		if h.inRun > 0 && h.inPPr == 0 {
			if typ, _ := AttrW(se, "type"); typ == "page" {
				h.pageBreak()
			} else {
				h.appendText("\n")
			}
		}
	case "cr":
		if h.inRun > 0 {
			h.appendText("\n")
		}
	case "noBreakHyphen":
		h.appendText("-")
	case "fldChar":
		typ, _ := AttrW(se, "fldCharType")
		switch typ {
		case "begin":
			h.fields = append(h.fields, true)
			h.code++
		case "separate":
			if k := len(h.fields); k > 0 && h.fields[k-1] {
				h.fields[k-1] = false
				h.code--
			}
		case "end":
			if k := len(h.fields); k > 0 {
				if h.fields[k-1] {
					h.code--
				}
				h.fields = h.fields[:k-1]
			}
		}
	case "footnoteReference":
		if id, _ := AttrW(se, "id"); id != "" {
			h.appendText("[" + cleanInline(id, 32) + "]")
		}
	case "endnoteReference":
		if id, _ := AttrW(se, "id"); id != "" {
			h.appendText("[fim " + cleanInline(id, 32) + "]")
		}
	case "txbxContent":
		// Keep the reading order: the host paragraph's text so far comes
		// before the text box.
		if p := h.cur(); p != nil {
			h.flushAll(p, false)
			p.emitted = true // a host holding only a box must not add a blank line
		}
	case "tbl":
		if len(h.cells) == 0 && h.wrote && !h.lastBlank {
			h.emit("")
		}
		h.tables = append(h.tables, &docxTable{})
	case "tr":
		if k := len(h.tables); k > 0 {
			h.tables[k-1].row = nil
		}
	case "tc":
		h.cells = append(h.cells, &docxCell{})
	case "vMerge":
		if k := len(h.cells); k > 0 {
			if v, ok := AttrW(se, "val"); !ok || v == "continue" {
				h.cells[k-1].cont = true
			}
		}
	case "hMerge":
		if k := len(h.cells); k > 0 {
			if v, ok := AttrW(se, "val"); !ok || v == "continue" {
				h.cells[k-1].drop = true
			}
		}
	case "headerReference":
		if id, ok := AttrR(se, "id"); ok {
			h.headerIDs = append(h.headerIDs, id)
		}
	case "footerReference":
		if id, ok := AttrR(se, "id"); ok {
			h.footerIDs = append(h.footerIDs, id)
		}
	case "footnote", "endnote", "comment":
		if h.notes && h.note == nil {
			nt := &docxNote{kind: local}
			nt.id, _ = AttrW(se, "id")
			nt.author, _ = AttrW(se, "author")
			if typ, _ := AttrW(se, "type"); typ != "" && typ != "normal" {
				nt.skip = true // separator, continuationSeparator, continuationNotice
			}
			h.note = nt
			h.cells = append(h.cells, &docxCell{}) // the block collects its lines like a cell
		}
	}
}

func (h *docxWalker) end(ee xml.EndElement) {
	if h.skip > 0 {
		h.skip--
		if h.skip == 0 {
			h.skipRev = false
		}
		return
	}
	n := ee.Name
	switch {
	case isWNS(n.Space):
		h.endW(n.Local)
	case n.Space == NSMarkupCompat:
		if n.Local == "AlternateContent" && len(h.ac) > 0 {
			h.ac = h.ac[:len(h.ac)-1]
		}
	case n.Local == "t" && inNS(n.Space, nsMath, nsMathStrict):
		h.inT = false
	}
}

func (h *docxWalker) endW(local string) {
	switch local {
	case "ins", "moveTo":
		if h.ins > 0 {
			h.ins--
		}
	case "p":
		if k := len(h.paras); k > 0 {
			p := h.paras[k-1]
			h.paras = h.paras[:k-1]
			h.flushAll(p, true)
		}
	case "rPr":
		h.inRunPr = false
	case "pPr":
		if h.inPPr > 0 {
			h.inPPr--
		}
		if h.inPPr == 0 && h.num != nil {
			if p := h.cur(); p != nil && (p.styleID != "" || p.numID != "") {
				p.prefix = h.num.Prefix(p.styleID, p.numID, p.ilvl)
			}
		}
	case "numPr":
		h.inNumPr = false
	case "r":
		if h.inRun > 0 {
			h.inRun--
		}
		if k := len(h.runs); k > 0 {
			h.runs = h.runs[:k-1]
		}
	case "t":
		h.inT = false
	case "tbl":
		if k := len(h.tables); k > 0 {
			h.tables = h.tables[:k-1]
		}
		if len(h.cells) == 0 {
			h.emit("")
		}
	case "tr":
		if k := len(h.tables); k > 0 {
			t := h.tables[k-1]
			nonEmpty := false
			for _, c := range t.row {
				if c != "" {
					nonEmpty = true
					break
				}
			}
			if nonEmpty {
				h.emit(strings.Join(t.row, " | "))
			}
			t.row = nil
		}
	case "tc":
		if k := len(h.cells); k > 0 {
			c := h.cells[k-1]
			h.cells = h.cells[:k-1]
			if t := len(h.tables); t > 0 && !c.drop {
				text := ""
				if !c.cont {
					text = strings.Join(c.parts, " ")
				}
				h.tables[t-1].row = append(h.tables[t-1].row, text)
			}
		}
	case "footnote", "endnote", "comment":
		if h.note != nil && h.note.kind == local && len(h.cells) > 0 {
			nt := h.note
			c := h.cells[len(h.cells)-1]
			h.cells = h.cells[:len(h.cells)-1]
			h.note = nil
			text := strings.Join(c.parts, " ")
			if nt.skip || text == "" {
				return
			}
			switch nt.kind {
			case "footnote":
				h.emit("[" + labelFootnote + " " + cleanInline(nt.id, 32) + "] " + text)
			case "endnote":
				h.emit("[" + labelEndnote + " " + cleanInline(nt.id, 32) + "] " + text)
			default:
				if a := cleanInline(nt.author, 200); a != "" {
					h.emit(labelComment + " " + a + ": " + text)
				} else {
					h.emit(labelComment + " " + text)
				}
			}
		}
	}
}

// cleanInline makes an attribute value safe for a single output line.
func cleanInline(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}

// parseDocxPart walks one XML part of the document.
func parseDocxPart(doc *docxDoc, name string, num paragraphNumberer, notes bool) (*docxPart, error) {
	if err := ctxErr(doc.ctx); err != nil {
		return nil, err
	}
	data, err := doc.pkg.Read(name)
	if err != nil {
		return nil, err
	}
	h := &docxWalker{
		doc:      doc,
		part:     name,
		dec:      NewDecoder(bytes.NewReader(data), doc.lim),
		total:    doc.total,
		maxRunes: doc.lim.MaxTextRunes,
		num:      num,
		notes:    notes,
	}
	for tick := 1; ; tick++ {
		if tick&0x3ff == 0 {
			if err := ctxErr(doc.ctx); err != nil {
				return nil, err
			}
		}
		tok, err := h.dec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			h.start(t)
		case xml.EndElement:
			h.end(t)
		case xml.CharData:
			if h.inT && h.skip == 0 {
				h.appendText(string(t))
			}
		}
		if h.err != nil {
			return nil, h.err
		}
	}
	// A field still in its code part at the end of the part hid everything
	// after its begin (fldChar begin without separate/end, or with them inside
	// a deleted run): refuse instead of returning a silently truncated text.
	if h.code > 0 {
		return nil, refuse(ReasonMalformed)
	}
	return &docxPart{text: h.out.String(), pending: h.pending, headerIDs: h.headerIDs, footerIDs: h.footerIDs}, nil
}

// collapseBlankLines trims trailing blanks of each line, keeps at most two
// empty lines in a row and drops the empty lines at both ends. A non-empty
// result ends with exactly one "\n".
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			blank++
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	text := strings.Trim(strings.Join(out, "\n"), "\n")
	if text != "" {
		text += "\n"
	}
	return text
}

// notesParts are read once, after the body, as their own blocks.
var notesParts = map[string]bool{"word/footnotes.xml": true, "word/endnotes.xml": true, "word/comments.xml": true}

const docxDocPart = "word/document.xml"

// extractDOCX extracts the text of a .docx. Output order: headers (each one
// once), body, footers, footnotes, endnotes, comments.
func extractDOCX(ctx context.Context, data []byte, lim Limits) (*Result, error) {
	lim = lim.withDefaults()
	pkg, err := OpenPackage(data, lim)
	if err != nil {
		return nil, err
	}
	total := 0
	doc := &docxDoc{ctx: ctx, pkg: pkg, lim: lim, total: &total}
	return extractDOCXDoc(doc)
}

// extractDOCXDoc runs the extraction over an open package. A DOCX imported by
// w:altChunk (doc.depth 1) goes through it too, without headers and footers
// (Word imports the content, not the page setup).
func extractDOCXDoc(doc *docxDoc) (*Result, error) {
	pkg := doc.pkg
	if !pkg.Has(docxDocPart) {
		return nil, refuse(ReasonMalformed)
	}
	doc.styles = loadHiddenStyles(pkg, doc.lim)
	var num paragraphNumberer
	numUnavailable := false
	if numbererLoader != nil {
		num, numUnavailable = numbererLoader(pkg, doc.lim)
	}
	body, err := parseDocxPart(doc, docxDocPart, num, false)
	if err != nil {
		return nil, err
	}
	pending := body.pending

	// Headers and footers really used by a section, once each (by target).
	var headers, footers []string
	if doc.depth == 0 && (len(body.headerIDs) > 0 || len(body.footerIDs) > 0) {
		rels, err := pkg.Relationships(docxDocPart)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]string, len(rels))
		for _, r := range rels {
			byID[r.ID] = r.Target
		}
		seen := make(map[string]bool)
		collect := func(ids []string, label string) ([]string, error) {
			var blocks []string
			for _, id := range ids {
				target, ok := byID[id]
				key := strings.ToLower(target)
				// Never a part that is read elsewhere (the body or the notes):
				// a rel pointing there would duplicate it as a header/footer.
				if !ok || seen[key] || !pkg.Has(target) || key == docxDocPart || notesParts[key] {
					continue
				}
				seen[key] = true
				part, err := parseDocxPart(doc, target, nil, false)
				if err != nil {
					return nil, err
				}
				pending = pending || part.pending
				if t := strings.Trim(part.text, " \t\n"); t != "" {
					blocks = append(blocks, label+"\n"+t+"\n")
				}
			}
			return blocks, nil
		}
		if headers, err = collect(body.headerIDs, labelHeader); err != nil {
			return nil, err
		}
		if footers, err = collect(body.footerIDs, labelFooter); err != nil {
			return nil, err
		}
	}

	var blocks []string
	blocks = append(blocks, headers...)
	blocks = append(blocks, body.text)
	blocks = append(blocks, footers...)
	// Footnotes, endnotes and comments form one block, one line per item.
	var notes strings.Builder
	for _, n := range []string{"word/footnotes.xml", "word/endnotes.xml", "word/comments.xml"} {
		if !pkg.Has(n) {
			continue
		}
		part, err := parseDocxPart(doc, n, nil, true)
		if err != nil {
			return nil, err
		}
		pending = pending || part.pending
		notes.WriteString(part.text)
	}
	blocks = append(blocks, notes.String())
	var sb strings.Builder
	for i, b := range blocks {
		if strings.TrimSpace(b) == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteByte('\n') // a blank line between sections
		}
		sb.WriteString(b)
		if !strings.HasSuffix(b, "\n") {
			sb.WriteByte('\n')
		}
	}
	return &Result{
		Text:                 collapseBlankLines(sb.String()),
		PendingRevisions:     pending || doc.importPending,
		ImportedContent:      doc.imported,
		UnreadContent:        doc.unread,
		NumberingUnavailable: numUnavailable || doc.importNumUnavailable,
		HiddenText:           doc.hidden,
	}, nil
}
