package main

// Verdict, report, baseline and blind files of the suite `erros` (stage 12.7;
// C-5.8 to C-5.14, C-5.16). The report NEVER holds the text of a request or of an
// answer: only ids, categories and numbers (C-5.6). The texts of the blind review
// go to a separate file, 0600, for the owner.

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

// reportInput is everything the verdict and the report need. No text.
type reportInput struct {
	Mode, RetratoID        string
	Retrato                treino.Retrato
	Atual, Candidato, Alvo string
	Cats                   []catRes
	Unmeasured, Fora       int
	Calls                  int
	GateExtra              []string // reasons already known (invalid retrato)
	Baseline               *baselineFile
	AtualS23, CandS23      *s23
	CandSameScore          bool   // the score of the candidate is the very file of the current model (I-9)
	BlindFile              string // base name only
	Now                    time.Time
	Apurado                bool // the report of mode apurar: the blind categories carry the owner's counts
	ResultFile             string
}

// resultFile is what a comparison leaves for mode apurar: ids, categories and
// numbers, no text.
type resultFile struct {
	RetratoID, Atual, Candidato, Alvo string
	Cats                              []catRes
	Unmeasured, Fora, Calls           int
	Baseline                          *baselineFile
	AtualS23, CandS23                 *s23
	CandSameScore                     bool
	BlindFile                         string
	GeradoEm                          string
}

type outcome struct {
	GateFail []string // C-5.13 reasons: the gate fails
	Rejects  []string // the evaluation is valid and the change does not meet the limits
	Warnings []string
	Accepted bool
	Code     int
}

// autoCat says whether a category is scored by the program (not blind, not out).
func autoCat(c catRes) bool { return c.Mode != modeBlind && c.Mode != modeOutside }

// scored says whether a category has numbers that decide: scored by the program,
// or judged by the owner (mode apurar).
func scored(c catRes) bool { return autoCat(c) || (c.Mode == modeBlind && c.Human) }

// blindPending says whether some category is still waiting for the owner.
func blindPending(in reportInput) bool {
	for _, c := range in.Cats {
		if c.Mode == modeBlind && !c.Human && c.Blind > 0 {
			return true
		}
	}
	return false
}

func catOf(in reportInput, name string) (catRes, bool) {
	for _, c := range in.Cats {
		if c.Cat == name {
			return c, true
		}
	}
	return catRes{}, false
}

