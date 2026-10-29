package main

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// ---------- text normalisation ----------

var foldMap = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// fold lowercases and removes the Portuguese diacritics.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := foldMap[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normWords folds s and keeps only letters and digits, single-spaced.
func normWords(s string) string {
	s = fold(s)
	var b strings.Builder
	space := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// ---------- JSON helpers ----------

var fenceRe = regexp.MustCompile("(?s)^```[a-zA-Z]*\\s*\n(.*?)\n?```$")

// unfence removes one markdown code fence around the whole body. fenced says
// whether there was one.
func unfence(body string) (string, bool) {
	b := strings.TrimSpace(body)
	if m := fenceRe.FindStringSubmatch(b); m != nil {
		return strings.TrimSpace(m[1]), true
	}
	return b, false
}

// ---------- S2: numbered sections against the Word gabarito ----------

type gabItem struct {
	Num   string
	Title string
}

type s2Score struct {
	JSONValid  bool     `json:"json_valido"`
	Strict     bool     `json:"json_estrito"` // valid without unfencing
	Items      int      `json:"itens"`
	NumOK      int      `json:"numeros_ok"`         // gabarito numbers present
	NumTitleOK int      `json:"numero_e_titulo_ok"` // number present and its title matches
	Invented   []string `json:"numeros_inventados"` // numbers not in the gabarito
	Missing    []string `json:"numeros_faltando"`
	Total      int      `json:"total_gabarito"`
}

// normNum canonicalises a section number: "1." -> "1", " 6.1 " -> "6.1".
func normNum(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ". )")
	s = strings.TrimLeft(s, "( ")
	return s
}

// titleMatch is a tolerant comparison: equal after normalisation, or one
// contains the other (the model may add or drop a short prefix).
func titleMatch(got, want string) bool {
	g, w := normWords(got), normWords(want)
	if g == "" || w == "" {
		return false
	}
	return g == w || strings.Contains(g, w) || strings.Contains(w, g)
}

func scoreS2(body string, gab []gabItem) s2Score {
	sc := s2Score{Total: len(gab)}
	raw, fenced := unfence(body)
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		sc.Missing = gabNums(gab)
		return sc
	}
	sc.JSONValid = true
	sc.Strict = !fenced
	sc.Items = len(arr)
	got := map[string][]string{} // number -> titles
	for _, it := range arr {
		num, title := "", ""
		for k, v := range it {
			s := strings.TrimSpace(anyString(v))
			switch normWords(k) {
			case "numero", "number", "num", "n":
				num = normNum(s)
			case "titulo", "title", "nome":
				title = s
			}
		}
		if num != "" {
			got[num] = append(got[num], title)
		}
	}
	want := map[string]string{}
	for _, g := range gab {
		want[normNum(g.Num)] = g.Title
	}
	for _, g := range gab {
		n := normNum(g.Num)
		titles, ok := got[n]
		if !ok {
			sc.Missing = append(sc.Missing, n)
			continue
		}
		sc.NumOK++
		for _, t := range titles {
			if titleMatch(t, g.Title) {
				sc.NumTitleOK++
				break
			}
		}
	}
	for n := range got {
		if _, ok := want[n]; !ok {
			sc.Invented = append(sc.Invented, n)
		}
	}
	sort.Strings(sc.Invented)
	return sc
}

func gabNums(gab []gabItem) []string {
	out := make([]string, 0, len(gab))
	for _, g := range gab {
		out = append(out, normNum(g.Num))
	}
	return out
}

func anyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		b, _ := json.Marshal(t)
		return string(b)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// ---------- S3: output format ----------

type fmtScore struct {
	OK     bool   `json:"ok"`     // the requested format, at most one code fence around it
	Strict bool   `json:"strito"` // the requested format with nothing else, not even a fence
	Why    string `json:"motivo,omitempty"`
}

// checkJSONStrings: a JSON array of strings, non-empty.
func checkJSONStrings(body string) fmtScore {
	raw, fenced := unfence(body)
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return fmtScore{Why: "json_invalido"}
	}
	if len(arr) == 0 {
		return fmtScore{Why: "array_vazio"}
	}
	return fmtScore{OK: true, Strict: !fenced}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// checkDashList: every non-empty line starts with "- ".
func checkDashList(body string) fmtScore {
	raw, fenced := unfence(body)
	ls := nonEmptyLines(raw)
	if len(ls) == 0 {
		return fmtScore{Why: "vazio"}
	}
	for _, l := range ls {
		if !strings.HasPrefix(l, "- ") {
			return fmtScore{Why: "linha_sem_marcador"}
		}
	}
	return fmtScore{OK: true, Strict: !fenced}
}

var sepCellRe = regexp.MustCompile(`^:?-{3,}:?$`)

