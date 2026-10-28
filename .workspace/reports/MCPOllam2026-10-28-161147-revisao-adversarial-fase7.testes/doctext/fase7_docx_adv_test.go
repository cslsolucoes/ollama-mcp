package doctext

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------- DOCX: integridade ----------------

func advOnce(t *testing.T, area, name string, data []byte, want ...string) {
	t.Helper()
	txt, err := advDocx(data)
	if err != nil {
		vec(t, area, name, "ACHADO", "recusado: "+reasonStr(err))
		return
	}
	var bad []string
	for _, w := range want {
		if c := count(txt, w); c != 1 {
			bad = append(bad, fmt.Sprintf("%s x%d", w, c))
		}
	}
	if len(bad) > 0 {
		vec(t, area, name, "SILENCIOSO", "contagens erradas: "+strings.Join(bad, ", "))
		return
	}
	vec(t, area, name, "FIEL", fmt.Sprintf("%d marcadores presentes exatamente 1 vez", len(want)))
}

func TestAdvDOCXIntegrity(t *testing.T) {
	// 1. AlternateContent aninhado dentro de Choice, com Fallback VML duplicando tudo.
	inner := `<mc:AlternateContent><mc:Choice Requires="wps"><w:txbxContent>` + dxP("INTERNO-AC") + `</w:txbxContent></mc:Choice>` +
		`<mc:Fallback><w:pict><w:txbxContent>` + dxP("INTERNO-AC") + `</w:txbxContent></w:pict></mc:Fallback></mc:AlternateContent>`
	body := `<w:p><w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wps:txbx><w:txbxContent>` + dxP("EXTERNO-AC") +
		`<w:p><w:r>` + inner + `</w:r></w:p></w:txbxContent></wps:txbx></w:drawing></mc:Choice><mc:Fallback><w:pict><w:txbxContent>` +
		dxP("EXTERNO-AC") + dxP("INTERNO-AC") + `</w:txbxContent></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>` + dxP("DEPOIS-AC")
	advOnce(t, "docx", "ac-aninhado", dxBuild(t, body), "EXTERNO-AC", "INTERNO-AC", "DEPOIS-AC")

	// 2. AlternateContent sem Choice (so Fallback).
	advOnce(t, "docx", "ac-sem-choice", dxBuild(t, `<mc:AlternateContent><mc:Fallback>`+dxP("SO-FALLBACK")+`</mc:Fallback></mc:AlternateContent>`+dxP("APOS")),
		"SO-FALLBACK", "APOS")

	// 3. Dois Choice com o mesmo texto + Fallback.
	advOnce(t, "docx", "ac-dois-choice", dxBuild(t, `<mc:AlternateContent><mc:Choice Requires="a">`+dxP("DUPLO-CH")+`</mc:Choice><mc:Choice Requires="b">`+
		dxP("DUPLO-CH")+`</mc:Choice><mc:Fallback>`+dxP("DUPLO-CH")+`</mc:Fallback></mc:AlternateContent>`), "DUPLO-CH")

	// 3b. Choice sem texto, Fallback com texto (Word mostra o Choice se o entende).
	{
		txt, err := advDocx(dxBuild(t, `<mc:AlternateContent><mc:Choice Requires="wps"><w:p/></mc:Choice><mc:Fallback>`+dxP("FB-ONLY-TEXT")+`</mc:Fallback></mc:AlternateContent>`))
		vec(t, "docx", "ac-choice-vazio-fallback-texto", "INFO", fmt.Sprintf("err=%s fallback_presente=%v (le so o Choice, como o Word quando entende o Requires)", reasonStr(err), strings.Contains(txt, "FB-ONLY-TEXT")))
	}

	// 4. sdt aninhado em bloco e em linha.
	advOnce(t, "docx", "sdt-aninhado", dxBuild(t, `<w:sdt><w:sdtPr><w:alias w:val="ALIAS-NAO"/></w:sdtPr><w:sdtContent><w:sdt><w:sdtContent>`+dxP("SDT-INTERNO")+
		`</w:sdtContent></w:sdt>`+dxP("SDT-EXTERNO")+`</w:sdtContent></w:sdt><w:p><w:sdt><w:sdtContent><w:r><w:t>SDT-LINHA</w:t></w:r></w:sdtContent></w:sdt></w:p>`),
		"SDT-INTERNO", "SDT-EXTERNO", "SDT-LINHA")

	// 5. Campo com begin sem separate/end: o resto do documento some?
	{
		body := dxP("CLAUSULA-UM") + `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r></w:p>` +
			dxP("CLAUSULA-TRES-TEXTO") + dxP("CLAUSULA-QUATRO")
		txt, err := advDocx(dxBuild(t, body))
		lost := !strings.Contains(txt, "CLAUSULA-TRES-TEXTO") || !strings.Contains(txt, "CLAUSULA-QUATRO")
		if err == nil && lost {
			vec(t, "docx", "fldchar-begin-sem-fim", "SILENCIOSO", fmt.Sprintf("2 paragrafos apos o campo sumiram sem erro; saida=%d runas, revisoes=nao", len([]rune(txt))))
		} else {
			vec(t, "docx", "fldchar-begin-sem-fim", "FIEL", reasonStr(err))
		}
	}
	// 6. separate+resultado+end dentro de w:del (begin fora): campo excluido pela metade.
	{
		body := dxP("ANTES-DO-CAMPO") + `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> REF x </w:instrText></w:r>` +
			`<w:del w:id="1" w:author="a"><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:delText>velho</w:delText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:del>` +
			`<w:r><w:t>MESMO-PARAGRAFO</w:t></w:r></w:p>` + dxP("PARAGRAFO-SEGUINTE")
		txt, err := advDocx(dxBuild(t, body))
		lost := []string{}
		for _, w := range []string{"MESMO-PARAGRAFO", "PARAGRAFO-SEGUINTE"} {
			if !strings.Contains(txt, w) {
				lost = append(lost, w)
			}
		}
		if err == nil && len(lost) > 0 {
			vec(t, "docx", "fldchar-separate-end-em-del", "SILENCIOSO", "sumiram: "+strings.Join(lost, ","))
		} else {
			vec(t, "docx", "fldchar-separate-end-em-del", "FIEL", reasonStr(err))
		}
	}
	// 6b. end dentro de w:del (begin e separate fora).
	advOnce(t, "docx", "fldchar-end-em-del", dxBuild(t, `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>PAGE</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r>`+
		`<w:r><w:t>RESULTADO-7</w:t></w:r><w:del w:id="2" w:author="a"><w:r><w:fldChar w:fldCharType="end"/></w:r></w:del></w:p>`+dxP("APOS-END-DEL")), "RESULTADO-7", "APOS-END-DEL")
	// 6c. end sem begin; separate sem begin.
	advOnce(t, "docx", "fldchar-end-sem-begin", dxBuild(t, `<w:p><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>ORFAO-OK</w:t></w:r></w:p>`), "ORFAO-OK")
	// 6d. campos aninhados (IF com REF dentro).
	advOnce(t, "docx", "fldchar-aninhado", dxBuild(t, `<w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>IF </w:instrText></w:r>`+
		`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>REF a</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>CODIGO-INTERNO</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`+
		`<w:r><w:instrText> = 1 "x"</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>RESULTADO-IF</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`+dxP("APOS-IF")),
		"RESULTADO-IF", "APOS-IF")

	// 7. Revisoes aninhadas.
	{
		body := `<w:p><w:del w:id="1" w:author="a"><w:ins w:id="2" w:author="b"><w:r><w:t>INS-DENTRO-DEL</w:t></w:r></w:ins></w:del>` +
			`<w:ins w:id="3" w:author="b"><w:del w:id="4" w:author="a"><w:r><w:delText>DEL-DENTRO-INS</w:delText></w:r></w:del><w:r><w:t>INSERIDO-OK</w:t></w:r></w:ins></w:p>` +
			`<w:p><w:moveFrom w:id="5" w:author="a"><w:r><w:t>MOVIDO-X</w:t></w:r></w:moveFrom></w:p><w:p><w:moveTo w:id="6" w:author="a"><w:r><w:t>MOVIDO-X</w:t></w:r></w:moveTo></w:p>`
		r, err := Extract(context.Background(), FormatDOCX, dxBuild(t, body), Limits{})
		if err != nil {
			vec(t, "docx", "revisoes-aninhadas", "ACHADO", reasonStr(err))
		} else {
			ok := count(r.Text, "INS-DENTRO-DEL") == 0 && count(r.Text, "DEL-DENTRO-INS") == 0 && count(r.Text, "INSERIDO-OK") == 1 && count(r.Text, "MOVIDO-X") == 1 && r.PendingRevisions
			res := "FIEL"
			if !ok {
				res = "SILENCIOSO"
			}
			vec(t, "docx", "revisoes-aninhadas", res, fmt.Sprintf("ins-em-del=%d del-em-ins=%d inserido=%d movido=%d pendentes=%v",
				count(r.Text, "INS-DENTRO-DEL"), count(r.Text, "DEL-DENTRO-INS"), count(r.Text, "INSERIDO-OK"), count(r.Text, "MOVIDO-X"), r.PendingRevisions))
		}
	}
	// 7b. marca de paragrafo excluida (Word funde os paragrafos no estado aceito).
	{
		txt, _ := advDocx(dxBuild(t, `<w:p><w:pPr><w:rPr><w:del w:id="9" w:author="a"/></w:rPr></w:pPr><w:r><w:t>PARTE-A</w:t></w:r></w:p>`+dxP("PARTE-B")))
		vec(t, "docx", "marca-paragrafo-excluida", "INFO", fmt.Sprintf("fundido=%v (nenhum texto perdido; so a quebra de linha difere do Word)", strings.Contains(txt, "PARTE-APARTE-B")))
	}
	// 7c. linha excluida (w:trPr/w:del) com runs NAO marcados.
	{
		txt, _ := advDocx(dxBuild(t, `<w:tbl><w:tr><w:trPr><w:del w:id="1" w:author="a"/></w:trPr><w:tc>`+dxP("LINHA-EXCLUIDA")+`</w:tc></w:tr><w:tr><w:tc>`+dxP("LINHA-OK")+`</w:tc></w:tr></w:tbl>`))
		vec(t, "docx", "linha-excluida-trpr-del", "INFO", fmt.Sprintf("linha_excluida_aparece=%v (o Word marca tambem os runs com w:del; so arquivo forjado expoe isso)", strings.Contains(txt, "LINHA-EXCLUIDA")))
	}

	// 8. hyperlink com runs internos, fldSimple, smartTag, customXml, dir/bdo.
	advOnce(t, "docx", "hyperlink-smarttag-customxml-dir", dxBuild(t, `<w:p><w:hyperlink r:id="rId9"><w:r><w:t>LINK-TXT</w:t></w:r><w:fldSimple w:instr="PAGE"><w:r><w:t>SIMPLE-TXT</w:t></w:r></w:fldSimple></w:hyperlink>`+
		`<w:smartTag w:uri="u" w:element="e"><w:r><w:t>SMART-TXT</w:t></w:r></w:smartTag><w:customXml w:element="c"><w:r><w:t>CXML-TXT</w:t></w:r></w:customXml>`+
		`<w:dir w:val="rtl"><w:r><w:t>DIR-TXT</w:t></w:r></w:dir><w:bdo w:val="rtl"><w:r><w:t>BDO-TXT</w:t></w:r></w:bdo></w:p>`),
		"LINK-TXT", "SIMPLE-TXT", "SMART-TXT", "CXML-TXT", "DIR-TXT", "BDO-TXT")

	// 9. tabelas aninhadas em 3 niveis com gridSpan.
	{
		lvl3 := `<w:tbl><w:tr><w:tc>` + dxP("N3-A") + `</w:tc><w:tc>` + dxP("N3-B") + `</w:tc></w:tr></w:tbl>`
		lvl2 := `<w:tbl><w:tr><w:tc>` + dxP("N2-A") + lvl3 + `</w:tc><w:tc><w:tcPr><w:gridSpan w:val="1000"/></w:tcPr>` + dxP("N2-B") + `</w:tc></w:tr></w:tbl>`
		body := `<w:tbl><w:tr><w:tc>` + dxP("N1-A") + lvl2 + dxP("N1-A2") + `</w:tc><w:tc>` + dxP("N1-B") + `</w:tc></w:tr></w:tbl>` + dxP("APOS-TABELA")
		advOnce(t, "docx", "tabelas-aninhadas-3-niveis", dxBuild(t, body), "N3-A", "N3-B", "N2-A", "N2-B", "N1-A2", "N1-B", "APOS-TABELA")
		txt, _ := advDocx(dxBuild(t, body))
		ia, ib := strings.Index(txt, "N1-A"), strings.Index(txt, "N3-A")
		vec(t, "docx", "tabelas-aninhadas-ordem", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[ia >= 0 && ib > ia], "ordem de leitura N1-A antes de N3-A")
	}
	// 10. celulas mescladas: continuacao com texto.
	{
		body := `<w:tbl><w:tr><w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr>` + dxP("VM-INICIO") + `</w:tc><w:tc>` + dxP("X1") + `</w:tc></w:tr>` +
			`<w:tr><w:tc><w:tcPr><w:vMerge/></w:tcPr>` + dxP("VM-CONTINUACAO-COM-TEXTO") + `</w:tc><w:tc>` + dxP("X2") + `</w:tc></w:tr>` +
			`<w:tr><w:tc><w:tcPr><w:hMerge w:val="restart"/></w:tcPr>` + dxP("HM-INICIO") + `</w:tc><w:tc><w:tcPr><w:hMerge/></w:tcPr>` + dxP("HM-CONTINUACAO-COM-TEXTO") + `</w:tc></w:tr></w:tbl>`
		txt, _ := advDocx(dxBuild(t, body))
		vec(t, "docx", "merge-continuacao-com-texto", "INFO", fmt.Sprintf("vMerge_cont_aparece=%v hMerge_cont_aparece=%v (descartado de proposito: o Word nao exibe texto de celula absorvida; arquivo so forjado)",
			strings.Contains(txt, "VM-CONTINUACAO-COM-TEXTO"), strings.Contains(txt, "HM-CONTINUACAO-COM-TEXTO")))
	}
	// 11. caixa de texto em cabecalho (AlternateContent).
	{
		hdr := `<w:p><w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:txbxContent>` + dxP("CAIXA-NO-CABECALHO") + `</w:txbxContent></mc:Choice><mc:Fallback><w:pict><w:txbxContent>` +
			dxP("CAIXA-NO-CABECALHO") + `</w:txbxContent></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>`
		data := dxBuild(t, dxP("CORPO")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="header1.xml"/>`)},
			entry{name: "word/header1.xml", data: dxPartXML("hdr", hdr)})
		advOnce(t, "docx", "caixa-texto-cabecalho", data, "CAIXA-NO-CABECALHO", "CORPO")
	}
	// 12. notas com ids repetidos; nota aninhada; comentario com resposta.
	{
		fn := `<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>` +
			`<w:footnote w:id="1">` + dxP("NOTA-A") + `</w:footnote><w:footnote w:id="1">` + dxP("NOTA-B") + `</w:footnote>`
		cm := `<w:comment w:id="0" w:author="Revisor Um">` + dxP("COMENTARIO-PAI") + `</w:comment><w:comment w:id="1" w:author="Revisor Dois">` + dxP("RESPOSTA-FILHA") + `</w:comment>`
		data := dxBuild(t, `<w:p><w:r><w:t>CORPO-NOTA</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`,
			entry{name: "word/footnotes.xml", data: dxPartXML("footnotes", fn)},
			entry{name: "word/comments.xml", data: dxPartXML("comments", cm)},
			entry{name: "word/commentsExtended.xml", data: []byte(`<w15:commentsEx xmlns:w15="urn:x"/>`)})
		advOnce(t, "docx", "notas-ids-repetidos-e-respostas", data, "NOTA-A", "NOTA-B", "COMENTARIO-PAI", "RESPOSTA-FILHA")
		nested := `<w:footnote w:id="1">` + dxP("NF-A") + `<w:footnote w:id="2">` + dxP("NF-B") + `</w:footnote>` + dxP("NF-C") + `</w:footnote>`
		txt, err := advDocx(dxBuild(t, dxP("C"), entry{name: "word/footnotes.xml", data: dxPartXML("footnotes", nested)}))
		vec(t, "docx", "nota-aninhada", "INFO", fmt.Sprintf("err=%s A=%d B=%d C=%d (texto preservado; atribuicao de NF-C fica fora do rotulo; schema-invalido)", reasonStr(err), count(txt, "NF-A"), count(txt, "NF-B"), count(txt, "NF-C")))
	}
	// 13. w:t fora de w:p (schema-invalido).
	{
		txt, err := advDocx(dxBuild(t, `<w:r><w:t>T-SEM-PARAGRAFO</w:t></w:r>`+dxP("OK")))
		vec(t, "docx", "t-fora-de-p", "INFO", fmt.Sprintf("err=%s presente=%v (schema-invalido; Word recusa)", reasonStr(err), strings.Contains(txt, "T-SEM-PARAGRAFO")))
	}
	// 14. texto oculto (w:vanish) e altChunk.
	{
		txt, _ := advDocx(dxBuild(t, `<w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>TEXTO-OCULTO-VANISH</w:t></w:r></w:p>`+dxP("VISIVEL")))
		// passo 2 (decisao do owner): o texto oculto entra rotulado "[oculto]"
		res := "SILENCIOSO"
		if strings.Contains(txt, "[oculto] TEXTO-OCULTO-VANISH") {
			res = "FIEL"
		}
		vec(t, "docx", "texto-oculto-vanish", res, fmt.Sprintf("rotulado=%v", res == "FIEL"))
	}
	{
		data := dxBuild(t, dxP("ANTES-ALTCHUNK")+`<w:altChunk r:id="rIdAlt"/>`+dxP("DEPOIS-ALTCHUNK"),
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rIdAlt" Type="` + relT + `aFChunk" Target="afchunk.htm"/>`)},
			entry{name: "word/afchunk.htm", data: []byte("<html><body><p>CLAUSULA-NO-ALTCHUNK</p></body></html>")})
		r, err := Extract(context.Background(), FormatDOCX, data, Limits{})
		if err == nil && !strings.Contains(r.Text, "CLAUSULA-NO-ALTCHUNK") {
			vec(t, "docx", "altchunk", "SILENCIOSO", fmt.Sprintf("conteudo do w:altChunk (que o Word exibe) sumiu sem erro nem sinal no rodape; pendentes=%v", r.PendingRevisions))
		} else {
			vec(t, "docx", "altchunk", "FIEL", reasonStr(err))
		}
	}
	// 15. alvo do .rels com caixa diferente do nome da entrada (OPC e case-insensitive).
	{
		data := dxBuild(t, dxP("CORPO-CASE")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="Header1.xml"/>`)},
			entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("TIMBRE-CABECALHO-CASE"))})
		txt, err := advDocx(data)
		if err == nil && !strings.Contains(txt, "TIMBRE-CABECALHO-CASE") {
			vec(t, "docx", "rels-alvo-caixa-diferente", "SILENCIOSO", "cabecalho que o Word resolve (nomes de parte sao case-insensitive) foi pulado sem sinal")
		} else {
			vec(t, "docx", "rels-alvo-caixa-diferente", "FIEL", reasonStr(err))
		}
	}
	// 16. entrada principal com caixa diferente; alvo absoluto (legal no OPC).
	{
		data := buildZip(t, entry{name: "[Content_Types].xml", data: []byte(`<Types/>`)}, entry{name: "Word/Document.xml", data: dxDocXML(dxP("X"))})
		_, err := advDocx(data)
		vec(t, "docx", "entrada-word-maiuscula", "INFO", "recusa ruidosa: "+reasonStr(err)+" (o Word abriria; nao ha perda silenciosa)")
		data = dxBuild(t, dxP("CORPO-ABS")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="/word/header1.xml"/>`)},
			entry{name: "word/header1.xml", data: dxPartXML("hdr", dxP("H-ABS"))})
		_, err = advDocx(data)
		vec(t, "docx", "rels-alvo-absoluto-opc", "INFO", "documento inteiro recusado: "+reasonStr(err)+" (alvo absoluto /word/... e valido no OPC; recusa ruidosa)")
	}
	// 17. cabecalho cujo alvo e o proprio document.xml / footnotes.xml (duplicacao).
	{
		data := dxBuild(t, dxP("CORPO-UNICO-X")+`<w:sectPr><w:headerReference w:type="default" r:id="rId1"/></w:sectPr>`,
			entry{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="` + relT + `header" Target="document.xml"/>`)})
		txt, err := advDocx(data)
		if err == nil && count(txt, "CORPO-UNICO-X") > 1 {
			vec(t, "docx", "cabecalho-aponta-document-xml", "SILENCIOSO", fmt.Sprintf("corpo duplicado %dx como [cabecalho] (alvo/Type do rel nao verificados)", count(txt, "CORPO-UNICO-X")))
		} else {
			vec(t, "docx", "cabecalho-aponta-document-xml", "FIEL", reasonStr(err))
		}
	}
}

// ---------------- DOCX: numeracao ----------------

func TestAdvDOCXNumbering(t *testing.T) {
	abs := nbAbs("0", nbLvl(0, "decimal", "%1.", ""), nbLvl(1, "decimal", "%1.%2.", ""), nbLvl(8, "lowerRoman", "(%9)", ""))
	num := abs + nbNum("1", "0", "")
	// numId inexistente
	{
		txt, err := advDocx(nbDocx(t, num, "", nbP("99", 0, "TEXTO-NUMID-INEXISTENTE")+nbP("1", 0, "PRIMEIRA")))
		vec(t, "num", "numid-inexistente", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[err == nil && strings.Contains(txt, "TEXTO-NUMID-INEXISTENTE") && strings.Contains(txt, "1.\tPRIMEIRA")],
			fmt.Sprintf("texto preservado sem prefixo; saida=%q", txt))
	}
	// ilvl fora de faixa
	for _, il := range []string{"12", "-3", "abc", "99999999999999999999"} {
		body := `<w:p><w:pPr><w:numPr><w:ilvl w:val="` + il + `"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>ILVL</w:t></w:r></w:p>`
		txt, err := advDocx(nbDocx(t, num, "", body))
		vec(t, "num", "ilvl-"+il, "INFO", fmt.Sprintf("err=%s saida=%q (texto preservado; prefixo por fixacao em 0..8)", reasonStr(err), strings.TrimSpace(txt)))
	}
	// %10
	{
		n := nbAbs("0", nbLvl(0, "decimal", "%10.", "")) + nbNum("1", "0", "")
		txt, _ := advDocx(nbDocx(t, n, "", nbP("1", 0, "A")+nbP("1", 0, "B")))
		vec(t, "num", "percent-10", "INFO", fmt.Sprintf("saida=%q (%%10 = %%1 seguido de '0', igual ao Word: o 1o item vira \"10.\")", txt))
	}
	// ciclo de styleLink/numStyleLink
	{
		n := `<w:abstractNum w:abstractNumId="0"><w:styleLink w:val="S1"/><w:numStyleLink w:val="S2"/></w:abstractNum>` +
			`<w:abstractNum w:abstractNumId="1"><w:styleLink w:val="S2"/><w:numStyleLink w:val="S1"/></w:abstractNum>` + nbNum("1", "0", "")
		done := make(chan string, 1)
		go func() { txt, err := advDocx(nbDocx(t, n, "", nbP("1", 0, "CICLO-OK"))); done <- reasonStr(err) + " " + txt }()
		select {
		case r := <-done:
			vec(t, "num", "ciclo-stylelink", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[strings.Contains(r, "CICLO-OK")], strings.TrimSpace(r))
		case <-time.After(10 * time.Second):
			vec(t, "num", "ciclo-stylelink", "ESCAPOU", "travou > 10 s")
		}
	}
	// ciclo basedOn nos estilos
	{
		st := nbStyle("A", "B", "") + nbStyle("B", "A", "")
		txt, err := advDocx(nbDocx(t, num, st, nbStylePara("A", "BASEDON-CICLO")))
		vec(t, "num", "ciclo-basedon", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[err == nil && strings.Contains(txt, "BASEDON-CICLO")], reasonStr(err))
	}
	// numbering.xml malformado: o documento sai SEM nenhuma numeracao e sem sinal.
	{
		data := dxBuild(t, nbP("1", 0, "CLAUSULA-COM-NUMERO"), entry{name: "word/numbering.xml", data: []byte(`<?xml version="1.0"?><w:numbering xmlns:w="` + NSWordprocessing + `">` + num + `<w:abstractNum>`)})
		r, err := Extract(context.Background(), FormatDOCX, data, Limits{})
		// passo 2 (decisao do owner): sem numeros, mas com o sinal NumberingUnavailable
		// (rodape numeracao=indisponivel); silencioso so se o sinal faltar
		switch {
		case err == nil && !strings.Contains(r.Text, "1.") && !r.NumberingUnavailable:
			vec(t, "num", "numbering-xml-malformado", "SILENCIOSO", fmt.Sprintf("documento extraido sem os numeros das clausulas e sem sinal; saida=%q", r.Text))
		case err == nil && r.NumberingUnavailable:
			vec(t, "num", "numbering-xml-malformado", "FIEL", fmt.Sprintf("texto preservado (%q) e NumberingUnavailable=true (rodape numeracao=indisponivel)", r.Text))
		default:
			vec(t, "num", "numbering-xml-malformado", "FIEL", reasonStr(err))
		}
	}
	// numbering.xml com > 100000 elementos w: (valido, so grande).
	{
		var sb strings.Builder
		sb.WriteString(num)
		for i := 2; i < 50005; i++ {
			sb.WriteString(fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="0"/></w:num>`, i))
		}
		data := nbDocx(t, sb.String(), "", nbP("1", 0, "CLAUSULA-NUM-GRANDE"))
		txt, err := advDocx(data)
		if err == nil && !strings.Contains(txt, "1.\tCLAUSULA-NUM-GRANDE") {
			vec(t, "num", "numbering-100k-elementos", "SILENCIOSO", fmt.Sprintf("numbering.xml de %d bytes (valido) perde TODA a numeracao sem sinal; saida=%q", sb.Len(), strings.TrimSpace(txt)))
		} else {
			vec(t, "num", "numbering-100k-elementos", "FIEL", reasonStr(err))
		}
	}
}

