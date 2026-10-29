package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cslsoftwares/mcpollama/doctext"
)

// maxRefRunes mirrors the server: MaxPromptChars (128000) x MaxChunks (40).
const maxRefRunes = 128000 * 40

// runRef writes <out>/ref/<id>.txt: the text doctext extracts from each corpus
// document, which is exactly what the model receives (no OCR, as in the test
// binary). Only metadata goes to stdout.
func runRef(o options, logf func(string, ...any)) int {
	if err := doctext.ConfigurePDF(doctext.PDFOptions{CacheDir: o.pdfCache}); err != nil {
		logf("FATAL pdf: %v", err)
		return 2
	}
	defer doctext.ClosePDF()
	ents, err := os.ReadDir(o.corpus)
	if err != nil {
		logf("FATAL corpus: %v", err)
		return 2
	}
	refDir := filepath.Join(o.out, "ref")
	if err := os.MkdirAll(refDir, 0o700); err != nil {
		logf("FATAL ref: %v", err)
		return 2
	}
	lim := doctext.DefaultLimits()
	lim.MaxTextRunes = maxRefRunes
	for _, e := range ents {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".docx" && ext != ".pptx" && ext != ".pdf" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(o.corpus, name))
		if err != nil {
			logf("REF %s erro_leitura", name)
			continue
		}
		head := data
		if len(head) > 4096 {
			head = head[:4096]
		}
		f, err := doctext.Detect(name, head)
		if err != nil {
			logf("REF %s detect_erro=%v", name, err)
			continue
		}
		res, err := doctext.Extract(context.Background(), f, data, lim)
		if err != nil {
			logf("REF %s extract_erro=%v", name, err)
			continue
		}
		if err := os.WriteFile(filepath.Join(refDir, name+".txt"), []byte(res.Text), 0o600); err != nil {
			logf("REF %s erro_gravacao", name)
			continue
		}
		logf("REF %s formato=%s bytes=%d caracteres=%d linhas=%d paginas=%d paginas_sem_texto=%d slides=%d numeracao_indisponivel=%v texto_oculto=%v",
			name, res.Format, len(data), len([]rune(res.Text)), strings.Count(res.Text, "\n")+1, res.Pages, res.PagesWithoutText, res.Slides, res.NumberingUnavailable, res.HiddenText)
	}
	return 0
}

func readGab(path string) ([]gabItem, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []gabItem
	for i, l := range strings.Split(strings.TrimPrefix(string(b), string(rune(0xFEFF))), "\n") {
		l = strings.TrimRight(l, "\r")
		if i == 0 || strings.TrimSpace(l) == "" {
			continue
		}
		p := strings.Split(l, "\t")
		if len(p) < 5 {
			continue
		}
		// Only numbered headings: a list number and a non-empty title
		// (Word also numbers three empty "Normal" paragraphs and the bullets).
		// Bullets carry a private-use glyph as their list string: require a digit.
		if digitsOnly(p[2]) == "" || strings.TrimSpace(p[4]) == "" {
			continue
		}
		out = append(out, gabItem{Num: p[2], Title: p[4]})
	}
	return out, nil
}

func readBody(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return bodyOf(string(b)), true
}

