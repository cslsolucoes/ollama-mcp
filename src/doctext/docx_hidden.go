package doctext

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// Hidden text of a DOCX (w:vanish, decision of the owner after 7.7a): text the
// document holds but does not show or print. It is extracted, on lines of its
// own labelled "[oculto]", so the reader knows it is not in the printed
// document. A run is hidden by its direct w:vanish, else by its character
// style (w:rStyle), else by the paragraph style, else by the document
// defaults; styles follow w:basedOn (at most maxStyleHops). Simplification:
// w:vanish is a toggle property in Word, and toggles between nested styles
// (a style that hides on top of one that hides) are taken as "hidden", not
// XOR-ed; direct formatting always wins, as in Word.

type docxHiddenStyle struct {
	basedOn string
	vanish  *bool
}

type docxHiddenStyles struct {
	styles map[string]*docxHiddenStyle
	def    bool // w:docDefaults/w:rPrDefault/w:rPr/w:vanish
}

// vanish resolves the w:vanish of a style through its w:basedOn chain; ok is
// false when no style in the chain sets it.
func (s *docxHiddenStyles) vanish(id string) (v bool, ok bool) {
	for hops := 0; id != "" && hops < maxStyleHops; hops++ {
		st := s.styles[id]
		if st == nil {
			return false, false
		}
		if st.vanish != nil {
			return *st.vanish, true
		}
		id = st.basedOn
	}
	return false, false
}

// onOff reads a WordprocessingML on/off property: absent w:val means on.
func onOff(se xml.StartElement) bool {
	v, ok := AttrW(se, "val")
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off":
		return false
	}
	return true
}

// loadHiddenStyles reads the w:vanish of each style of word/styles.xml (only
// w:style/w:rPr/w:vanish and w:basedOn, found by element depth, so table
// style conditional formatting is out of reach). nil when there is no
// styles.xml or it cannot be read: then only direct w:vanish counts.
func loadHiddenStyles(pkg *Package, lim Limits) *docxHiddenStyles {
	if !pkg.Has("word/styles.xml") {
		return nil
	}
	data, err := pkg.Read("word/styles.xml")
	if err != nil {
		return nil
	}
	out := &docxHiddenStyles{styles: map[string]*docxHiddenStyle{}}
	dec := NewDecoder(bytes.NewReader(data), lim)
	var (
		cur      *docxHiddenStyle
		styleDep int
		inRPr    bool
		inDefRPr bool
		defDep   int
	)
	for {
		tok, err := dec.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			return nil
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
				cur, styleDep, inRPr = &docxHiddenStyle{}, d, false
				if id != "" {
					out.styles[id] = cur
				}
			case "basedOn":
				if cur != nil && d == styleDep+1 {
					cur.basedOn = wVal(t)
				}
			case "rPr":
				if cur != nil && d == styleDep+1 {
					inRPr = true
				}
			case "rPrDefault":
				inDefRPr, defDep = true, d
			case "vanish":
				switch {
				case cur != nil && inRPr && d == styleDep+2:
					v := onOff(t)
					cur.vanish = &v
				case cur == nil && inDefRPr && d == defDep+2:
					out.def = onOff(t)
				}
			}
		case xml.EndElement:
			if !isWNS(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "style":
				cur, inRPr = nil, false
			case "rPr":
				inRPr = false
			case "rPrDefault":
				inDefRPr = false
			}
		}
	}
}