// Amplificacao: w:lvlText e um atributo (nao conta em MaxTextRunes) e o prefixo
// e copiado em CADA paragrafo numerado; dentro de celula as linhas nao contam
// no teto ate a linha da tabela terminar.
func advLvlTextDocx(t *testing.T, lvlPairs, paras int, inCell bool) []byte {
	// %2 aponta para o nivel 1, que nunca e incrementado: vale sempre o w:start
	// dele (1664), que em upperLetter rende 64 letras por ocorrencia.
	lvl := strings.Repeat("%2", lvlPairs)
	n := `<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="` + lvl + `"/></w:lvl>` +
		`<w:lvl w:ilvl="1"><w:start w:val="1664"/><w:numFmt w:val="upperLetter"/><w:lvlText w:val="x"/></w:lvl></w:abstractNum>` + nbNum("1", "0", "")
	p := `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr></w:p>`
	body := strings.Repeat(p, paras)
	if inCell {
		body = `<w:tbl><w:tr><w:tc>` + body + `</w:tc></w:tr></w:tbl>`
	}
	return nbDocx(t, n, "", body)
}

func TestAdvDOCXLvlTextAmplification(t *testing.T) {
	for _, c := range []struct {
		name          string
		pairs, paras  int
		inCell        bool
	}{
		{"raiz-1par", 64 << 10, 1, false},
		{"raiz-1par-lvltext-2MiB", 1 << 20, 1, false},
		{"celula-8par", 64 << 10, 8, true},
		{"celula-16par", 64 << 10, 16, true},
		{"celula-32par", 64 << 10, 32, true},
	} {
		data := advLvlTextDocx(t, c.pairs, c.paras, c.inCell)
		var err error
		var start = time.Now()
		peak, total := peakHeap(func() { _, err = advDocx(data) })
		el := time.Since(start)
		prefix := c.pairs * 64
		res := "INFO"
		if peak > 64<<20 {
			res = "ACHADO"
		}
		vec(t, "rec", "lvltext-amplificacao-"+c.name, res, fmt.Sprintf("docx=%d B; prefixo/paragrafo=%d runas; pico heap=%s; alocado=%s; %v; resultado=%s; razao pico/arquivo=%.0fx",
			len(data), prefix, mib(peak), mib(total), el.Round(time.Millisecond), reasonStr(err), float64(peak)/float64(len(data))))
	}
}

