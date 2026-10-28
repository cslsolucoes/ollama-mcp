package doctext

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DOCX list numbering (brief sections 1.3 and 4.1). The number of a clause is
// not in any w:t: it is drawn by Word from w:numPr (on the paragraph or
// inherited from its style) plus word/numbering.xml. This file rebuilds it.
//
// Counters are kept per (resolved abstractNum, level). A level restarts when a
// lower-numbered level is incremented (honoring w:lvlRestart). Whatever cannot
// be resolved yields "" or "- ": numbering never fails the document.
//
// Documented simplifications:
//   - "ordinal" renders "1º", "2º"... for every language (Word draws it by the
//     document language; pt-BR is the use case);
//   - w:startOverride (and a level redefined inside w:lvlOverride) is applied when
//     its w:num is first used, for all of its levels at once;
//   - a missing w:start means 0, as the specification says (Word-written
//     documents always carry it).

func init() { numbererLoader = loadNumbering }

const maxStyleHops = 32

// maxLvlTextRunes bounds w:lvlText. Word's own dialog accepts far less; with
// at most 9 "%n" of at most 64 runes each, a prefix stays under ~700 runes.
const maxLvlTextRunes = 128

type numLevel struct {
	start       int
	format      string
	text        string
	suff        string
	restart     int
	hasRestart  bool
	isLgl       bool
	pStyle      string
	hasFormatEl bool
}

type numAbstract struct {
	levels       [9]*numLevel
	styleLink    string
	numStyleLink string
}

type numInstance struct {
	abstractID string
	startOver  [9]*int
	overLvl    [9]*numLevel
}

type numStyle struct {
	basedOn, numID, ilvl string
}

type numCounters struct {
	cnt  [9]int
	seen [9]bool
}

type docxNumbering struct {
	abstract  map[string]*numAbstract
	nums      map[string]*numInstance
	styles    map[string]*numStyle
	linkOwner map[string]string // styleLink -> abstractNum id
	counters  map[string]*numCounters
	applied   map[string]bool
}

// loadNumbering reads word/numbering.xml and the numbering part of
// word/styles.xml. It returns nil (no prefixes at all) when there is no
// numbering.xml or it cannot be parsed.
func loadNumbering(pkg *Package, lim Limits) paragraphNumberer {
	if !pkg.Has("word/numbering.xml") {
		return nil
	}
	data, err := pkg.Read("word/numbering.xml")
	if err != nil {
		return nil
	}
	n := &docxNumbering{
		abstract:  map[string]*numAbstract{},
		nums:      map[string]*numInstance{},
		styles:    map[string]*numStyle{},
		linkOwner: map[string]string{},
		counters:  map[string]*numCounters{},
		applied:   map[string]bool{},
	}
	if err := parseNumbering(data, lim, n); err != nil {
		return nil
	}
	if pkg.Has("word/styles.xml") {
		if sd, err := pkg.Read("word/styles.xml"); err == nil {
			// A broken styles part only loses style-inherited numbering.
			if err := parseNumStyles(sd, lim, n); err != nil {
				n.styles = map[string]*numStyle{}
			}
		}
	}
	return n
}

func wVal(se xml.StartElement) string {
	v, _ := AttrW(se, "val")
	return v
}

func atoiClamp(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	if v > 1_000_000_000 {
		v = 1_000_000_000
	}
	if v < -1_000_000_000 {
		v = -1_000_000_000
	}
	return v, true
}