func tableCells(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	l = strings.TrimSuffix(l, "|")
	parts := strings.Split(l, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// checkTable2: a markdown table with exactly 2 columns: header, separator,
// at least one row; every non-empty line is a table line.
func checkTable2(body string) fmtScore {
	raw, fenced := unfence(body)
	ls := nonEmptyLines(raw)
	if len(ls) < 3 {
		return fmtScore{Why: "linhas_insuficientes"}
	}
	for i, l := range ls {
		if !strings.HasPrefix(l, "|") || !strings.HasSuffix(l, "|") {
			return fmtScore{Why: "texto_fora_da_tabela"}
		}
		cells := tableCells(l)
		if len(cells) != 2 {
			return fmtScore{Why: "colunas_diferentes_de_2"}
		}
		if i == 1 {
			for _, c := range cells {
				if !sepCellRe.MatchString(c) {
					return fmtScore{Why: "sem_separador"}
				}
			}
		}
	}
	return fmtScore{OK: true, Strict: !fenced}
}

// ---------- S4: structure of a translation ----------

type structScore struct {
	SrcLines   int      `json:"linhas_origem"`
	DstLines   int      `json:"linhas_traducao"`
	LineRatio  float64  `json:"razao_linhas"`
	SrcMarkers int      `json:"marcadores_origem"`
	DstMarkers int      `json:"marcadores_traducao"`
	IdsTotal   int      `json:"identificadores"`
	IdsKept    int      `json:"identificadores_preservados"`
	IdsLost    []string `json:"identificadores_perdidos,omitempty"`
}

var markerRe = regexp.MustCompile(`^(?:[-*•▪◦]\s|#{1,6}\s|\d+(?:\.\d+)*[.)]?\s|\|)`)

// idRe: tokens a translation must keep verbatim: words with a digit, an
// underscore, a dot inside or an inner capital (CamelCase), and URLs.
var idRe = regexp.MustCompile(`https?://\S+|\b[\p{L}_][\p{L}\p{N}_]*(?:[._][\p{L}\p{N}_]+)+\b|\b\p{L}*\p{Ll}\p{Lu}[\p{L}\p{N}]*\b|\b[\p{L}_]*\p{N}[\p{L}\p{N}_]*\b|\b\p{Lu}{2,}[\p{L}\p{N}]*\b`)

func countMarkers(ls []string) int {
	n := 0
	for _, l := range ls {
		if markerRe.MatchString(l) {
			n++
		}
	}
	return n
}

func identifiers(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range idRe.FindAllString(s, -1) {
		m = strings.TrimRight(m, ".,;:)")
		if len([]rune(m)) < 2 || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func scoreStruct(src, dst string) structScore {
	sl, dl := nonEmptyLines(src), nonEmptyLines(dst)
	sc := structScore{SrcLines: len(sl), DstLines: len(dl), SrcMarkers: countMarkers(sl), DstMarkers: countMarkers(dl)}
	if len(sl) > 0 {
		sc.LineRatio = float64(len(dl)) / float64(len(sl))
	}
	ids := identifiers(src)
	sc.IdsTotal = len(ids)
	for _, id := range ids {
		if strings.Contains(dst, id) {
			sc.IdsKept++
		} else {
			sc.IdsLost = append(sc.IdsLost, id)
		}
	}
	return sc
}

// ---------- S1: automatic screening of a summary ----------

var numTokRe = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// capTokRe: a capitalised word or an acronym (candidate proper noun).
var capTokRe = regexp.MustCompile(`\b\p{Lu}[\p{L}\p{N}\-]{2,}\b`)

type s1Screen struct {
	Numbers    int      `json:"numeros"`
	NumsAbsent []string `json:"numeros_ausentes_na_fonte"`
	Names      int      `json:"nomes"`
	NameAbsent []string `json:"nomes_ausentes_na_fonte"`
}

// screenSummary lists the numbers and capitalised words of a summary that do
// not occur in the source text (case and accents ignored; "1.000" == "1000").
// It is a filter for the human review, not a verdict: a translated word or a
// sentence-initial word is a false positive.
func screenSummary(summary, source string) s1Screen {
	srcFold := fold(source)
	srcDigits := map[string]bool{}
	for _, m := range numTokRe.FindAllString(source, -1) {
		srcDigits[digitsOnly(m)] = true
	}
	var sc s1Screen
	seen := map[string]bool{}
	for _, m := range numTokRe.FindAllString(summary, -1) {
		d := digitsOnly(m)
		if seen["n"+d] {
			continue
		}
		seen["n"+d] = true
		sc.Numbers++
		if !srcDigits[d] && !strings.Contains(srcFold, fold(m)) {
			sc.NumsAbsent = append(sc.NumsAbsent, m)
		}
	}
	for _, m := range capTokRe.FindAllString(summary, -1) {
		f := fold(m)
		if seen["w"+f] {
			continue
		}
		seen["w"+f] = true
		sc.Names++
		if !strings.Contains(srcFold, f) {
			sc.NameAbsent = append(sc.NameAbsent, m)
		}
	}
	return sc
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