// evaluate applies the gate of C-5.13 and, only when the data suffice, the limits
// of C-5.10 to C-5.12. All comparisons of points use integers (no rounding).
func evaluate(in reportInput) outcome {
	var o outcome
	o.GateFail = append(o.GateFail, in.GateExtra...)
	total, decisive := 0, 0
	pending := blindPending(in)
	for _, c := range in.Cats {
		if !scored(c) {
			continue
		}
		total += c.Atual.N
		if c.Atual.N >= minDecide {
			decisive++
		}
	}
	if len(in.GateExtra) == 0 {
		// With the owner's review pending the numbers are not all in: nothing
		// automatic is fine in a comparison, and in the baseline (which has nothing to
		// record for the blind categories); but automatic cases that exist have to
		// reach the minimum in the baseline.
		switch {
		case total == 0:
			if !pending {
				o.GateFail = append(o.GateFail, "nenhum caso foi avaliado (0 casos)")
			}
		case decisive == 0:
			if !(pending && in.Mode == "comparar") {
				o.GateFail = append(o.GateFail, fmt.Sprintf("nenhuma categoria atingiu o mínimo de %d casos para decidir (C-5.8)", minDecide))
			}
		}
	}
	if in.Unmeasured > 0 {
		o.GateFail = append(o.GateFail, fmt.Sprintf("medição incompleta: %d caso(s) não puderam ser medidos", in.Unmeasured))
	}
	if in.Mode == "comparar" {
		if in.CandSameScore { // I-9: the model name is the same, so the S2/S3 of the plain model must not stand for the one with instructions
			o.GateFail = append(o.GateFail, "C-5.12: o S2 e o S3 do candidato apontam o MESMO score.json do atual (o gateway só aceita o nome qwen3-ia, então o candidato com instruções tem o nome do atual): meça o S2/S3 com as instruções, grave num score.json próprio e passe -candidato-score")
		}
		if in.AtualS23 == nil || in.CandS23 == nil {
			o.GateFail = append(o.GateFail, "o S2 e o S3 da Fase 8 não foram medidos para os dois modelos (C-5.12): rode as suítes s2 e s3 e depois -suites score")
		}
		if c, ok := catOf(in, in.Alvo); in.Alvo != "" && ok && c.Mode == modeBlind && !c.Human {
			// the target is judged by the owner: its N is only known after mode apurar
		} else if in.Alvo != "" {
			if c, ok := catOf(in, in.Alvo); !ok || c.Atual.N < minDecide {
				n := 0
				if ok {
					n = c.Atual.N
				}
				o.GateFail = append(o.GateFail, fmt.Sprintf("a categoria-alvo %s tem N=%d, abaixo de %d: não decide (C-5.8)", in.Alvo, n, minDecide))
			}
		}
		if in.Baseline == nil {
			o.Warnings = append(o.Warnings, "não há linha de base registrada para este retrato (C-5.14): rode o modo base antes")
		}
	}
	// C-5.16: the baseline against the new measure of the current model
	if in.Baseline != nil && in.Mode == "comparar" {
		for _, c := range in.Cats {
			b, ok := in.Baseline.Cats[c.Cat]
			if !ok || !autoCat(c) || c.Atual.N < minReport || b.N < minReport {
				continue
			}
			if absInt(b.Acertos*c.Atual.N-c.Atual.Pass*b.N)*100 > baselineDiffP*c.Atual.N*b.N {
				o.Warnings = append(o.Warnings, fmt.Sprintf("C-5.16: categoria %s: linha de base %s (N=%d) e nova medição do modelo atual %s (N=%d) diferem em mais de %d pontos; o servidor pode ter mudado",
					c.Cat, pct(b.Acertos, b.N), b.N, pct(c.Atual.Pass, c.Atual.N), c.Atual.N, baselineDiffP))
			}
		}
	}
	if len(o.GateFail) > 0 {
		o.Code = exitGate
		return o
	}
	if in.Mode == "base" {
		o.Accepted, o.Code = true, exitOK
		return o
	}
	for _, c := range in.Cats {
		if !scored(c) || c.Atual.N < minDecide {
			continue
		}
		if c.Cand.N != c.Atual.N { // m-12: the same cases were not measured for both: it does not decide, and it is not a pass
			o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.8: a categoria %s tem N=%d no atual e N=%d no candidato: os mesmos casos não foram medidos nos dois", c.Cat, c.Atual.N, c.Cand.N))
			continue
		}
		n := c.Atual.N
		if c.Cat == in.Alvo { // C-5.10
			lo, _ := wilson(c.Cand.Pass, n)
			point := float64(c.Atual.Pass) / float64(n)
			if (c.Cand.Pass-c.Atual.Pass)*100 < gainPP*n {
				o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.10: o alvo %s ganhou menos de %d pontos (atual %s, candidato %s)", c.Cat, gainPP, pct(c.Atual.Pass, n), pct(c.Cand.Pass, n)))
			} else if lo <= point {
				o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.10: o limite inferior do candidato em %s (%.1f%%) não passa o acerto do atual (%s)", c.Cat, 100*lo, pct(c.Atual.Pass, n)))
			}
			continue
		}
		if (c.Atual.Pass-c.Cand.Pass)*100 > otherDropPP*n { // C-5.11
			o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.11: a categoria %s caiu mais de %d pontos (atual %s, candidato %s)", c.Cat, otherDropPP, pct(c.Atual.Pass, n), pct(c.Cand.Pass, n)))
		}
	}
	a, b := in.AtualS23, in.CandS23 // C-5.12
	if (a.S2OK*b.S2Tot-b.S2OK*a.S2Tot)*100 > s23DropPP*a.S2Tot*b.S2Tot {
		o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.12: o S2 caiu mais de %d pontos (atual %s, candidato %s)", s23DropPP, pct(a.S2OK, a.S2Tot), pct(b.S2OK, b.S2Tot)))
	}
	if (a.S3OK*b.S3Tot-b.S3OK*a.S3Tot)*100 > s23DropPP*a.S3Tot*b.S3Tot {
		o.Rejects = append(o.Rejects, fmt.Sprintf("C-5.12: o S3 caiu mais de %d pontos (atual %s, candidato %s)", s23DropPP, pct(a.S3OK, a.S3Tot), pct(b.S3OK, b.S3Tot)))
	}
	o.Accepted = len(o.Rejects) == 0 && !pending
	o.Code = exitOK
	switch {
	case len(o.Rejects) > 0:
		o.Code = exitRejected // the automatic categories already break a limit: the owner's review cannot fix that
	case pending:
		o.Code = exitPending
	}
	return o
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func pct(k, n int) string {
	if n == 0 {
		return "-"
	}
	return strings.Replace(fmt.Sprintf("%.1f%%", 100*float64(k)/float64(n)), ".", ",", 1)
}

func wil(k, n int) string {
	lo, hi := wilson(k, n)
	return strings.Replace(fmt.Sprintf("[%.1f–%.1f]", 100*lo, 100*hi), ".", ",", 2)
}

// renderReport writes the report. It contains ids, categories and numbers only.
func renderReport(in reportInput, o outcome) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	label := in.Mode
	if in.Apurado {
		label = "apurar"
	}
	w("# Banco de provas `erros` (modo %s)", label)
	w("")
	w("- Gerado em: %s", in.Now.Format("2006-01-02 15:04:05"))
	w("- **Retrato:** `%s` (válido: %s; criado em %s)", in.RetratoID, yesNo(in.Retrato.Valido), time.UnixMilli(in.Retrato.CriadoEm).Format("2006-01-02 15:04:05"))
	w("- Índice de exemplos: `%s` · Instruções: `%s` / `%s`", orNone(in.Retrato.IndiceHash), orNone(in.Retrato.InstrucoesID), orNone(in.Retrato.InstrucoesHash))
	if in.Retrato.ExemplosK > 0 {
		w("- Exemplos: k=%d · limiar de similaridade=%.4f (C-8.10)", in.Retrato.ExemplosK, in.Retrato.ExemplosLimiar)
	}
	w("- Modelo atual (medido de novo nesta sessão, C-5.16): `%s`", in.Atual)
	if in.Mode == "comparar" {
		w("- Candidato: `%s` · Categoria-alvo: `%s`", in.Candidato, orNone(in.Alvo))
	}
	w("- Casos do retrato: %d · fora da avaliação (C-5.7): %d · não medidos: %d · chamadas ao gateway: %d", len(in.Retrato.Itens), in.Fora, in.Unmeasured, in.Calls)
	w("")
	w("## Resultado")
	w("")
	switch {
	case len(o.GateFail) > 0:
		w("**DADOS INSUFICIENTES. O gate FALHA (C-5.13).** Nada se conclui sobre melhora nem sobre regressão.")
		for _, r := range o.GateFail {
			w("- %s", r)
		}
	case o.Code == exitPending:
		w("**PENDENTE da revisão às cegas (C-5.5).** O que o programa mede não quebra nenhum limite, mas há categorias que só o owner julga, e nada foi aceito: nem melhora, nem \"regressão\" verificada.")
		w("")
		w("Para concluir, o owner abre `%s` (sensível), avalia cada SAÍDA A e B como `certa` ou `errada` e grava o arquivo de respostas, uma linha por caso: `rótulo<TAB>avaliação de A<TAB>avaliação de B`, por exemplo `C01<TAB>certa<TAB>errada`. Linhas vazias e as que começam com # são ignoradas; todos os rótulos têm de ser respondidos. Depois roda `-erros-modo apurar -resultado <resultado-*.json> -blind-respostas <arquivo>`, que aplica C-5.10 e C-5.11 a essas categorias.", orNone(in.BlindFile))
	case in.Mode == "base":
		w("**LINHA DE BASE REGISTRADA** (C-5.14): primeira rodada do modelo `%s` neste retrato. É a referência histórica de tudo que vier depois.", in.Atual)
	case in.Alvo != "" && o.Accepted && in.Apurado:
		w("**MELHORA ACEITA** na categoria `%s` (C-5.10), com as categorias às cegas julgadas pelo owner; nenhuma outra categoria com N ≥ %d caiu mais de %d pontos (C-5.11), nem o S2 e o S3 mais de %d (C-5.12).", in.Alvo, minDecide, otherDropPP, s23DropPP)
	case in.Alvo != "" && o.Accepted:
		w("**MELHORA ACEITA** na categoria `%s` (C-5.10), e nenhuma outra categoria com N ≥ %d caiu mais de %d pontos (C-5.11), nem o S2 e o S3 mais de %d (C-5.12).", in.Alvo, minDecide, otherDropPP, s23DropPP)
	case o.Accepted:
		w("**Sem regressão** nas categorias que decidem (N ≥ %d, queda máxima de %d pontos, C-5.11) e no S2 e S3 (queda máxima de %d, C-5.12). Nenhuma categoria-alvo foi indicada, então a melhora não foi avaliada.", minDecide, otherDropPP, s23DropPP)
	default:
		w("**MUDANÇA REPROVADA.** A avaliação é válida, mas não cumpre C-5.10 a C-5.12:")
		for _, r := range o.Rejects {
			w("- %s", r)
		}
	}
	w("")
	w("## Categorias com número que decide (automáticas e, no modo apurar, as julgadas pelo owner)")
	w("")
	if in.Mode == "comparar" {
		w("| categoria | N | atual | Wilson 95%% | mudaram | candidato | Wilson 95%% | mudaram | Δ (pontos) | situação |")
		w("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	} else {
		w("| categoria | N | atual | Wilson 95%% | mudaram | situação |")
		w("| --- | --- | --- | --- | --- | --- |")
	}
	for _, c := range in.Cats {
		if !scored(c) {
			continue
		}
		n := c.Atual.N
		sit := "decide"
		switch {
		case n < minReport:
			sit = fmt.Sprintf("N < %d: não reportada", minReport)
		case n < minDecide:
			sit = fmt.Sprintf("reportada; N < %d, não decide", minDecide)
		}
		if c.Inapplicable > 0 {
			sit += fmt.Sprintf("; %d inaplicável(is)", c.Inapplicable)
		}
		if n < minReport {
			if in.Mode == "comparar" {
				w("| %s | %d | - | - | - | - | - | - | - | %s |", c.Cat, n, sit)
			} else {
				w("| %s | %d | - | - | - | %s |", c.Cat, n, sit)
			}
			continue
		}
		if in.Mode == "comparar" {
			delta := strings.Replace(fmt.Sprintf("%+.1f", 100*float64(c.Cand.Pass)/float64(n)-100*float64(c.Atual.Pass)/float64(n)), ".", ",", 1)
			w("| %s | %d | %s | %s | %d | %s | %s | %d | %s | %s |", c.Cat, n, pct(c.Atual.Pass, n), wil(c.Atual.Pass, n), c.Atual.Changed,
				pct(c.Cand.Pass, n), wil(c.Cand.Pass, n), c.Cand.Changed, delta, sit)
		} else {
			w("| %s | %d | %s | %s | %d | %s |", c.Cat, n, pct(c.Atual.Pass, n), wil(c.Atual.Pass, n), c.Atual.Changed, sit)
		}
	}
	w("")
	w("Acerto = caso com 2 de 3 rodadas certas (seeds 1, 2 e 3, C-5.15); \"mudaram\" = casos cujo resultado variou entre as rodadas. Nas categorias julgadas pelo owner há 1 rodada (seed 1) e o acerto é a saída marcada como `certa`.")
	var blindCats []catRes
	for _, c := range in.Cats {
		if c.Mode == modeBlind && !c.Human {
			blindCats = append(blindCats, c)
		}
	}
	if len(blindCats) > 0 {
		w("")
		w("## Avaliação humana às cegas (C-5.5)")
		w("")
		for _, c := range blindCats {
			w("- %s: %d caso(s)", c.Cat, c.Blind)
		}
		if in.BlindFile != "" {
			w("- Arquivo para o owner (sensível, 0600): `%s`. Nenhum texto sai da máquina (C-5.6).", in.BlindFile)
			if in.ResultFile != "" {
				w("- Resultado para o modo apurar: `%s`.", in.ResultFile)
			}
		} else if in.Mode != "comparar" {
			w("- Só o modo comparar roda a avaliação às cegas.")
		}
	}
	if in.AtualS23 != nil || in.CandS23 != nil {
		w("")
		w("## S2 e S3 da Fase 8 (C-5.12)")
		w("")
		if a := in.AtualS23; a != nil {
			w("- atual: S2 %s (%d/%d) · S3 %s (%d/%d)", pct(a.S2OK, a.S2Tot), a.S2OK, a.S2Tot, pct(a.S3OK, a.S3Tot), a.S3OK, a.S3Tot)
		}
		if a := in.CandS23; a != nil {
			w("- candidato: S2 %s (%d/%d) · S3 %s (%d/%d)", pct(a.S2OK, a.S2Tot), a.S2OK, a.S2Tot, pct(a.S3OK, a.S3Tot), a.S3OK, a.S3Tot)
		}
	}
	if len(o.Warnings) > 0 {
		w("")
		w("## Avisos")
		w("")
		for _, x := range o.Warnings {
			w("- %s", x)
		}
	}
	w("")
	w("_Este relatório traz só ids, categorias e números; nenhum texto de pedido ou de resposta (C-5.6)._")
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "sim"
	}
	return "não"
}