func parseNumbering(data []byte, lim Limits, nb *docxNumbering) error {
	dec := NewDecoder(bytes.NewReader(data), lim)
	var (
		curAbs   *numAbstract
		curAbsID string
		curNum   *numInstance
		curLvl   *numLevel
		ovIdx    = -1
		maxItems = 100000
	)
	for {
		tok, err := dec.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !isWNS(t.Name.Space) {
				continue
			}
			maxItems--
			if maxItems < 0 {
				return refuse(ReasonMalformed)
			}
			switch t.Name.Local {
			case "abstractNum":
				id, _ := AttrW(t, "abstractNumId")
				curAbs, curAbsID = &numAbstract{}, id
				nb.abstract[id] = curAbs
			case "styleLink":
				if curAbs != nil {
					curAbs.styleLink = wVal(t)
					if curAbs.styleLink != "" {
						nb.linkOwner[curAbs.styleLink] = curAbsID
					}
				}
			case "numStyleLink":
				if curAbs != nil {
					curAbs.numStyleLink = wVal(t)
				}
			case "lvl":
				idx, ok := atoiClamp(func() string { v, _ := AttrW(t, "ilvl"); return v }())
				l := &numLevel{}
				curLvl = l // a detached level when the index is unusable
				if ok && idx >= 0 && idx < 9 {
					if curNum != nil && ovIdx >= 0 {
						curNum.overLvl[ovIdx] = l
					} else if curAbs != nil && curNum == nil {
						curAbs.levels[idx] = l
					}
				}
			case "start":
				if curLvl != nil {
					if v, ok := atoiClamp(wVal(t)); ok {
						curLvl.start = v
					}
				}
			case "numFmt":
				if curLvl != nil {
					curLvl.format = wVal(t)
				}
			case "lvlText":
				if curLvl != nil {
					// The prefix is copied into EVERY numbered paragraph, so its
					// size is an amplification factor: a level text longer than
					// any real one ("CLÁUSULA %1ª", "%1.%2.%3.") is not drawn
					// and the paragraph gets the bullet fallback.
					if v := wVal(t); utf8.RuneCountInString(v) <= maxLvlTextRunes {
						curLvl.text = v
					} else {
						curLvl.text, curLvl.format = "", "bullet"
					}
				}
			case "suff":
				if curLvl != nil {
					curLvl.suff = wVal(t)
				}
			case "lvlRestart":
				if curLvl != nil {
					if v, ok := atoiClamp(wVal(t)); ok {
						curLvl.restart, curLvl.hasRestart = v, true
					}
				}
			case "isLgl":
				if curLvl != nil {
					switch strings.ToLower(wVal(t)) {
					case "0", "false", "off":
					default:
						curLvl.isLgl = true
					}
				}
			case "pStyle":
				if curLvl != nil {
					curLvl.pStyle = wVal(t)
				}
			case "num":
				id, _ := AttrW(t, "numId")
				curNum = &numInstance{}
				nb.nums[id] = curNum
			case "abstractNumId":
				if curNum != nil {
					curNum.abstractID = wVal(t)
				}
			case "lvlOverride":
				ovIdx = -1
				if v, ok := atoiClamp(func() string { s, _ := AttrW(t, "ilvl"); return s }()); ok && v >= 0 && v < 9 {
					ovIdx = v
				}
			case "startOverride":
				if curNum != nil && ovIdx >= 0 {
					if v, ok := atoiClamp(wVal(t)); ok {
						curNum.startOver[ovIdx] = &v
					}
				}
			}
		case xml.EndElement:
			if !isWNS(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "abstractNum":
				curAbs = nil
			case "lvl":
				curLvl = nil
			case "num":
				curNum = nil
			case "lvlOverride":
				ovIdx = -1
			}
		}
	}
}

// parseNumStyles reads, for each w:style, its w:basedOn and the numbering of
// its own paragraph properties (w:style/w:pPr/w:numPr). Table-style
// conditional formatting (w:tblStylePr) is deliberately out of reach: only
// children of the w:style element itself are read, found by element depth.
func parseNumStyles(data []byte, lim Limits, nb *docxNumbering) error {
	dec := NewDecoder(bytes.NewReader(data), lim)
	var (
		cur       *numStyle
		styleDep  int
		inStylePP bool
	)
	for {
		tok, err := dec.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !isWNS(t.Name.Space) {
				continue
			}
			d := dec.Depth()
			switch t.Name.Local {
			case "style":
				id, _ := AttrW(t, "styleId")
				cur = &numStyle{}
				styleDep = d
				if id != "" {
					nb.styles[id] = cur
				}
			case "basedOn":
				if cur != nil && d == styleDep+1 {
					cur.basedOn = wVal(t)
				}
			case "pPr":
				inStylePP = cur != nil && d == styleDep+1
			case "numId":
				if cur != nil && inStylePP && d == styleDep+3 {
					cur.numID = wVal(t)
				}
			case "ilvl":
				if cur != nil && inStylePP && d == styleDep+3 {
					cur.ilvl = wVal(t)
				}
			}
		case xml.EndElement:
			if !isWNS(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "style":
				cur = nil
				inStylePP = false
			case "pPr":
				inStylePP = false
			}
		}
	}
}

// styleNumbering resolves the numId and ilvl a style gives, following basedOn
// for at most maxStyleHops hops (a cycle just ends the walk).
func (n *docxNumbering) styleNumbering(id string) (numID, ilvl string) {
	cur := id
	for hops := 0; cur != "" && hops < maxStyleHops; hops++ {
		s := n.styles[cur]
		if s == nil {
			break
		}
		if numID == "" {
			numID = s.numID
		}
		if ilvl == "" {
			ilvl = s.ilvl
		}
		if numID != "" && ilvl != "" {
			break
		}
		cur = s.basedOn
	}
	return numID, ilvl
}

// resolveAbstract follows numStyleLink (an abstractNum defined by a numbering
// style) to the abstractNum that owns that style link.
func (n *docxNumbering) resolveAbstract(id string) (string, *numAbstract) {
	for hops := 0; hops < 8; hops++ {
		a := n.abstract[id]
		if a == nil {
			return id, nil
		}
		if a.numStyleLink == "" {
			return id, a
		}
		owner, ok := n.linkOwner[a.numStyleLink]
		if !ok || owner == id {
			return id, a
		}
		id = owner
	}
	return id, n.abstract[id]
}

