package doctext

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// extractPPTX extracts the text of a .pptx in presentation order (brief
// section 4.2): slides follow p:sldIdLst (never the file names), each one is
// labelled "[slide N]" ("[slide N — oculto]" when hidden) and is followed by
// its speaker notes as "[notas do slide N]". Layouts and masters are never
// read. The output is deterministic.
func extractPPTX(ctx context.Context, data []byte, lim Limits) (res *Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, refuse(ReasonMalformed)
		}
	}()
	lim = lim.withDefaults()
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	pkg, err := OpenPackage(data, lim)
	if err != nil {
		return nil, err
	}
	const presPart = "ppt/presentation.xml"
	if !pkg.Has(presPart) {
		return nil, refuse(ReasonMalformed)
	}
	rids, err := slideRIDs(pkg, presPart, lim)
	if err != nil {
		return nil, err
	}
	rels, err := pkg.Relationships(presPart)
	if err != nil {
		return nil, err
	}
	target := make(map[string]string, len(rels))
	for _, r := range rels {
		if _, dup := target[r.ID]; !dup {
			target[r.ID] = r.Target
		}
	}

	var blocks []string
	total := 0
	for i, rid := range rids {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		n := i + 1
		part, ok := target[rid]
		if !ok || !pkg.Has(part) {
			return nil, refuse(ReasonMalformed)
		}
		xmlData, err := pkg.Read(part)
		if err != nil {
			return nil, err
		}
		lines, hidden, err := parsePPTXPart(xmlData, lim, false)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		b.WriteString("[slide " + strconv.Itoa(n))
		if hidden {
			b.WriteString(" — oculto")
		}
		b.WriteString("]")
		for _, l := range lines {
			b.WriteString("\n" + l)
		}
		srels, err := pkg.Relationships(part)
		if err != nil {
			return nil, err
		}
		for _, r := range srels {
			if !strings.HasSuffix(r.Type, "/notesSlide") || !pkg.Has(r.Target) {
				continue
			}
			nd, err := pkg.Read(r.Target)
			if err != nil {
				return nil, err
			}
			nl, _, err := parsePPTXPart(nd, lim, true)
			if err != nil {
				return nil, err
			}
			if len(nl) > 0 {
				b.WriteString("\n\n[notas do slide " + strconv.Itoa(n) + "]")
				for _, l := range nl {
					b.WriteString("\n" + l)
				}
			}
			break
		}
		block := b.String()
		total += utf8.RuneCountInString(block)
		if total > lim.MaxTextRunes {
			return nil, refuse(ReasonTooLong)
		}
		blocks = append(blocks, block)
	}
	text, err := NormalizeText(strings.Join(blocks, "\n\n"))
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(text) > lim.MaxTextRunes {
		return nil, refuse(ReasonTooLong)
	}
	return &Result{Text: text, Format: FormatPPTX, Slides: len(rids)}, nil
}

// slideRIDs returns the r:id of each p:sldId, in presentation order.
func slideRIDs(pkg *Package, part string, lim Limits) ([]string, error) {
	data, err := pkg.Read(part)
	if err != nil {
		return nil, err
	}
	dec := NewDecoder(bytes.NewReader(data), lim)
	var out []string
	seen := map[string]bool{}
	for {
		tok, err := dec.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok && IsP(se.Name, "sldId") {
			rid, ok := AttrR(se, "id")
			// The same slide listed twice would come out twice, numbered as two
			// slides: PowerPoint repairs such a file, the extraction refuses it.
			if !ok || seen[rid] {
				return nil, refuse(ReasonMalformed)
			}
			seen[rid] = true
			out = append(out, rid)
		}
	}
}

// lineSink collects lines, with blank lines that are only emitted between
// content (never first, never doubled, never trailing).
type lineSink struct {
	lines   []string
	pending bool
}

func (s *lineSink) add(l string) {
	if s.pending && len(s.lines) > 0 {
		s.lines = append(s.lines, "")
	}
	s.pending = false
	s.lines = append(s.lines, l)
}

func (s *lineSink) blank() { s.pending = true }

func (s *lineSink) merge(o *lineSink) {
	for _, l := range o.lines {
		s.add(l)
	}
}

type acState struct{ choiceSeen bool }