// Determinismo (requisito 1.10).
func TestAdvDOCXDeterminism(t *testing.T) {
	num := nbAbs("0", nbLvl(0, "decimal", "%1.", ""), nbLvl(1, "lowerLetter", "%2)", "")) + nbAbs("5", nbLvl(0, "upperRoman", "%1 -", "")) + nbNum("1", "0", "") + nbNum("2", "5", "")
	st := nbStyle("Clausula", "", `<w:ilvl w:val="0"/><w:numId w:val="2"/>`)
	body := nbP("1", 0, "um") + nbP("1", 1, "a") + nbStylePara("Clausula", "estilo") + nbP("1", 0, "dois") +
		`<w:tbl><w:tr><w:tc>` + nbP("1", 1, "cel") + `</w:tc><w:tc>` + dxP("x") + `</w:tc></w:tr></w:tbl>` +
		`<w:p><w:r><w:t>nota</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p><w:sectPr><w:headerReference w:type="default" r:id="rId1"/><w:headerReference w:type="first" r:id="rId2"/><w:footerReference w:type="default" r:id="rId3"/></w:sectPr>`
	extra := []entry{
		{name: "word/numbering.xml", data: dxPartXML("numbering", num)},
		{name: "word/styles.xml", data: dxPartXML("styles", st)},
		{name: "word/_rels/document.xml.rels", data: relsXML(`<Relationship Id="rId1" Type="t" Target="header1.xml"/>`, `<Relationship Id="rId2" Type="t" Target="header2.xml"/>`, `<Relationship Id="rId3" Type="t" Target="footer1.xml"/>`)},
		{name: "word/header1.xml", data: dxPartXML("hdr", dxP("H1"))},
		{name: "word/header2.xml", data: dxPartXML("hdr", dxP("H2"))},
		{name: "word/footer1.xml", data: dxPartXML("ftr", dxP("F1"))},
		{name: "word/footnotes.xml", data: dxPartXML("footnotes", `<w:footnote w:id="1">`+dxP("fn")+`</w:footnote>`)},
		{name: "word/comments.xml", data: dxPartXML("comments", `<w:comment w:id="1" w:author="A">`+dxP("c1")+`</w:comment><w:comment w:id="2" w:author="B">`+dxP("c2")+`</w:comment>`)},
	}
	data := dxBuild(t, body, extra...)
	hashes := map[[32]byte]int{}
	for i := 0; i < 50; i++ {
		txt, err := advDocx(data)
		if err != nil {
			t.Fatal(err)
		}
		hashes[sha256.Sum256([]byte(txt))]++
	}
	vec(t, "det", "docx-50x", map[bool]string{true: "FIEL", false: "SILENCIOSO"}[len(hashes) == 1], fmt.Sprintf("%d hash(es) distintos em 50 extracoes (numeracao, estilos, 2 cabecalhos, rodape, notas, comentarios)", len(hashes)))
}
