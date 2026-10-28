package doctext

import (
	"encoding/xml"
	"io"
	"unicode/utf8"
)

// Namespace URIs. OOXML comes in two dialects: Transitional (the default of
// Word and PowerPoint) and Strict (purl.oclc.org). Both are accepted; elements
// are always compared by URI, never by prefix.
const (
	NSWordprocessing       = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	NSWordprocessingStrict = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	NSDrawing              = "http://schemas.openxmlformats.org/drawingml/2006/main"
	NSDrawingStrict        = "http://purl.oclc.org/ooxml/drawingml/main"
	NSPresentation         = "http://schemas.openxmlformats.org/presentationml/2006/main"
	NSPresentationStrict   = "http://purl.oclc.org/ooxml/presentationml/main"
	// NSRelationships is the namespace of r:id style attributes in documents.
	NSRelationships       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	NSRelationshipsStrict = "http://purl.oclc.org/ooxml/officeDocument/relationships"
	// NSPackageRels is the namespace of the elements of a .rels part.
	NSPackageRels = "http://schemas.openxmlformats.org/package/2006/relationships"
	// NSMarkupCompat is mc:AlternateContent / mc:Choice / mc:Fallback.
	NSMarkupCompat = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

func inNS(space string, a, b string) bool { return space == a || space == b }

// IsW reports whether name is the WordprocessingML element local ("w:p"...).
func IsW(name xml.Name, local string) bool {
	return name.Local == local && inNS(name.Space, NSWordprocessing, NSWordprocessingStrict)
}

// IsA reports whether name is the DrawingML element local ("a:t"...).
func IsA(name xml.Name, local string) bool {
	return name.Local == local && inNS(name.Space, NSDrawing, NSDrawingStrict)
}

// IsP reports whether name is the PresentationML element local ("p:sld"...).
func IsP(name xml.Name, local string) bool {
	return name.Local == local && inNS(name.Space, NSPresentation, NSPresentationStrict)
}

// IsMC reports whether name is the markup-compatibility element local.
func IsMC(name xml.Name, local string) bool {
	return name.Local == local && name.Space == NSMarkupCompat
}

// IsPkgRel reports whether name is an element of a .rels part.
func IsPkgRel(name xml.Name, local string) bool {
	return name.Local == local && name.Space == NSPackageRels
}

// AttrW returns the value of the WordprocessingML attribute local (w:val...).
func AttrW(se xml.StartElement, local string) (string, bool) {
	return attr(se, local, func(s string) bool { return inNS(s, NSWordprocessing, NSWordprocessingStrict) })
}

// AttrR returns the value of the relationships attribute local (r:id...).
func AttrR(se xml.StartElement, local string) (string, bool) {
	return attr(se, local, func(s string) bool { return inNS(s, NSRelationships, NSRelationshipsStrict) })
}

// AttrPlain returns the value of an attribute with no namespace (e.g. the
// show="0" of a slide, or an Id of a .rels).
func AttrPlain(se xml.StartElement, local string) (string, bool) {
	return attr(se, local, func(s string) bool { return s == "" })
}

func attr(se xml.StartElement, local string, okSpace func(string) bool) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == local && okSpace(a.Name.Space) {
			return a.Value, true
		}
	}
	return "", false
}

// Decoder is a streaming XML reader with the limits of brief section 5.2:
// element depth, accumulated character data, and no DTD. It never builds a
// tree. encoding/xml does not resolve external entities, and Decoder.Entity is
// deliberately left unset. Any directive is refused: legitimate OOXML has none,
// and refusing all of them (not only <!DOCTYPE) leaves no <!ENTITY to reason
// about.
type Decoder struct {
	d        *xml.Decoder
	depth    int
	maxDepth int
	runes    int
	maxRunes int
}

// NewDecoder wraps r. Zero fields of lim take their defaults.
func NewDecoder(r io.Reader, lim Limits) *Decoder {
	lim = lim.withDefaults()
	d := xml.NewDecoder(r)
	d.Strict = true
	// CharsetReader stays nil: a non-UTF-8 declaration is a syntax error.
	return &Decoder{d: d, maxDepth: lim.MaxXMLDepth, maxRunes: lim.MaxTextRunes}
}

// Next returns the next token. The bytes of a CharData token are only valid
// until the next call (encoding/xml semantics): copy what you keep. It returns
// io.EOF at the end; any other failure is a *Error (ReasonMalformed, or
// ReasonTooLong when the text accumulated passes MaxTextRunes).
func (x *Decoder) Next() (xml.Token, error) {
	tok, err := x.d.Token()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, refuse(ReasonMalformed)
	}
	switch t := tok.(type) {
	case xml.StartElement:
		x.depth++
		if x.depth > x.maxDepth {
			return nil, refuse(ReasonMalformed)
		}
	case xml.EndElement:
		x.depth--
	case xml.Directive:
		// "<!DOCTYPE ..." and anything else of the kind.
		return nil, refuse(ReasonMalformed)
	case xml.CharData:
		x.runes += utf8.RuneCount(t)
		if x.runes > x.maxRunes {
			return nil, refuse(ReasonTooLong)
		}
	}
	return tok, nil
}

// Depth is the current element nesting (0 outside the root).
func (x *Decoder) Depth() int { return x.depth }

// TextRunes is the character data accumulated so far.
func (x *Decoder) TextRunes() int { return x.runes }