// parsePPTXPart reads one slide (notes=false) or notes slide (notes=true) and
// returns its text lines. For notes only the body placeholder is kept.
func parsePPTXPart(data []byte, lim Limits, notes bool) (lines []string, hidden bool, err error) {
	dec := NewDecoder(bytes.NewReader(data), lim)
	main := &lineSink{}
	var shape *lineSink // current p:sp (notes mode only)
	phType := ""
	spDepth := 0

	var para strings.Builder
	inPara, inT := false, false

	tblDepth := 0
	var rows [][]string
	var row []string
	inCell, cellSkip := false, false
	var cellParas []string

	var acs []*acState
	skipTo := -1 // when >= 0: ignoring tokens until an EndElement leaves depth skipTo

	sink := func() *lineSink {
		if shape != nil {
			return shape
		}
		return main
	}

	for {
		tok, e := dec.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, false, e
		}
		if skipTo >= 0 {
			switch tok.(type) {
			case xml.StartElement, xml.EndElement:
				if _, isEnd := tok.(xml.EndElement); isEnd && dec.Depth() == skipTo {
					skipTo = -1
				}
			}
			continue
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := t.Name
			switch {
			case IsMC(n, "AlternateContent"):
				acs = append(acs, &acState{})
			case IsMC(n, "Choice"):
				if len(acs) > 0 {
					a := acs[len(acs)-1]
					if a.choiceSeen {
						skipTo = dec.Depth() - 1
					}
					a.choiceSeen = true
				}
			case IsMC(n, "Fallback"):
				// Read only when no Choice was taken (as the DOCX walker does):
				// an AlternateContent with only a Fallback keeps its text.
				if len(acs) == 0 || acs[len(acs)-1].choiceSeen {
					skipTo = dec.Depth() - 1
				}
			case IsP(n, "sld"):
				if v, ok := AttrPlain(t, "show"); ok && (v == "0" || v == "false") {
					hidden = true
				}
			case IsP(n, "sp"):
				if notes {
					spDepth++
					if spDepth == 1 {
						shape = &lineSink{}
						phType = ""
					}
				}
			case IsP(n, "ph"):
				if notes && shape != nil {
					if v, ok := AttrPlain(t, "type"); ok {
						phType = v
					}
				}
			case IsA(n, "tbl"):
				tblDepth++
				if tblDepth == 1 {
					rows = nil
				}
			case IsA(n, "tr"):
				if tblDepth == 1 {
					row = nil
				}
			case IsA(n, "tc"):
				if tblDepth == 1 {
					inCell, cellSkip, cellParas = true, false, nil
					if v, ok := AttrPlain(t, "hMerge"); ok && (v == "1" || v == "true") {
						cellSkip = true
					}
				}
			case IsA(n, "p"):
				inPara = true
				para.Reset()
			case IsA(n, "br"):
				if inPara {
					para.WriteString("\n")
				}
			case IsA(n, "t"):
				inT = inPara
			}
		case xml.CharData:
			if inT {
				para.Write(t)
			}
		case xml.EndElement:
			n := t.Name
			switch {
			case IsMC(n, "AlternateContent"):
				if len(acs) > 0 {
					acs = acs[:len(acs)-1]
				}
			case IsA(n, "t"):
				inT = false
			case IsA(n, "p"):
				inPara = false
				txt := strings.TrimRight(para.String(), " \t\r\n")
				if strings.TrimSpace(txt) == "" {
					break
				}
				if inCell {
					cellParas = append(cellParas, strings.Join(strings.Fields(txt), " "))
				} else {
					sink().add(txt)
				}
			case IsA(n, "tc"):
				if tblDepth == 1 {
					if !cellSkip {
						row = append(row, strings.Join(cellParas, " "))
					}
					inCell = false
				}
			case IsA(n, "tr"):
				if tblDepth == 1 && len(row) > 0 {
					rows = append(rows, row)
				}
			case IsA(n, "tbl"):
				if tblDepth == 1 {
					s := sink()
					s.blank()
					for _, r := range rows {
						empty := true
						for _, c := range r {
							if c != "" {
								empty = false
							}
						}
						if empty {
							continue
						}
						for len(r) > 0 && r[len(r)-1] == "" {
							r = r[:len(r)-1]
						}
						s.add(strings.Join(r, " | "))
					}
					s.blank()
				}
				if tblDepth > 0 {
					tblDepth--
				}
			case IsP(n, "sp"):
				if notes && spDepth > 0 {
					spDepth--
					if spDepth == 0 {
						if phType == "body" {
							main.merge(shape)
						}
						shape = nil
					}
				}
			}
		}
	}
	return main.lines, hidden, nil
}