// Prefix implements paragraphNumberer.
func (n *docxNumbering) Prefix(styleID, numID, ilvl string) (out string) {
	defer func() {
		if recover() != nil {
			out = ""
		}
	}()
	if numID == "" || ilvl == "" {
		sn, sl := n.styleNumbering(styleID)
		if numID == "" {
			numID = sn
		}
		if ilvl == "" {
			ilvl = sl
		}
	}
	if numID == "" || numID == "0" {
		return ""
	}
	inst := n.nums[numID]
	if inst == nil {
		return ""
	}
	absID, abs := n.resolveAbstract(inst.abstractID)
	if abs == nil {
		return ""
	}
	idx := 0
	if v, ok := atoiClamp(ilvl); ok && ilvl != "" {
		if v < 0 {
			v = 0
		}
		if v > 8 {
			v = 8
		}
		idx = v
	} else if styleID != "" {
		// No explicit level: the level that is linked to the style, if any.
		for i, l := range abs.levels {
			if l != nil && l.pStyle == styleID {
				idx = i
				break
			}
		}
	}

	level := func(i int) *numLevel {
		if inst.overLvl[i] != nil {
			return inst.overLvl[i]
		}
		return abs.levels[i]
	}

	st := n.counters[absID]
	if st == nil {
		st = &numCounters{}
		n.counters[absID] = st
	}
	if !n.applied[numID] {
		n.applied[numID] = true
		for l := 0; l < 9; l++ {
			startAt, restart := 0, false
			switch {
			case inst.startOver[l] != nil:
				startAt, restart = *inst.startOver[l], true
			case inst.overLvl[l] != nil:
				// A level redefined by the w:num restarts that level too.
				startAt, restart = inst.overLvl[l].start, true
			}
			if restart {
				st.cnt[l] = startAt - 1
				st.seen[l] = true
				for j := l + 1; j < 9; j++ {
					st.seen[j] = false
				}
			}
		}
	}
	def := level(idx)
	if def == nil {
		return "- "
	}
	// Advance this level and restart the ones below it.
	if !st.seen[idx] {
		st.cnt[idx] = def.start
		st.seen[idx] = true
	} else {
		st.cnt[idx]++
	}
	for j := idx + 1; j < 9; j++ {
		d := level(j)
		restart := true
		if d != nil && d.hasRestart {
			restart = d.restart != 0 && idx+1 <= d.restart
		}
		if restart {
			st.seen[j] = false
		}
	}

	format := def.format
	if format == "" {
		format = "decimal"
	}
	if format == "bullet" || !knownNumFormat(format) {
		return "- "
	}
	value := func(l int) int {
		if st.seen[l] {
			return st.cnt[l]
		}
		if d := level(l); d != nil {
			return d.start
		}
		return 1
	}
	var sb strings.Builder
	rs := []rune(def.text)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '%' && i+1 < len(rs) && rs[i+1] >= '1' && rs[i+1] <= '9' {
			l := int(rs[i+1] - '1')
			f := "decimal"
			if d := level(l); d != nil && !def.isLgl {
				f = d.format
			}
			switch {
			case f == "none":
			case !knownNumFormat(f):
				sb.WriteString(formatNumber("decimal", value(l)))
			default:
				sb.WriteString(formatNumber(f, value(l)))
			}
			i++
			continue
		}
		sb.WriteRune(rs[i])
	}
	text := sb.String()
	if text == "" {
		return ""
	}
	switch def.suff {
	case "space":
		return text + " "
	case "nothing":
		return text
	default: // "tab" or absent
		return text + "\t"
	}
}

func knownNumFormat(f string) bool {
	switch f {
	case "decimal", "lowerLetter", "upperLetter", "lowerRoman", "upperRoman", "ordinal", "none":
		return true
	}
	return false
}

// formatNumber renders n in a known format. A value a format cannot draw
// (zero or negative letters and numerals, absurd sizes) falls back to decimal.
func formatNumber(format string, n int) string {
	switch format {
	case "lowerLetter", "upperLetter":
		if n < 1 || n > 26*64 {
			return strconv.Itoa(n)
		}
		base := 'a'
		if format == "upperLetter" {
			base = 'A'
		}
		// Word repeats the letter: 27 is "aa", 28 is "bb".
		return strings.Repeat(string(base+rune((n-1)%26)), (n-1)/26+1)
	case "lowerRoman":
		if s, ok := roman(n); ok {
			return strings.ToLower(s)
		}
		return strconv.Itoa(n)
	case "upperRoman":
		if s, ok := roman(n); ok {
			return s
		}
		return strconv.Itoa(n)
	case "ordinal":
		return strconv.Itoa(n) + "º"
	default:
		return strconv.Itoa(n)
	}
}

func roman(n int) (string, bool) {
	if n < 1 || n > 3999 {
		return "", false
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var sb strings.Builder
	for i, v := range vals {
		for n >= v {
			sb.WriteString(syms[i])
			n -= v
		}
	}
	return sb.String(), true
}