func orNone(s string) string {
	if s == "" {
		return treino.NoneValue
	}
	return s
}

// ---- finishing: files and exit code -----------------------------------------------------------------

// finish evaluates, writes the files and prints the report; it returns the exit code.
func finish(eo errosOpts, outDir string, in reportInput, results []caseRes, blind []blindItem, logf func(string, ...any)) int {
	in.Now = eo.Now()
	stamp := in.Now.Format("20060102-150405")
	basePath := filepath.Join(outDir, "linha-de-base.json")
	if eo.Mode == "comparar" && !in.Apurado {
		if bf, err := readBaseline(basePath, in.RetratoID); err == nil {
			in.Baseline = bf
		}
		in.AtualS23, _ = readS23(filepath.Join(eo.Out, safeName(eo.Atual), "score.json"), eo.Atual, in.Now, eo.S23MaxAge)
		// I-9: the candidate of an improvement has the NAME of the current model (the gateway accepts only qwen3-ia), so its S2/S3
		// come from a score.json of its own (-candidato-score); the same file as the current model's measures nothing.
		atualScore := filepath.Join(eo.Out, safeName(eo.Atual), "score.json")
		candScore := eo.CandScore
		if candScore == "" {
			candScore = filepath.Join(eo.Out, safeName(eo.Candidato), "score.json")
		}
		if sameFile(atualScore, candScore) {
			in.CandSameScore = true
		} else {
			in.CandS23, _ = readS23(candScore, eo.Candidato, in.Now, eo.S23MaxAge)
		}
	}
	if eo.Mode == "base" {
		in.AtualS23, _ = readS23(filepath.Join(eo.Out, safeName(eo.Atual), "score.json"), eo.Atual, in.Now, eo.S23MaxAge) // optional
	}
	if len(blind) > 0 {
		name := "blind-" + stamp + ".txt"
		if err := writeBlind(outDir, name, in.RetratoID, blind, eo.ShuffleSeed); err != nil {
			logf("FATAL arquivo do blind: %v", err)
			return exitUsage
		}
		in.BlindFile = name
	}
	o := evaluate(in)
	if eo.Mode == "base" && o.Code == exitOK {
		bf := baselineFile{RetratoID: in.RetratoID, Modelo: in.Atual, GeradaEm: in.Now.Format(time.RFC3339), Cats: map[string]baselineCt{}, S23: in.AtualS23}
		for _, c := range in.Cats {
			if autoCat(c) && c.Atual.N > 0 {
				bf.Cats[c.Cat] = baselineCt{N: c.Atual.N, Acertos: c.Atual.Pass, Mudaram: c.Atual.Changed}
			}
		}
		for _, r := range results {
			bf.Casos = append(bf.Casos, baselineCase{OcorrenciaID: r.OcorrenciaID, Categoria: r.Categoria, Rodadas: r.Rounds["atual"]})
		}
		b, _ := json.MarshalIndent(bf, "", "  ")
		if err := writeNew(basePath, b); err != nil {
			logf("FATAL linha de base: %v", err)
			return exitUsage
		}
		logf("LINHA-DE-BASE registrada em %s", basePath)
	} else if eo.Mode == "base" {
		logf("LINHA-DE-BASE NÃO registrada: dados insuficientes")
	}
	if eo.Mode == "comparar" && !in.Apurado { // what mode apurar needs to finish the verdict (ids and numbers only)
		rf := resultFile{RetratoID: in.RetratoID, Atual: in.Atual, Candidato: in.Candidato, Alvo: in.Alvo, Cats: in.Cats,
			Unmeasured: in.Unmeasured, Fora: in.Fora, Calls: in.Calls, Baseline: in.Baseline, AtualS23: in.AtualS23, CandS23: in.CandS23, CandSameScore: in.CandSameScore,
			BlindFile: in.BlindFile, GeradoEm: in.Now.Format(time.RFC3339)}
		b, _ := json.MarshalIndent(rf, "", "  ")
		name := "resultado-" + stamp + ".json"
		if err := writeNew(filepath.Join(outDir, name), b); err != nil {
			logf("FATAL resultado: %v", err)
			return exitUsage
		}
		in.ResultFile = name
	}
	rep := renderReport(in, o)
	label := eo.Mode
	if in.Apurado {
		label = "apurar"
	}
	repPath := filepath.Join(outDir, fmt.Sprintf("relatorio-%s-%s.md", label, stamp))
	if err := writeNew(repPath, []byte(rep)); err != nil {
		logf("FATAL relatório: %v", err)
		return exitUsage
	}
	for _, l := range strings.Split(strings.TrimRight(rep, "\n"), "\n") {
		logf("%s", l)
	}
	logf("RELATORIO %s", repPath)
	switch o.Code {
	case exitGate:
		logf("GATE FALHOU: dados insuficientes")
	case exitRejected:
		logf("MUDANCA REPROVADA")
	case exitPending:
		logf("PENDENTE da revisão às cegas do owner")
	}
	return o.Code
}