// runScore prints the automatic scores of S1 (screening), S2, S3 and S4 for
// each model and writes them as JSON to <out>/<model>/score.json.
func runScore(o options, logf func(string, ...any)) int {
	gab, err := readGab(o.gab)
	if err != nil {
		logf("FATAL gabarito: %v", err)
		return 2
	}
	logf("GAB itens=%d", len(gab))
	for _, m := range splitList(o.models) {
		dir := filepath.Join(o.out, safeName(m))
		score := map[string]any{"modelo": m}
		// S2
		var s2 []s2Score
		sumNum, sumNT, valid, strict, inv := 0, 0, 0, 0, 0
		for k := 1; k <= o.reps; k++ {
			body, ok := readBody(filepath.Join(dir, "s2", fmt.Sprintf("%s-r%d.txt", o.s2Doc, k)))
			if !ok {
				continue
			}
			sc := scoreS2(body, gab)
			s2 = append(s2, sc)
			sumNum += sc.NumOK
			sumNT += sc.NumTitleOK
			inv += len(sc.Invented)
			if sc.JSONValid {
				valid++
			}
			if sc.Strict {
				strict++
			}
			logf("S2 modelo=%s r%d json=%v estrito=%v itens=%d numeros_ok=%d/%d numero_titulo_ok=%d/%d inventados=%v faltando=%v",
				m, k, sc.JSONValid, sc.Strict, sc.Items, sc.NumOK, sc.Total, sc.NumTitleOK, sc.Total, sc.Invented, sc.Missing)
		}
		if n := len(s2); n > 0 {
			logf("S2-TOTAL modelo=%s execucoes=%d numeros_ok=%d/%d (%.1f%%) numero_titulo_ok=%d/%d (%.1f%%) json_valido=%d/%d estrito=%d/%d numeros_inventados=%d",
				m, n, sumNum, n*len(gab), pctf(sumNum, n*len(gab)), sumNT, n*len(gab), pctf(sumNT, n*len(gab)), valid, n, strict, n, inv)
		}
		score["s2"] = s2
		// S3
		s3 := map[string]fmtScore{}
		okN, stN, tot := 0, 0, 0
		perFmt := map[string][2]int{}
		for _, d := range splitList(o.s3Docs) {
			for _, f := range s3Formats {
				for k := 1; k <= o.s3Reps; k++ {
					key := fmt.Sprintf("%s-%s-r%d", d, f.key, k)
					body, ok := readBody(filepath.Join(dir, "s3", key+".txt"))
					if !ok {
						s3[key] = fmtScore{Why: "sem_saida"}
						tot++
						pf := perFmt[f.key]
						pf[1]++
						perFmt[f.key] = pf
						continue
					}
					var sc fmtScore
					switch f.key {
					case "json":
						sc = checkJSONStrings(body)
					case "lista":
						sc = checkDashList(body)
					default:
						sc = checkTable2(body)
					}
					s3[key] = sc
					tot++
					pf := perFmt[f.key]
					pf[1]++
					if sc.OK {
						okN++
						pf[0]++
					}
					if sc.Strict {
						stN++
					}
					perFmt[f.key] = pf
					if !sc.OK {
						logf("S3 modelo=%s %s falhou motivo=%s", m, key, sc.Why)
					}
				}
			}
		}
		logf("S3-TOTAL modelo=%s ok=%d/%d (%.1f%%) estrito_sem_cerca=%d/%d json=%d/%d lista=%d/%d tabela=%d/%d",
			m, okN, tot, pctf(okN, tot), stN, tot, perFmt["json"][0], perFmt["json"][1], perFmt["lista"][0], perFmt["lista"][1], perFmt["tabela"][0], perFmt["tabela"][1])
		score["s3"] = s3
		// S4
		s4 := map[string]structScore{}
		for _, p := range splitList(o.s4Pairs) {
			id, _, _ := strings.Cut(p, "=")
			src, err := os.ReadFile(filepath.Join(o.corpus, id))
			if err != nil {
				continue
			}
			body, ok := readBody(filepath.Join(dir, "s4", id+".traducao.txt"))
			if !ok {
				logf("S4 modelo=%s %s sem_saida", m, id)
				continue
			}
			sc := scoreStruct(string(src), body)
			s4[id] = sc
			logf("S4 modelo=%s %s linhas=%d->%d razao=%.2f marcadores=%d->%d identificadores=%d/%d perdidos=%v",
				m, id, sc.SrcLines, sc.DstLines, sc.LineRatio, sc.SrcMarkers, sc.DstMarkers, sc.IdsKept, sc.IdsTotal, firstN(sc.IdsLost, 8))
		}
		score["s4"] = s4
		// S1 screening
		s1 := map[string]s1Screen{}
		for _, d := range splitList(o.s1Docs) {
			ref, err := os.ReadFile(filepath.Join(o.out, "ref", d+".txt"))
			if err != nil {
				continue
			}
			for k := 1; k <= o.reps; k++ {
				key := fmt.Sprintf("%s-r%d", d, k)
				body, ok := readBody(filepath.Join(dir, "s1", key+".txt"))
				if !ok {
					continue
				}
				sc := screenSummary(body, string(ref))
				s1[key] = sc
			}
		}
		score["s1_triagem"] = s1
		if b, err := json.MarshalIndent(score, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "score.json"), b, 0o600)
		}
	}
	return 0
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return append(append([]string(nil), s[:n]...), fmt.Sprintf("...+%d", len(s)-n))
	}
	return s
}

func pctf(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// runBlind writes, for each S1 document, <out>/blind/<doc>.txt with every
// model's summaries under random labels (A01..), the automatic screening of
// each label, and the key in <out>/blind/<doc>.key.tsv (not for the reviewer).
func runBlind(o options, logf func(string, ...any)) int {
	dir := filepath.Join(o.out, "blind")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logf("FATAL blind: %v", err)
		return 2
	}
	models := splitList(o.models)
	for di, d := range splitList(o.s1Docs) {
		ref, _ := os.ReadFile(filepath.Join(o.out, "ref", d+".txt"))
		type item struct{ model, key, body string }
		var items []item
		for _, m := range models {
			for k := 1; k <= o.reps; k++ {
				key := fmt.Sprintf("%s-r%d", d, k)
				body, ok := readBody(filepath.Join(o.out, safeName(m), "s1", key+".txt"))
				if !ok {
					continue // not run (e.g. pulado-risco): nothing to judge
				}
				items = append(items, item{m, key, body})
			}
		}
		idx := make([]string, len(items))
		for i := range items {
			idx[i] = fmt.Sprint(i)
		}
		order := shuffled(idx, int64(8000+di))
		var txt, key strings.Builder
		key.WriteString("rotulo\tmodelo\tarquivo\n")
		for pos, si := range order {
			it := items[atoiOr(si, 0)]
			label := fmt.Sprintf("%c%02d", 'A'+di, pos+1)
			sc := screenSummary(it.body, string(ref))
			fmt.Fprintf(&txt, "===== %s\n[triagem automática: números ausentes na fonte=%v; palavras capitalizadas ausentes na fonte=%v]\n%s\n\n", label, sc.NumsAbsent, firstN(sc.NameAbsent, 15), it.body)
			fmt.Fprintf(&key, "%s\t%s\t%s\n", label, it.model, it.key)
		}
		_ = os.WriteFile(filepath.Join(dir, d+".txt"), []byte(txt.String()), 0o600)
		_ = os.WriteFile(filepath.Join(dir, d+".key.tsv"), []byte(key.String()), 0o600)
		logf("BLIND %s resumos=%d", d, len(items))
	}
	return 0
}
