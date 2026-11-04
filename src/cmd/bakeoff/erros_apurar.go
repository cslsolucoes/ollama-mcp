package main

// Mode apurar of the suite `erros` (stage 12.7; C-5.5, C-5.6, C-5.10, C-5.11): it
// takes the owner's judgement of the blind review and, only then, applies the
// limits to the categories that the program cannot score. It makes no call to any
// gateway: it reads the result file of the comparison, the key written beside the
// blind file and the owner's answers, and checks again that the retrato is valid.
//
// The answers file is plain text, one line per case, fields separated by TAB (or
// spaces): label, judgement of output A, judgement of output B, each `certa` or
// `errada`. Empty lines and lines starting with # are ignored. Every label of the
// key has to be answered, none may repeat, and none may be unknown:
//
//	C01<TAB>certa<TAB>errada
//	C02<TAB>errada<TAB>errada
//
// The key (rotulo, ocorrencia_id, categoria, A, B) says which output was the
// current model's and which the candidate's; the owner never sees it.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cslsoftwares/mcpollama/gateway/treino"
)

type keyRow struct {
	Label, Oc, Cat, A, B string
}

func readKey(path string) ([]keyRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("chave do blind ilegível")
	}
	defer f.Close()
	var rows []keyRow
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan(); i++ {
		if i == 0 {
			continue // header
		}
		p := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		if len(p) != 5 || (p[3] != "atual" && p[3] != "candidato") || (p[4] != "atual" && p[4] != "candidato") || p[3] == p[4] {
			return nil, fmt.Errorf("chave do blind: linha %d inválida", i+1)
		}
		rows = append(rows, keyRow{p[0], p[1], p[2], p[3], p[4]})
	}
	return rows, sc.Err()
}

// readAnswers parses the owner's file: label -> [judgement of A, judgement of B]
// (true = certa).
func readAnswers(path string, known map[string]bool) (map[string][2]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("arquivo de respostas ilegível")
	}
	defer f.Close()
	out := map[string][2]bool{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		p := strings.Fields(l)
		if len(p) != 3 {
			return nil, fmt.Errorf("respostas: linha %d: esperado rótulo, avaliação de A e avaliação de B", n)
		}
		if !known[p[0]] {
			return nil, fmt.Errorf("respostas: linha %d: rótulo %q desconhecido", n, p[0])
		}
		if _, dup := out[p[0]]; dup {
			return nil, fmt.Errorf("respostas: linha %d: rótulo %q repetido", n, p[0])
		}
		var v [2]bool
		for i, w := range p[1:] {
			switch strings.ToLower(w) {
			case "certa":
				v[i] = true
			case "errada":
			default:
				return nil, fmt.Errorf("respostas: linha %d: %q não é certa nem errada", n, w)
			}
		}
		out[p[0]] = v
	}
	return out, sc.Err()
}

func errosApurar(ctx context.Context, st *treino.Store, eo errosOpts, logf func(string, ...any)) int {
	if eo.Resultado == "" || eo.BlindRespostas == "" {
		logf("FATAL -resultado e -blind-respostas são obrigatórios no modo apurar")
		return exitUsage
	}
	b, err := os.ReadFile(eo.Resultado)
	if err != nil {
		logf("FATAL resultado ilegível")
		return exitUsage
	}
	var rf resultFile
	if err := json.Unmarshal(b, &rf); err != nil || !validID(rf.RetratoID) {
		logf("FATAL resultado inválido")
		return exitUsage
	}
	if rf.BlindFile == "" || rf.BlindFile != filepath.Base(rf.BlindFile) || !strings.HasSuffix(rf.BlindFile, ".txt") {
		logf("FATAL este resultado não tem avaliação às cegas para apurar")
		return exitUsage
	}
	dir := filepath.Dir(eo.Resultado)
	rows, err := readKey(filepath.Join(dir, strings.TrimSuffix(rf.BlindFile, ".txt")+".chave.tsv"))
	if err != nil {
		logf("FATAL %v", err)
		return exitUsage
	}
	known := map[string]bool{}
	for _, r := range rows {
		known[r.Label] = true
	}
	ans, err := readAnswers(eo.BlindRespostas, known)
	if err != nil {
		logf("FATAL %v", err)
		return exitUsage
	}
	cats := append([]catRes(nil), rf.Cats...)
	idx := map[string]int{}
	for i := range cats {
		idx[cats[i].Cat] = i
		if cats[i].Mode == modeBlind {
			cats[i].Human, cats[i].Atual, cats[i].Cand = true, modelStat{}, modelStat{}
		}
	}
	unanswered := 0
	for _, r := range rows {
		a, ok := ans[r.Label]
		i, has := idx[r.Cat]
		if !has || cats[i].Mode != modeBlind {
			logf("FATAL a chave cita a categoria %q, que não é às cegas", r.Cat)
			return exitUsage
		}
		if !ok {
			unanswered++
			continue
		}
		for k, model := range []string{r.A, r.B} {
			ms := &cats[i].Atual
			if model == "candidato" {
				ms = &cats[i].Cand
			}
			ms.N++
			if a[k] {
				ms.Pass++
			}
		}
	}
	in := reportInput{Mode: "comparar", Apurado: true, RetratoID: rf.RetratoID, Atual: rf.Atual, Candidato: rf.Candidato, Alvo: rf.Alvo,
		Cats: cats, Unmeasured: rf.Unmeasured, Fora: rf.Fora, Calls: rf.Calls, Baseline: rf.Baseline,
		AtualS23: rf.AtualS23, CandS23: rf.CandS23, BlindFile: rf.BlindFile}
	rt, cases, err := st.RetratoCases(ctx, rf.RetratoID)
	switch {
	case errors.Is(err, treino.ErrRetratoNotFound):
		logf("FATAL retrato inexistente")
		return exitUsage
	case errors.Is(err, treino.ErrRetratoInvalido):
		in.GateExtra = append(in.GateExtra, "o retrato está inválido (valido=0, C-5.13): "+err.Error())
	case err != nil:
		logf("FATAL retrato: %v", err)
		return exitUsage
	}
	_ = cases
	in.Retrato = rt
	if unanswered > 0 {
		in.GateExtra = append(in.GateExtra, fmt.Sprintf("avaliação às cegas incompleta: %d caso(s) sem resposta do owner", unanswered))
	}
	return finish(eo, dir, in, nil, nil, logf)
}