// privateFileMode is the mode of everything this suite writes (C-5.6): the owner only.
const privateFileMode os.FileMode = 0o600

// writeNew creates a 0600 file and refuses to overwrite an existing one.
func writeNew(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFileMode)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeBlind writes the file for the owner (the request and the two outputs under
// the labels A and B, in random order) and, beside it, the key that says which is
// which. Both are 0600 (on Windows the mode is not enforced: the file is for the
// owner on the server, which is Linux). The texts never go anywhere else.
func writeBlind(dir, name, retrato string, items []blindItem, seed int64) error {
	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		seed = int64(binary.BigEndian.Uint64(b[:]) >> 1)
	}
	rng := mrand.New(mrand.NewSource(seed))
	order := rng.Perm(len(items))
	var txt, key strings.Builder
	fmt.Fprintf(&txt, "AVALIAÇÃO ÀS CEGAS (C-5.5), retrato %s\nARQUIVO SENSÍVEL: contém pedidos de clientes. Só o owner lê (C-5.6). Não copiar para fora da rede.\nPara cada caso, avalie a SAÍDA A e a SAÍDA B como certa ou errada (1 rodada, seed=1).\nRegistre num arquivo de texto, uma linha por caso: rótulo<TAB>avaliação de A<TAB>avaliação de B, por exemplo C01<TAB>certa<TAB>errada.\n\n", retrato)
	key.WriteString("rotulo\tocorrencia_id\tcategoria\tA\tB\n")
	for pos, ix := range order {
		it := items[ix]
		label := fmt.Sprintf("C%02d", pos+1)
		a, b := it.Atual, it.Cand
		ka, kb := "atual", "candidato"
		if rng.Intn(2) == 1 {
			a, b, ka, kb = b, a, kb, ka
		}
		fmt.Fprintf(&txt, "===== %s (categoria: %s)\n--- PEDIDO\n%s\n--- SAÍDA A\n%s\n--- SAÍDA B\n%s\n--- AVALIAÇÃO (no arquivo de respostas): A certa|errada, B certa|errada\n\n", label, it.Categoria, it.Prompt, a, b)
		fmt.Fprintf(&key, "%s\t%s\t%s\t%s\t%s\n", label, it.OcorrenciaID, it.Categoria, ka, kb)
	}
	if err := writeNew(filepath.Join(dir, name), []byte(txt.String())); err != nil {
		return err
	}
	keyName := strings.TrimSuffix(name, ".txt") + ".chave.tsv"
	if err := writeNew(filepath.Join(dir, keyName), []byte(key.String())); err != nil {
		return err
	}
	return nil
}

// sameFile says whether two paths name the same file: the same cleaned absolute path, or (when both exist) the same
// file on disk (a link or a different spelling).
func sameFile(a, b string) bool {
	aa, e1 := filepath.Abs(a)
	bb, e2 := filepath.Abs(b)
	if e1 == nil && e2 == nil && filepath.Clean(aa) == filepath.Clean(bb) {
		return true
	}
	fa, e1 := os.Stat(a)
	fb, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(fa, fb)
}
