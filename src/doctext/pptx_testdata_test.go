package doctext

import (
	"strconv"
	"testing"
)

const (
	nsPA = NSDrawing
	nsPP = NSPresentation
	nsPR = NSRelationships
	relT = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)

// sldXML wraps shapes in a p:sld with the given root attributes.
func sldXML(rootAttrs, shapes string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><p:sld xmlns:a="` + nsPA + `" xmlns:p="` + nsPP + `" xmlns:r="` + nsPR + `" ` + rootAttrs +
		`><p:cSld><p:spTree><p:nvGrpSpPr/><p:grpSpPr/>` + shapes + `</p:spTree></p:cSld></p:sld>`)
}

// textShape is a p:sp with one a:p per argument.
func textShape(paras ...string) string {
	s := `<p:sp><p:nvSpPr><p:cNvPr id="1" name="x"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/>`
	for _, p := range paras {
		s += `<a:p><a:r><a:t>` + p + `</a:t></a:r></a:p>`
	}
	return s + `</p:txBody></p:sp>`
}

// presXML lists r:ids in display order.
func presXML(rids ...string) []byte {
	s := `<?xml version="1.0" encoding="UTF-8"?><p:presentation xmlns:a="` + nsPA + `" xmlns:p="` + nsPP + `" xmlns:r="` + nsPR + `"><p:sldIdLst>`
	for i, r := range rids {
		s += `<p:sldId id="` + strconv.Itoa(256+i) + `" r:id="` + r + `"/>`
	}
	return []byte(s + `</p:sldIdLst></p:presentation>`)
}

func slideRel(id, target string) string {
	return `<Relationship Id="` + id + `" Type="` + relT + `slide" Target="` + target + `"/>`
}

func notesRel(target string) string {
	return `<Relationship Id="rIdN" Type="` + relT + `notesSlide" Target="` + target + `"/>`
}

func notesXML(shapes string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><p:notes xmlns:a="` + nsPA + `" xmlns:p="` + nsPP + `"><p:cSld><p:spTree>` + shapes + `</p:spTree></p:cSld></p:notes>`)
}

func phShape(typ string, paras ...string) string {
	s := `<p:sp><p:nvSpPr><p:cNvPr id="2" name="n"/><p:cNvSpPr/><p:nvPr><p:ph type="` + typ + `"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/>`
	for _, p := range paras {
		s += `<a:p><a:r><a:t>` + p + `</a:t></a:r></a:p>`
	}
	return s + `</p:txBody></p:sp>`
}

// buildPPTX makes a package with presentation.xml, its rels and the extra entries.
func buildPPTX(tb testing.TB, pres []byte, rels []string, extra ...entry) []byte {
	es := []entry{
		{name: "[Content_Types].xml", data: []byte(`<Types/>`)},
		{name: "ppt/presentation.xml", data: pres},
		{name: "ppt/_rels/presentation.xml.rels", data: relsXML(rels...)},
	}
	return buildZip(tb, append(es, extra...)...)
}

// samplePPTX: three slides whose display order (slide10, slide1, slide2) differs
// from the order of the file names, with notes, a layout and a master.
func samplePPTX(tb testing.TB) []byte {
	return buildPPTX(tb, presXML("rId3", "rId1", "rId2"),
		[]string{slideRel("rId1", "slides/slide1.xml"), slideRel("rId2", "slides/slide2.xml"), slideRel("rId3", "slides/slide10.xml")},
		entry{name: "ppt/slides/slide1.xml", data: sldXML("", textShape("Primeiro"))},
		entry{name: "ppt/slides/slide2.xml", data: sldXML("", textShape("Segundo"))},
		entry{name: "ppt/slides/slide10.xml", data: sldXML("", textShape("Décimo, mas exibido primeiro"))},
		entry{name: "ppt/slides/_rels/slide1.xml.rels", data: relsXML(notesRel("../notesSlides/notesSlide1.xml"))},
		entry{name: "ppt/notesSlides/notesSlide1.xml", data: notesXML(phShape("body", "Nota do apresentador") + phShape("sldNum", "7"))},
		entry{name: "ppt/slideLayouts/slideLayout1.xml", data: sldXML("", textShape("Clique para editar o título"))},
		entry{name: "ppt/slideMasters/slideMaster1.xml", data: sldXML("", textShape("Clique para editar o estilo do título mestre"))},
	)
}
