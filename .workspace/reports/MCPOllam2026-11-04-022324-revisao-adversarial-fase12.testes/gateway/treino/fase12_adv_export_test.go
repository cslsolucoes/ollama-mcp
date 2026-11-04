package treino

// Revisão adversarial da etapa 12.9 (Fase 12), revisor E: exportação JSONL e partição
// (export.go, partition.go, retrato.go, indice.go, state.go, ingest.go, query.go).
// Contrato 1.1.5: cap. 3 (C-3.1 a C-3.16, C-3.10a), cap. 4 (C-4.5 a C-4.15, C-4.11b),
// C-1.12b, C-1.15, C-6.4, C-7.7.
//
// Cada vetor imprime uma linha VEC|id|OK ou FALHA|descrição. Uma FALHA chama t.Errorf.
//
// Dependências de helpers de outros _test.go do pacote: openStore, testKey, newFake/fakeSrc,
// mkID, pjOf, report, jr, mustValid, stationO (treino_test.go); ctx, admin, must (state_test.go).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func vec12E(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("%s: %s", id, desc)
	}
}

const adv12EModel = "nomic-embed-text"

// adv12EIngest reports r from stationO with the test key.
func adv12EIngest(t *testing.T, s *Store, f *fakeSrc, r Report) string {
	t.Helper()
	res, err := s.Ingest(ctx, mustValid(t, r), stationO, "", f, testKey(t))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	return res.OcorrenciaID
}

// adv12EEl puts job n in the source (mut changes it), reports it as unico with category cat
// and confirms it when conf.
func adv12EEl(t *testing.T, s *Store, f *fakeSrc, n int, cat string, mut func(j *Job, c *Content), conf bool) string {
	t.Helper()
	id := mkID(n)
	if _, ok := f.jobs[id]; !ok {
		f.add(n)
	}
	if mut != nil {
		j, c := f.jobs[id], f.conts[id]
		mut(&j, &c)
		f.jobs[id], f.conts[id] = j, c
	}
	r := report(jr(n, "unico", "primario"))
	r.Categoria, r.RespostaCorreta = cat, fmt.Sprintf("resposta certa %d", n)
	oc := adv12EIngest(t, s, f, r)
	if conf {
		must(t, s.Confirm(ctx, oc, admin))
	}
	return oc
}

// adv12EBridge reports jobs a (map1) and b (final): it links the groups of a and b.
func adv12EBridge(t *testing.T, s *Store, f *fakeSrc, a, b int) string {
	t.Helper()
	r := report(jr(a, "map1", "primario"), jr(b, "final", "primario"))
	r.RespostaCorreta = "certa ponte"
	return adv12EIngest(t, s, f, r)
}

func adv12EExport(t *testing.T, s *Store, parte string) ([]byte, []string) {
	t.Helper()
	var b bytes.Buffer
	if _, err := s.Export(ctx, parte, admin, &b, nil); err != nil {
		t.Fatalf("Export(%s): %v", parte, err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return b.Bytes(), out
}

type adv12ELine struct {
	Messages []map[string]any `json:"messages"`
	Meta     map[string]any   `json:"meta"`
}

// adv12ECheckLine: valid UTF-8, one JSON object with exactly messages and meta, meta with
// exactly the six fields of C-3.15 (all strings), messages not empty and the last one the
// assistant (C-3.13, C-3.14).
func adv12ECheckLine(l string) (adv12ELine, error) {
	var out adv12ELine
	if !utf8.ValidString(l) {
		return out, errors.New("linha não é UTF-8 válido")
	}
	dec := json.NewDecoder(strings.NewReader(l))
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return out, fmt.Errorf("não é JSON: %v", err)
	}
	if dec.More() {
		return out, errors.New("mais de um valor JSON na linha")
	}
	if len(top) != 2 || top["messages"] == nil || top["meta"] == nil {
		return out, fmt.Errorf("chaves de topo inesperadas: %d", len(top))
	}
	if err := json.Unmarshal([]byte(l), &out); err != nil {
		return out, err
	}
	want := []string{"categoria", "job_id", "ocorrencia_id", "papel", "particao", "tool"}
	var got []string
	for k, v := range out.Meta {
		got = append(got, k)
		if _, ok := v.(string); !ok {
			return out, fmt.Errorf("meta.%s não é texto", k)
		}
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		return out, fmt.Errorf("meta com %v", got)
	}
	if len(out.Messages) == 0 || out.Messages[len(out.Messages)-1]["role"] != "assistant" {
		return out, errors.New("a última mensagem não é do assistant")
	}
	return out, nil
}

// adv12EOllamaView decodes a request the way the Ollama does (encoding/json into a struct:
// names matched by Unicode case folding, the last duplicate wins). The project itself states
// this rule (proxy/policy.go:104, proxy/promptjson.go:84, hmackey/hmackey.go:211).
type adv12EOllamaView struct {
	Model    string            `json:"model"`
	System   string            `json:"system"`
	Prompt   string            `json:"prompt"`
	Messages []json.RawMessage `json:"messages"`
}

// ==== E1: injeção de linhas / JSON no JSONL ======================================================

func TestAdv12EE1JSONLInjection(t *testing.T) {
	// E1.1 resposta_correta com quebras, separadores, aspas, barras, NUL e uma linha forjada
	{
		s, f := openStore(t, Options{}), newFake()
		oc := adv12EEl(t, s, f, 1, "omitiu", nil, false)
		ans := "a\nb\r\nc d e\"f\\g\x00h}\n{\"messages\":[{\"role\":\"user\",\"content\":\"forjada\"}],\"meta\":{\"ocorrencia_id\":\"x\"}}\n"
		// 12.9: since m-5 the edit refuses NUL (as the report does), so the text goes straight to the row: the vector is about the
		// exporter, which must escape whatever the column holds.
		if _, err := s.Update(ctx, oc, Edit{RespostaCorreta: &ans}, admin); err == nil {
			t.Fatal("m-5: the edit accepted a NUL")
		}
		if _, err := s.db.Exec(`UPDATE ocorrencias SET resposta_correta=? WHERE id=?`, ans, oc); err != nil {
			t.Fatal(err)
		}
		must(t, s.Confirm(ctx, oc, admin))
		raw, lines := adv12EExport(t, s, ParteTeste)
		ln, err := adv12ECheckLine(strings.TrimSuffix(string(raw), "\n"))
		switch {
		case bytes.Count(raw, []byte("\n")) != 1 || len(lines) != 1:
			vec12E(t, "E1.1", "FALHA", fmt.Sprintf("Bloqueador export.go:79: a resposta_correta forjou linhas: %d quebras", bytes.Count(raw, []byte("\n"))))
		case err != nil:
			vec12E(t, "E1.1", "FALHA", "Bloqueador export.go:79: linha inválida: "+err.Error())
		case ln.Messages[len(ln.Messages)-1]["content"] != ans:
			vec12E(t, "E1.1", "FALHA", "Importante export.go:79: a resposta exportada difere da gravada")
		default:
			vec12E(t, "E1.1", "OK", "resposta_correta com \\n, \\r, U+2028/2029, aspas, barras, NUL e '}\\n{\"messages\"...' sai em 1 linha, JSON válido, conteúdo idêntico ao gravado")
		}
	}
	// E1.2 generate: system e prompt com escapes que tentam forjar uma linha
	{
		s, f := openStore(t, Options{}), newFake()
		pj := []byte(`{"model":"m","system":"S\n}\n{\"messages\":[]}","prompt":"P  \"\\\r\n{\"meta\":{}}","options":{"seed":1}}`)
		oc := adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj }, true)
		_ = oc
		raw, _ := adv12EExport(t, s, ParteTeste)
		var want adv12EOllamaView
		json.Unmarshal(pj, &want)
		ln, err := adv12ECheckLine(strings.TrimSuffix(string(raw), "\n"))
		switch {
		case bytes.Count(raw, []byte("\n")) != 1:
			vec12E(t, "E1.2", "FALHA", "Bloqueador export.go:71-76: o prompt forjou linhas")
		case err != nil:
			vec12E(t, "E1.2", "FALHA", "Bloqueador export.go:82: "+err.Error())
		case len(ln.Messages) != 3 || ln.Messages[0]["content"] != want.System || ln.Messages[1]["content"] != want.Prompt:
			vec12E(t, "E1.2", "FALHA", fmt.Sprintf("Importante export.go:60-77: system/user diferentes do pedido: %v", ln.Messages))
		default:
			vec12E(t, "E1.2", "OK", "generate com system/prompt cheios de escapes e linhas forjadas: 1 linha, [system,user,assistant] iguais ao pedido decodificado")
		}
	}
	// E1.3 chat: mensagens com U+2028 cru, role tool, tool_calls, content em partes (/v1) e objeto
	{
		s, f := openStore(t, Options{}), newFake()
		pj := []byte("{\"model\":\"m\",\"messages\":[{\"role\":\"system\",\"content\":\"S cru\"},{\"role\":\"user\",\"content\":\"u1\\n}\\n{\\\"x\\\":1}\"}," +
			"{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"f\",\"arguments\":{\"a\":1}}}]},{\"role\":\"tool\",\"content\":\"r\"}," +
			"{\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"partes\"}]},{\"role\":\"hacker\",\"content\":{\"o\":1}}]}")
		adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj; j.Endpoint, j.Tool = "/api/chat", "chat" }, true)
		raw, lines := adv12EExport(t, s, ParteTeste)
		var in struct {
			Messages []any `json:"messages"`
		}
		json.Unmarshal(pj, &in)
		ln, err := adv12ECheckLine(strings.TrimSuffix(string(raw), "\n"))
		var gotMsgs []any
		if err == nil {
			var o struct {
				Messages []any `json:"messages"`
			}
			json.Unmarshal([]byte(lines[0]), &o)
			gotMsgs = o.Messages
		}
		switch {
		case len(lines) != 1 || bytes.Count(raw, []byte("\n")) != 1:
			vec12E(t, "E1.3", "FALHA", "Bloqueador export.go:59: mensagens do chat forjaram linhas")
		case err != nil:
			vec12E(t, "E1.3", "FALHA", "Bloqueador export.go:59: "+err.Error())
		case len(gotMsgs) != len(in.Messages)+1 || !reflect.DeepEqual(gotMsgs[:len(in.Messages)], in.Messages):
			vec12E(t, "E1.3", "FALHA", "Importante export.go:59: o histórico do chat não é o do prompt_json")
		default:
			_ = ln
			vec12E(t, "E1.3", "OK", "MEDIDA chat com U+2028 cru, tool_calls, role tool, content em partes e objeto, role 'hacker': 1 linha válida, histórico idêntico + assistant; roles e content passam CRUS, sem validação (só leitura, Menor)")
		}
	}
	// E1.4 chat com UTF-8 inválido no conteúdo: o Ollama aceita (troca por U+FFFD); o JSONL?
	{
		s, f := openStore(t, Options{}), newFake()
		pj := []byte("{\"model\":\"m\",\"messages\":[{\"role\":\"user\",\"content\":\"abc\xff\xfedef\"}],\"think\":false}")
		var ov adv12EOllamaView
		ollamaOK := json.Unmarshal(pj, &ov) == nil
		// the policy, when it rewrites the body (map[string]json.RawMessage, policy.go), keeps the bytes
		var m map[string]json.RawMessage
		json.Unmarshal(pj, &m)
		rewritten, _ := json.Marshal(m)
		adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj; j.Endpoint, j.Tool = "/api/chat", "chat" }, true)
		raw, lines := adv12EExport(t, s, ParteTeste)
		switch {
		case len(lines) != 1:
			vec12E(t, "E1.4", "FALHA", fmt.Sprintf("Menor: fixture: %d linhas", len(lines)))
		case !utf8.Valid(raw):
			vec12E(t, "E1.4", "FALHA", fmt.Sprintf("Bloqueador export.go:56-59: a linha do chat sai com UTF-8 INVÁLIDO (json.RawMessage copiado cru do prompt_json); json.Valid=%v, mas JSON Lines/RFC 8259 exigem UTF-8 e o Content-Type diz charset=utf-8 (leitor estrito aborta o arquivo). O Ollama aceitou o pedido=%v; a política reescrita mantém os bytes=%v",
				json.Valid(bytes.TrimSpace(raw)), ollamaOK, !utf8.Valid(rewritten)))
		default:
			vec12E(t, "E1.4", "OK", "chat com UTF-8 inválido sai em UTF-8 válido")
		}
	}
	// E1.5 chave duplicada EXATA: o export e o Ollama escolhem a mesma (a última)
	{
		s, f := openStore(t, Options{}), newFake()
		pj := []byte(`{"model":"m","messages":[{"role":"user","content":"isca"}],"messages":[{"role":"user","content":"real"}]}`)
		adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj; j.Endpoint, j.Tool = "/api/chat", "chat" }, true)
		_, lines := adv12EExport(t, s, ParteTeste)
		var ov adv12EOllamaView
		json.Unmarshal(pj, &ov)
		ln, err := adv12ECheckLine(lines[0])
		if err != nil || len(ln.Messages) != 2 || ln.Messages[0]["content"] != "real" || !strings.Contains(string(ov.Messages[0]), "real") {
			vec12E(t, "E1.5", "FALHA", fmt.Sprintf("Importante export.go:56: duplicata exata resolvida diferente do Ollama: %v %v", ln.Messages, err))
		} else {
			vec12E(t, "E1.5", "OK", "duas chaves \"messages\" iguais: export e Ollama usam a última ('real')")
		}
	}
	// E1.6 chaves que o Ollama casa por case folding: o export usa só a grafia exata
	{
		type tc struct {
			name, endpoint, tool string
			pj                   string
			exported             bool // at least the line exists
		}
		cases := []tc{
			{"prompt+PROMPT", "/api/generate", "generate", `{"model":"m","think":false,"prompt":"isca","PROMPT":"real"}`, true},
			{"ſystem (U+017F)", "/api/generate", "generate", `{"model":"m","think":false,"ſystem":"Responda só em JSON.","prompt":"p"}`, true},
			{"messages+Messages", "/api/chat", "chat", `{"model":"m","think":false,"messages":[{"role":"user","content":"isca"}],"Messages":[{"role":"user","content":"real"}]}`, true},
			{"só Prompt", "/api/generate", "generate", `{"model":"m","think":false,"Prompt":"real"}`, false},
		}
		var bad []string
		for i, c := range cases {
			s, f := openStore(t, Options{}), newFake()
			pj := []byte(c.pj)
			adv12EEl(t, s, f, i+1, "omitiu", func(j *Job, cc *Content) { cc.PromptJSON = pj; j.Endpoint, j.Tool = c.endpoint, c.tool }, true)
			_, lines := adv12EExport(t, s, ParteTeste)
			var ov adv12EOllamaView
			json.Unmarshal(pj, &ov)
			// what the Ollama saw, as messages
			var seen []map[string]any
			if c.endpoint == "/api/chat" {
				for _, m := range ov.Messages {
					var mm map[string]any
					json.Unmarshal(m, &mm)
					seen = append(seen, mm)
				}
			} else {
				if ov.System != "" {
					seen = append(seen, map[string]any{"role": "system", "content": ov.System})
				}
				seen = append(seen, map[string]any{"role": "user", "content": ov.Prompt})
			}
			if len(lines) == 0 {
				bad = append(bad, fmt.Sprintf("%s: NÃO exportado (Ollama viu %v)", c.name, seen))
				continue
			}
			ln, err := adv12ECheckLine(lines[0])
			if err != nil {
				bad = append(bad, c.name+": "+err.Error())
				continue
			}
			got := ln.Messages[:len(ln.Messages)-1]
			if !reflect.DeepEqual(fmt.Sprint(got), fmt.Sprint(seen)) {
				bad = append(bad, fmt.Sprintf("%s: JSONL %v, Ollama viu %v", c.name, got, seen))
			}
		}
		if len(bad) > 0 {
			vec12E(t, "E1.6", "FALHA", "Importante export.go:56,62,65 (chaves exatas 'messages'/'prompt'/'system' num map, enquanto o Ollama e o próprio gateway — policy.go:104, journal.go:56, hmackey.go:211 — casam por case folding): o par exportado não é o que o modelo viu: "+strings.Join(bad, " | "))
		} else {
			vec12E(t, "E1.6", "OK", "chaves com outra grafia são resolvidas como o Ollama")
		}
	}
	// E1.7 mensagem final do assistant já presente (prefill) e campos que o modelo viu e o JSONL descarta
	{
		s, f := openStore(t, Options{}), newFake()
		pj := []byte(`{"model":"m","messages":[{"role":"user","content":"u"},{"role":"assistant","content":"Comece assim:"}]}`)
		adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj; j.Endpoint, j.Tool = "/api/chat", "chat" }, true)
		_, l1 := adv12EExport(t, s, ParteTeste)
		s2, f2 := openStore(t, Options{}), newFake()
		pj2 := []byte(`{"model":"m","prompt":"def soma(a, b):","suffix":"    return r","raw":true,"template":"{{ .Prompt }}","context":[1,2,3]}`)
		adv12EEl(t, s2, f2, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj2; j.Tool = "generate" }, true)
		_, l2 := adv12EExport(t, s2, ParteTeste)
		ln1, e1 := adv12ECheckLine(l1[0])
		ln2, e2 := adv12ECheckLine(l2[0])
		if e1 != nil || e2 != nil {
			vec12E(t, "E1.7", "FALHA", fmt.Sprintf("Bloqueador: linha inválida %v %v", e1, e2))
		} else {
			n := len(ln1.Messages)
			vec12E(t, "E1.7", "OK", fmt.Sprintf("MEDIDA (só leitura, Menor): chat com assistant final (prefill) exporta %d mensagens com 2 assistant seguidos=%v; generate com suffix/raw/template/context exporta só %d mensagens (suffix, template, raw e context, que mudam o que o modelo viu, são descartados em export.go:60-77)",
				n, n >= 2 && ln1.Messages[n-2]["role"] == "assistant", len(ln2.Messages)))
		}
	}
}

// ==== E2: o predicado único, uma condição por vez ================================================

// adv12EPaths says what every reader of the predicate says about one occurrence in part
// parte: lines exported, Summary count, Detail.Exportavel and whether CreateRetrato froze it.
func adv12EPaths(t *testing.T, s *Store, oc, parte string) (lines int, sum int, det bool, ret bool) {
	t.Helper()
	_, ls := adv12EExport(t, s, parte)
	lines = len(ls)
	sm, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if parte == ParteTeste {
		sum = sm.ExportTeste
	} else {
		sum = sm.ExportTreino
	}
	d, err := s.Detail(ctx, oc)
	if err == nil {
		det = d.Exportavel
	}
	if parte == ParteTeste {
		if rt, err := s.CreateRetrato(ctx, RetratoOptions{}); err == nil {
			for _, it := range rt.Itens {
				if it.OcorrenciaID == oc {
					ret = true
				}
			}
		}
	}
	return
}

func TestAdv12EE2SinglePredicate(t *testing.T) {
	type variant struct {
		id, name string
		sql      []string
		parte    string
	}
	vs := []variant{
		{"E2.1", "status=ok, http 200, resp_final=0", []string{`UPDATE amostras SET resp_final=0`}, ParteTeste},
		{"E2.2", "endpoint /api/embed", []string{`UPDATE amostras SET endpoint='/api/embed'`}, ParteTeste},
		{"E2.3", "endpoint /api/embeddings", []string{`UPDATE amostras SET endpoint='/api/embeddings'`}, ParteTeste},
		{"E2.4", "endpoint /v1/embeddings", []string{`UPDATE amostras SET endpoint='/v1/embeddings'`}, ParteTeste},
		{"E2.5", "endpoint /API/GENERATE", []string{`UPDATE amostras SET endpoint='/API/GENERATE'`}, ParteTeste},
		{"E2.6", "endpoint /api/generate/ (barra final)", []string{`UPDATE amostras SET endpoint='/api/generate/'`}, ParteTeste},
		{"E2.7", "status 'OK' (maiúsculo)", []string{`UPDATE amostras SET status='OK'`}, ParteTeste},
		{"E2.8", "http_status 201", []string{`UPDATE amostras SET http_status=201`}, ParteTeste},
		{"E2.9", "tool 'Generate'", []string{`UPDATE ocorrencias SET tool='Generate'`}, ParteTeste},
		{"E2.10", "tool 'bakeoff-erros'", []string{`UPDATE ocorrencias SET tool='bakeoff-erros'`}, ParteTeste},
		{"E2.11", "tool vazia", []string{`UPDATE ocorrencias SET tool=''`}, ParteTeste},
		{"E2.12", "translate papel reduce1", []string{`UPDATE ocorrencias SET tool='translate'`, `UPDATE ocorrencia_jobs SET papel='reduce1'`}, ParteTeste},
		{"E2.13", "translate papel final", []string{`UPDATE ocorrencias SET tool='translate'`, `UPDATE ocorrencia_jobs SET papel='final'`}, ParteTeste},
		{"E2.14", "translate papel map0", []string{`UPDATE ocorrencias SET tool='translate'`, `UPDATE ocorrencia_jobs SET papel='map0'`}, ParteTeste},
		{"E2.15", "translate papel mapX", []string{`UPDATE ocorrencias SET tool='translate'`, `UPDATE ocorrencia_jobs SET papel='mapX'`}, ParteTeste},
		{"E2.16", "translate papel map10000", []string{`UPDATE ocorrencias SET tool='translate'`, `UPDATE ocorrencia_jobs SET papel='map10000'`}, ParteTeste},
		{"E2.17", "translate sem linha em ocorrencia_jobs (papel vazio)", []string{`UPDATE ocorrencias SET tool='translate'`, `DELETE FROM ocorrencia_jobs`}, ParteTeste},
		{"E2.18", "amostra com melhora=1", []string{`UPDATE amostras SET melhora=1`}, ParteTeste},
		{"E2.19", "prompt_json vazio (blob de 0 bytes)", []string{`UPDATE amostras SET prompt_json=x''`}, ParteTeste},
		{"E2.20", "partição pendente (vazia)", []string{`UPDATE ocorrencias SET particao=''`}, ParteTeste},
		{"E2.21", "resposta só de espaços Unicode (NBSP)", []string{"UPDATE ocorrencias SET resposta_correta='  \t'"}, ParteTeste},
		{"E2.22", "estado apagada por SQL", []string{`UPDATE ocorrencias SET estado='apagada'`}, ParteTeste},
		{"E2.23", "tem_imagem=1 com o resto perfeito", []string{`UPDATE amostras SET tem_imagem=1`}, ParteTeste},
		{"E2.24", "quarentena", []string{`UPDATE ocorrencias SET particao='quarentena'`}, ParteTeste},
	}
	for _, v := range vs {
		s, f := openStore(t, Options{}), newFake()
		oc := adv12EEl(t, s, f, 1, "omitiu", nil, true)
		if l, sum, det, _ := adv12EPaths(t, s, oc, ParteTeste); l != 1 || sum != 1 || !det {
			vec12E(t, v.id, "FALHA", fmt.Sprintf("Menor: fixture: base não exportável (%d %d %v)", l, sum, det))
			continue
		}
		s.db.Exec(`DELETE FROM retratos`)
		s.db.Exec(`DELETE FROM retrato_itens`)
		for _, q := range v.sql {
			if _, err := s.db.Exec(q); err != nil {
				t.Fatal(v.id, err)
			}
		}
		lt, st, dt, rt := adv12EPaths(t, s, oc, ParteTeste)
		lr, sr, _, _ := adv12EPaths(t, s, oc, ParteTreino)
		if v.id == "E2.20" && lt+lr+st+sr == 0 && !rt {
			// a confirmed exportable occurrence is never left pending by recompute (partition.go:326-341):
			// the state exists only by SQL; Detail still calls it exportable (Eligible ignores the empty part)
			vec12E(t, v.id, "OK", fmt.Sprintf("partição pendente: fora do JSONL nas duas partes, do Summary e do retrato; MEDIDA (só leitura, Menor) Detail.exportavel=%v porque Eligible (partition.go:80) não exige partição atribuída; estado inalcançável pelo recompute", dt))
			continue
		}
		if lt+lr+st+sr != 0 || dt || rt {
			vec12E(t, v.id, "FALHA", fmt.Sprintf("Bloqueador partition.go:64: '%s' passou: linhas teste=%d treino=%d, Summary %d/%d, Detail.exportavel=%v, retrato=%v", v.name, lt, lr, st, sr, dt, rt))
		} else {
			vec12E(t, v.id, "OK", "'"+v.name+"' (só essa condição): fora do JSONL nas duas partes, do Summary, do Detail.exportavel e do retrato")
		}
	}
	// E2.25 melhora_* preenchido em UM dos campos, pelo caminho real do relato (Content -> amostra)
	{
		muts := map[string]func(c *Content){
			"melhora_instrucoes_id":               func(c *Content) { c.MelhoraInstrucoesID = "v1" },
			"melhora_indice_id":                   func(c *Content) { c.MelhoraIndiceID = "ix" },
			"melhora_exemplos":                    func(c *Content) { c.MelhoraExemplos = "a,b" },
			"prompt_json_original":                func(c *Content) { c.PromptJSONOriginal = []byte(`{"prompt":"x"}`) },
			"prompt_json_original vazio não-nulo": func(c *Content) { c.PromptJSONOriginal = []byte{} },
		}
		var leaked []string
		for name, mu := range muts {
			s, f := openStore(t, Options{}), newFake()
			oc := adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { mu(c) }, true)
			if l, sum, det, ret := adv12EPaths(t, s, oc, ParteTeste); l+sum != 0 || det || ret {
				leaked = append(leaked, name)
			}
		}
		if len(leaked) > 0 {
			sort.Strings(leaked)
			vec12E(t, "E2.25", "FALHA", "Bloqueador ingest.go:72 / partition.go:84: job alterado pela melhora exportado com só "+strings.Join(leaked, ","))
		} else {
			vec12E(t, "E2.25", "OK", "cada campo melhora_* sozinho (e prompt_json_original vazio não-nulo) marca a amostra: fora do JSONL, do Summary, do Detail e do retrato (C-3.10a)")
		}
	}
	// E2.26 a amostra exportada é a do job_do_erro, não a de outro job da ocorrência
	{
		s, f := openStore(t, Options{}), newFake()
		f.add(10)
		f.add(11)
		r := report(jr(10, "map1", "primario"), jr(11, "final", "primario"))
		r.JobDoErro, r.RespostaCorreta = mkID(10), "certa do map1"
		oc := adv12EIngest(t, s, f, r)
		must(t, s.Confirm(ctx, oc, admin))
		_, lines := adv12EExport(t, s, ParteTeste)
		var ln adv12ELine
		if len(lines) == 1 {
			ln, _ = adv12ECheckLine(lines[0])
		}
		if len(lines) != 1 || ln.Messages[0]["content"] != "pedido 10" || ln.Meta["job_id"] != mkID(10) || ln.Meta["papel"] != "map1" {
			vec12E(t, "E2.26", "FALHA", fmt.Sprintf("Bloqueador export.go:196: a linha não é a do job_do_erro: %v", lines))
		} else {
			// and without the job_do_erro's sample (other job has one) nothing goes out
			s.db.Exec(`DELETE FROM amostras WHERE job_id=?`, mkID(10))
			if l, sum, det, ret := adv12EPaths(t, s, oc, ParteTeste); l+sum != 0 || det || ret {
				vec12E(t, "E2.26", "FALHA", "Bloqueador partition.go:68: exportou com a amostra de outro job")
			} else {
				vec12E(t, "E2.26", "OK", "job_do_erro=map1 de uma lista map1+final: a linha usa o pedido do map1 (meta.job_id/papel certos); sem a amostra do map1, a do final não substitui")
			}
		}
	}
	// E2.27 amostra compartilhada: apagar uma das ocorrências não tira a amostra da outra (C-6.4)
	{
		s, f := openStore(t, Options{}), newFake()
		a := adv12EEl(t, s, f, 1, "omitiu", nil, true)
		r := report(jr(1, "unico", "primario"))
		r.RespostaCorreta = "outra resposta"
		b := adv12EIngest(t, s, f, r) // same origin, human: a second occurrence of the job
		must(t, s.Delete(ctx, b, admin))
		_, lines := adv12EExport(t, s, ParteTeste)
		if len(lines) != 1 || !strings.Contains(lines[0], a) {
			vec12E(t, "E2.27", "FALHA", fmt.Sprintf("Importante state.go:210: apagar a outra ocorrência tirou a amostra: %v", lines))
		} else {
			vec12E(t, "E2.27", "OK", "duas ocorrências do mesmo job; apagar uma deixa a amostra e a linha da outra (C-6.4)")
		}
	}
	// E2.28 key_id da amostra diferente do da loja: nenhuma condição do cap. 3 depende dele
	{
		s, f := openStore(t, Options{}), newFake()
		oc := adv12EEl(t, s, f, 1, "omitiu", nil, true)
		s.db.Exec(`UPDATE amostras SET key_id='ffffffffffffffff'`)
		l, sum, det, ret := adv12EPaths(t, s, oc, ParteTeste)
		vec12E(t, "E2.28", "OK", fmt.Sprintf("MEDIDA amostra com key_id de outra chave: linhas=%d Summary=%d Detail=%v retrato=%v (o cap. 3 não olha key_id; o retrato reconfere o prompt_hmac com a chave da loja)", l, sum, det, ret))
	}
	// E2.29 o predicado diz "exportável" e o JSONL não tem a linha: buildLine tem condições escondidas
	{
		type tc struct{ name, endpoint, tool, pj string }
		cases := []tc{
			{"generate sem prompt (o Ollama responde 200: só carrega o modelo)", "/api/generate", "generate", `{"model":"m"}`},
			{"generate com prompt vazio", "/api/generate", "generate", `{"model":"m","prompt":""}`},
			{"chat com messages []", "/api/chat", "chat", `{"model":"m","messages":[]}`},
		}
		var bad []string
		for i, c := range cases {
			s, f := openStore(t, Options{}), newFake()
			oc := adv12EEl(t, s, f, i+1, "omitiu", func(j *Job, cc *Content) { cc.PromptJSON = []byte(c.pj); j.Endpoint, j.Tool = c.endpoint, c.tool }, true)
			var cota int
			s.db.QueryRow(`SELECT COUNT(*) FROM cota_grupos`).Scan(&cota)
			l, sum, det, ret := adv12EPaths(t, s, oc, ParteTeste)
			if l == 0 && (sum > 0 || det || ret || cota > 0) {
				bad = append(bad, fmt.Sprintf("%s: JSONL 0 linhas, mas Summary=%d Detail.exportavel=%v retrato=%v cota_grupos=%d", c.name, sum, det, ret, cota))
			}
		}
		if len(bad) > 0 {
			vec12E(t, "E2.29", "FALHA", "Menor export.go:56,62 x partition.go:64-88 (C-3.1 'se e somente se'): o predicado único não é único; "+strings.Join(bad, " | ")+"; o grupo gasta vaga da cota (C-4.6) e entra no retrato sem nunca ir ao JSONL")
		} else {
			vec12E(t, "E2.29", "OK", "predicado e JSONL concordam")
		}
	}
}

// ==== E3: partição sem vazamento =================================================================

// adv12ECotaRow is one row of cota_grupos.
type adv12ECotaRow struct{ grupo, cat, part string }

// adv12ECheck verifies C-4.12 (no job, doc_hash+key_id, conteudo_hmac+key_id or group in the
// exportable samples of both parts), one side per component, quarantine out of the export, and
// C-4.6/C-4.11b on cota_grupos (the first 30 rows of a category are the test, the later test
// rows are those of the 15% rule, no row ever leaves or changes).
func adv12ECheck(t *testing.T, s *Store, seenCota map[string]adv12ECotaRow) string {
	t.Helper()
	var comps []component
	if err := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		comps, err = buildComponents(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range comps {
		sides := map[string]bool{}
		for _, m := range c.members {
			sides[m.particao] = true
		}
		if len(sides) > 1 {
			return fmt.Sprintf("componente com lados %v (%d membros)", sides, len(c.members))
		}
	}
	sets := map[string]map[string]string{}
	for _, parte := range []string{ParteTreino, ParteTeste} {
		_, lines := adv12EExport(t, s, parte)
		for _, l := range lines {
			ln, err := adv12ECheckLine(l)
			if err != nil {
				return "linha inválida: " + err.Error()
			}
			oc, job := ln.Meta["ocorrencia_id"].(string), ln.Meta["job_id"].(string)
			var part, grupo, kid, dh, ch string
			s.db.QueryRow(`SELECT particao, COALESCE(grupo_id,'') FROM ocorrencias WHERE id=?`, oc).Scan(&part, &grupo)
			s.db.QueryRow(`SELECT COALESCE(key_id,''), COALESCE(doc_hash,''), COALESCE(conteudo_hmac,'') FROM amostras WHERE job_id=?`, job).Scan(&kid, &dh, &ch)
			if part != parte {
				return fmt.Sprintf("%s exportado em %s com partição %q", oc, parte, part)
			}
			for _, k := range []string{"j:" + job, "g:" + grupo, "d:" + kid + ":" + dh, "c:" + kid + ":" + ch} {
				if strings.HasSuffix(k, ":") {
					continue
				}
				if sets[k] == nil {
					sets[k] = map[string]string{}
				}
				sets[k][parte] = oc
			}
		}
	}
	for k, v := range sets {
		if len(v) > 1 {
			return fmt.Sprintf("%s em treino (%s) e teste (%s)", k, v[ParteTreino], v[ParteTeste])
		}
	}
	rows, err := s.db.Query(`SELECT grupo_id, categoria, particao FROM cota_grupos ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tcount := map[string]int{}
	now := map[string]adv12ECotaRow{}
	for rows.Next() {
		var r adv12ECotaRow
		rows.Scan(&r.grupo, &r.cat, &r.part)
		now[r.grupo] = r
		if tcount[r.cat] < CotaPorCategoria {
			if r.part != ParteTeste {
				return fmt.Sprintf("cota: o grupo %s da categoria %s foi ao treino com só %d vagas usadas", r.grupo, r.cat, tcount[r.cat])
			}
		} else if r.part != hashSide(r.grupo) {
			return fmt.Sprintf("cota EXCEDIDA: grupo %s de %s em %s depois de 30 vagas, e a regra dos 15%% diz %s", r.grupo, r.cat, r.part, hashSide(r.grupo))
		}
		if r.part == ParteTeste {
			tcount[r.cat]++
		}
	}
	for g, r := range seenCota {
		if now[g] != r {
			return fmt.Sprintf("cota LIBERADA/alterada: %v virou %v", r, now[g])
		}
	}
	for g, r := range now {
		seenCota[g] = r
	}
	return ""
}

func TestAdv12EE3PartitionFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(20261104))
	s, f := openStore(t, Options{}), newFake()
	k := testKey(t)
	const jobs = 500
	for i := 1; i <= jobs; i++ {
		f.add(i)
		j := f.jobs[mkID(i)]
		switch i % 11 {
		case 0:
			j.DocHash, j.DocKeyID = fmt.Sprintf("%064x", i%23), k.ID()
		case 3:
			j.DocHash, j.DocKeyID = fmt.Sprintf("%064x", i%23), "ffffffffffffffff" // same hash, another key: must not link with case 0
		}
		f.jobs[mkID(i)] = j
		if i > 460 {
			c := f.conts[mkID(i)]
			c.PromptJSON = pjOf(i - 400) // same content as an earlier job
			f.conts[mkID(i)] = c
		}
	}
	cats := []string{"omitiu", "omitiu", "omitiu", "omitiu", "inventou"}
	var live []string
	seenCota := map[string]adv12ECotaRow{}
	quar, releases, deletes, steps := 0, 0, 0, 1400
	fail := ""
	for step := 1; step <= steps && fail == ""; step++ {
		switch op := rng.Intn(11); {
		case op < 4 || len(live) < 6:
			a := rng.Intn(jobs) + 1
			list := []JobRef{jr(a, "unico", "primario")}
			if b := rng.Intn(jobs) + 1; b != a && rng.Intn(2) == 0 {
				list = []JobRef{jr(b, "map1", "primario"), jr(a, "final", "primario")}
			}
			r := report(list...)
			r.Categoria, r.RespostaCorreta = cats[rng.Intn(len(cats))], "certa"
			id := adv12EIngest(t, s, f, r)
			live = append(live, id)
			if sideOf(t, s, id) == ParteQuarentena {
				quar++
			}
		case op < 7:
			s.Confirm(ctx, live[rng.Intn(len(live))], admin)
		case op < 8:
			i := rng.Intn(len(live))
			if s.Delete(ctx, live[i], admin) == nil {
				deletes++
			}
			live = append(live[:i], live[i+1:]...)
		case op < 9:
			s.Discard(ctx, live[rng.Intn(len(live))], admin)
		case op < 10:
			nv := fmt.Sprintf("ajuste %d", step)
			s.Update(ctx, live[rng.Intn(len(live))], Edit{RespostaCorreta: &nv}, admin)
		default:
			var q []string
			for _, id := range live {
				if sideOf(t, s, id) == ParteQuarentena {
					q = append(q, id)
				}
			}
			if len(q) > 0 {
				lado := []string{ParteTreino, ParteTeste}[rng.Intn(2)]
				if s.ReleaseQuarantine(ctx, q[rng.Intn(len(q))], lado, admin) == nil {
					releases++
				}
			}
		}
		if step%20 == 0 {
			if msg := adv12ECheck(t, s, seenCota); msg != "" {
				fail = fmt.Sprintf("passo %d: %s", step, msg)
			}
		}
	}
	if fail == "" {
		fail = adv12ECheck(t, s, seenCota)
	}
	cotaMax := 0
	for _, c := range cats {
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM cota_grupos WHERE categoria=?`, c).Scan(&n)
		if n > cotaMax {
			cotaMax = n
		}
	}
	// sanity of the checker: a forced leak (an exported test occurrence given the group of an
	// exported training one, by SQL) must be seen; otherwise an OK above would mean nothing
	sane := false
	if fail == "" {
		_, lt := adv12EExport(t, s, ParteTreino)
		_, le := adv12EExport(t, s, ParteTeste)
		if len(lt) > 0 && len(le) > 0 {
			var a, b adv12ELine
			json.Unmarshal([]byte(lt[0]), &a)
			json.Unmarshal([]byte(le[0]), &b)
			s.db.Exec(`UPDATE ocorrencias SET grupo_id=? WHERE id=?`, gidOf(t, s, a.Meta["ocorrencia_id"].(string)), b.Meta["ocorrencia_id"])
			sane = adv12ECheck(t, s, seenCota) != ""
		}
	}
	switch {
	case fail == "" && !sane:
		vec12E(t, "E3.1", "FALHA", "Menor: fixture: o verificador não viu um vazamento forçado")
	case fail != "":
		vec12E(t, "E3.1", "FALHA", "Bloqueador partition.go:276 (recompute): "+fail)
	case quar == 0 || releases == 0 || deletes == 0 || cotaMax <= CotaPorCategoria:
		vec12E(t, "E3.1", "FALHA", fmt.Sprintf("Menor: fixture: o cenário não exercitou (quarentenas %d, liberações %d, apagados %d, cota máx %d)", quar, releases, deletes, cotaMax))
	default:
		vec12E(t, "E3.1", "OK", fmt.Sprintf("MEDIDA fuzz semente 20261104, %d passos (relatos com ponte, doc_hash com 2 key_id, conteúdo repetido, confirmar/desconfirmar/descartar/apagar/liberar): a cada 20 passos nenhum job/doc_hash+key_id/conteudo_hmac+key_id/grupo nos dois lados, 1 lado por componente, cota (30 primeiros = teste, depois só a regra dos 15%%) nunca excedida nem liberada; %d relatos caíram em quarentena, %d liberações, %d apagamentos, até %d linhas de cota numa categoria; o verificador acusou um vazamento forçado por SQL (prova de que o OK não é vazio)", steps, quar, releases, deletes, cotaMax))
	}
}

// adv12ETreinoGroup returns a confirmed occurrence that landed in the TRAINING part by the real
// path: the quota of the category is filled first (30 groups), then new groups are made until
// the 15% rule sends one to the training part. n is the next free job number.
func adv12ETreinoGroup(t *testing.T, s *Store, f *fakeSrc, n *int, cat string) string {
	t.Helper()
	for i := 0; i < 400; i++ {
		*n++
		oc := adv12EEl(t, s, f, *n, cat, nil, true)
		if sideOf(t, s, oc) == ParteTreino {
			return oc
		}
	}
	t.Fatal("no group went to the training part")
	return ""
}

func TestAdv12EE3KeyIDAndSplitRelink(t *testing.T) {
	// E3.2 ligação por doc_hash só com o mesmo key_id
	{
		s, f := openStore(t, Options{}), newFake()
		k := testKey(t)
		d := fmt.Sprintf("%064x", 77)
		set := func(j *Job, kid string) { j.DocHash, j.DocKeyID = d, kid }
		a := adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { set(j, k.ID()) }, false)
		b := adv12EEl(t, s, f, 2, "omitiu", func(j *Job, c *Content) { set(j, "ffffffffffffffff") }, false)
		c := adv12EEl(t, s, f, 3, "omitiu", func(j *Job, c *Content) { set(j, k.ID()) }, false)
		ga, gb, gc := gidOf(t, s, a), gidOf(t, s, b), gidOf(t, s, c)
		if ga == gb || ga != gc {
			vec12E(t, "E3.2", "FALHA", fmt.Sprintf("Bloqueador partition.go:150-181: ligação por doc_hash ignora o key_id: %s %s %s", ga, gb, gc))
		} else {
			vec12E(t, "E3.2", "OK", "mesmo doc_hash: com key_id igual liga (mesmo grupo), com key_id diferente não liga (C-4.3a)")
		}
	}
	// E3.3 divisão de grupo e nova ligação que reúne as partes com lados diferentes
	{
		s, f := openStore(t, Options{}), newFake()
		n := 100
		a := adv12ETreinoGroup(t, s, f, &n, "omitiu") // treino
		ja := n
		n++
		jb := n
		b := adv12EEl(t, s, f, jb, "inventou", nil, true) // teste (quota of 'inventou' is free)
		d := adv12EBridge(t, s, f, ja, jb)
		q1 := sideOf(t, s, a) == ParteQuarentena && sideOf(t, s, b) == ParteQuarentena && sideOf(t, s, d) == ParteQuarentena
		must(t, s.Delete(ctx, d, admin)) // the bridge goes: two parts, both still in quarantine
		q2 := sideOf(t, s, a) == ParteQuarentena && sideOf(t, s, b) == ParteQuarentena && gidOf(t, s, a) != gidOf(t, s, b)
		must(t, s.ReleaseQuarantine(ctx, a, ParteTreino, admin))
		must(t, s.ReleaseQuarantine(ctx, b, ParteTeste, admin))
		q3 := sideOf(t, s, a) == ParteTreino && sideOf(t, s, b) == ParteTeste
		e := adv12EBridge(t, s, f, ja, jb) // reunites the parts, now on opposite sides
		q4 := sideOf(t, s, a) == ParteQuarentena && sideOf(t, s, b) == ParteQuarentena && sideOf(t, s, e) == ParteQuarentena
		_, lt := adv12EExport(t, s, ParteTreino)
		_, le := adv12EExport(t, s, ParteTeste)
		var leak []string
		for _, l := range append(lt, le...) {
			if strings.Contains(l, a) || strings.Contains(l, b) {
				leak = append(leak, l[:40])
			}
		}
		if !(q1 && q2 && q3 && q4) || len(leak) > 0 {
			vec12E(t, "E3.3", "FALHA", fmt.Sprintf("Bloqueador partition.go:305-321: divisão+religação: %v %v %v %v, exportados %v", q1, q2, q3, q4, leak))
		} else {
			vec12E(t, "E3.3", "OK", "treino+teste ligados -> quarentena; apagar a ponte divide em 2 partes ainda em quarentena (ids distintos); liberar uma p/ treino e outra p/ teste; nova ponte -> quarentena de novo, nada exportado")
		}
	}
}

func TestAdv12EE3LeakAcrossTime(t *testing.T) {
	// E3.4 (M-1 da 12.6) liberar para o TESTE um componente cujo membro já foi exportado para o TREINO
	{
		s, f := openStore(t, Options{}), newFake()
		n := 100
		a := adv12ETreinoGroup(t, s, f, &n, "omitiu")
		ja := n
		_, lt := adv12EExport(t, s, ParteTreino) // the training file goes out with a
		inTreino := false
		for _, l := range lt {
			if strings.Contains(l, a) {
				inTreino = true
			}
		}
		n++
		jb := n
		adv12EEl(t, s, f, jb, "inventou", nil, true)
		adv12EBridge(t, s, f, ja, jb)
		info, err := s.ReleaseQuarantineInfo(ctx, a, ParteTeste, admin)
		// 12.9: C-4.11c (1.1.6) refuses this release; the case must not reach the snapshot either way
		if err != nil && !errors.Is(err, ErrReleaseExported) {
			t.Fatal(err)
		}
		rt, err := s.CreateRetrato(ctx, RetratoOptions{})
		inRet := false
		if err == nil {
			for _, it := range rt.Itens {
				if it.OcorrenciaID == a {
					inRet = true
				}
			}
		}
		if inTreino && inRet {
			vec12E(t, "E3.4", "FALHA", fmt.Sprintf("Importante partition.go:434-494 (lacuna de contrato C-4.11 x C-4.12/C-5.1): o caso %s foi ao arquivo de TREINO, a quarentena foi liberada para o TESTE (só aviso: exportados_na_outra_parte=%d) e o CreateRetrato o congelou como caso de avaliação; a C-4.12 vale só 'ao mesmo tempo', mas a avaliação mede um caso que pode ter sido treinado. Sugestão: recusar liberar para o teste (ou excluir do retrato) membro com 'exportou' campo=treino", a[:8], info.ExportadosNaOutraParte))
		} else {
			vec12E(t, "E3.4", "OK", fmt.Sprintf("caso exportado ao treino não entra no retrato após liberar ao teste (treino=%v retrato=%v)", inTreino, inRet))
		}
	}
	// E3.5 apagar e relatar de novo o MESMO job: o novo grupo pode cair no outro lado, sem aviso
	{
		s, f := openStore(t, Options{}), newFake()
		n := 100
		a := adv12ETreinoGroup(t, s, f, &n, "omitiu")
		ja := n
		_, lt := adv12EExport(t, s, ParteTreino)
		inTreino := false
		for _, l := range lt {
			if strings.Contains(l, mkID(ja)) {
				inTreino = true
			}
		}
		must(t, s.Delete(ctx, a, admin)) // the sample goes too (nobody else uses it)
		r := report(jr(ja, "unico", "primario"))
		r.Categoria, r.RespostaCorreta = "inventou", "certa de novo"
		a2 := adv12EIngest(t, s, f, r) // the job is still in requests and content.db (7 days)
		must(t, s.Confirm(ctx, a2, admin))
		_, le := adv12EExport(t, s, ParteTeste)
		inTeste := false
		for _, l := range le {
			if strings.Contains(l, mkID(ja)) {
				inTeste = true
			}
		}
		rt, _ := s.CreateRetrato(ctx, RetratoOptions{})
		inRet := false
		for _, it := range rt.Itens {
			if it.JobID == mkID(ja) {
				inRet = true
			}
		}
		if inTreino && (inTeste || inRet) {
			vec12E(t, "E3.5", "FALHA", fmt.Sprintf("Importante partition.go:131-133 (buildComponents ignora apagadas, C-4.5) + state.go:189: o job %s saiu no arquivo de TREINO, a ocorrência foi apagada, o mesmo job foi relatado de novo e caiu no TESTE (JSONL teste=%v, retrato=%v), sem nenhum aviso; o mesmo job_id em treino e teste ao longo do tempo (C-4.12 só 'ao mesmo tempo'). Sugestão: guardar os job_id/doc/conteúdo exportados (hash) e herdar o lado", mkID(ja)[:8], inTeste, inRet))
		} else {
			vec12E(t, "E3.5", "OK", fmt.Sprintf("relatar de novo um job já exportado mantém o lado (treino=%v teste=%v retrato=%v)", inTreino, inTeste, inRet))
		}
	}
}

func TestAdv12EE3CotaShownAboveThirty(t *testing.T) {
	// E3.6 a cota por categoria do resumo conta também os grupos de teste da regra dos 15%
	s, f := openStore(t, Options{}), newFake()
	n, hashTeste := 0, false
	for i := 0; i < 400 && !hashTeste; i++ {
		n++
		oc := adv12EEl(t, s, f, n, "omitiu", nil, true)
		if i >= CotaPorCategoria && sideOf(t, s, oc) == ParteTeste {
			hashTeste = true
		}
	}
	sm, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var quotaRows int
	s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT particao FROM cota_grupos WHERE categoria='omitiu' ORDER BY rowid LIMIT 30) WHERE particao='teste'`).Scan(&quotaRows)
	switch {
	case !hashTeste:
		vec12E(t, "E3.6", "FALHA", "Menor: fixture: nenhum grupo foi ao teste pela regra dos 15%")
	case sm.Cota["omitiu"] > CotaPorCategoria:
		vec12E(t, "E3.6", "FALHA", fmt.Sprintf("Menor query.go:96 (Summary.Cota 'cota_teste_por_categoria'): mostra %d para 'omitiu', acima da cota de 30 (as 30 vagas da cota são %d; o resto é a regra dos 15%% da C-4.7, que também grava em cota_grupos, partition.go:366). A cota em si não foi excedida (E3.1), mas o JSON do /monitor/api/state (st.Treino, monitor.go:440) diz que foi; o index.html não mostra o campo", sm.Cota["omitiu"], quotaRows))
	default:
		vec12E(t, "E3.6", "OK", fmt.Sprintf("resumo da cota = %d", sm.Cota["omitiu"]))
	}
}

// ==== E4: quarentena =============================================================================

func TestAdv12EE4Quarantine(t *testing.T) {
	s, f := openStore(t, Options{}), newFake()
	n := 100
	a := adv12ETreinoGroup(t, s, f, &n, "omitiu")
	ja := n
	// a is a public example with its embedding, in a frozen index
	must(t, s.SetExemploPublico(ctx, a, true, admin))
	pend, _ := s.PendingEmbeddings(ctx, 100)
	for _, p := range pend {
		if p.OcorrenciaID == a {
			must(t, s.SetEmbedding(ctx, a, p.Version, []float32{1, 0.5, 0.25}, adv12EModel))
		}
	}
	ix, err := s.FreezeIndex(ctx, FreezeOptions{K: 2, Limiar: 0.5, Modelo: adv12EModel, Formato: "f1"})
	if err != nil {
		t.Fatal(err)
	}
	n++
	jb := n
	b := adv12EEl(t, s, f, jb, "inventou", nil, true) // teste
	rt, err := s.CreateRetrato(ctx, RetratoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d := adv12EBridge(t, s, f, ja, jb)
	inQ := sideOf(t, s, a) == ParteQuarentena && sideOf(t, s, b) == ParteQuarentena && sideOf(t, s, d) == ParteQuarentena
	rtv, _ := s.GetRetrato(ctx, rt.ID)
	im, _ := s.GetIndexMeta(ctx, ix.ID)
	_, lt := adv12EExport(t, s, ParteTreino)
	_, le := adv12EExport(t, s, ParteTeste)
	leaked := 0
	for _, l := range append(append([]string{}, lt...), le...) {
		if strings.Contains(l, a) || strings.Contains(l, b) || strings.Contains(l, d) {
			leaked++
		}
	}
	sm, _ := s.Summary(ctx)
	rt2, errRet := s.CreateRetrato(ctx, RetratoOptions{})
	for _, it := range rt2.Itens {
		if it.OcorrenciaID == a || it.OcorrenciaID == b || it.OcorrenciaID == d {
			leaked++
		}
	}
	_, errIx := s.FreezeIndex(ctx, FreezeOptions{K: 2, Limiar: 0.5, Modelo: adv12EModel, Formato: "f1"})
	if !inQ || rtv.Valido || im.Valido || leaked != 0 || sm.ExportTreino != len(lt) || sm.ExportTeste != len(le) || errRet != nil || !errors.Is(errIx, ErrNoExamples) {
		vec12E(t, "E4.1", "FALHA", fmt.Sprintf("Bloqueador partition.go:307/396: quarentena q=%v retrato.valido=%v indice.valido=%v vazados=%d linhas=%d/%d summary=%d/%d retrato_novo=%v indice_novo=%v",
			inQ, rtv.Valido, im.Valido, leaked, len(lt), len(le), sm.ExportTreino, sm.ExportTeste, errRet, errIx))
	} else {
		vec12E(t, "E4.1", "OK", "treino+teste ligados: componente em quarentena; retrato e índice congelado com membros dele -> valido=0; fora do JSONL, do Summary, de um retrato novo e de um índice novo (C-1.15, C-4.10)")
	}
	// E4.2 liberar com lado inválido, fora de quarentena, apagada, inexistente
	n++
	other := adv12EEl(t, s, f, n, "calculo_errado", nil, true) // teste, not quarantined
	var h0 int
	s.db.QueryRow(`SELECT COUNT(*) FROM historico`).Scan(&h0)
	var bad []string
	for _, lado := range []string{"", "quarentena", "TREINO", "Teste", " treino", "pendente"} {
		if err := s.ReleaseQuarantine(ctx, a, lado, admin); !errors.Is(err, ErrInvalid) {
			bad = append(bad, fmt.Sprintf("lado %q: %v", lado, err))
		}
	}
	if err := s.ReleaseQuarantine(ctx, other, ParteTreino, admin); !errors.Is(err, ErrTransition) {
		bad = append(bad, fmt.Sprintf("fora da quarentena: %v", err))
	}
	if err := s.ReleaseQuarantine(ctx, mkID(999999), ParteTreino, admin); !errors.Is(err, ErrNotFound) {
		bad = append(bad, fmt.Sprintf("inexistente: %v", err))
	}
	var h1 int
	s.db.QueryRow(`SELECT COUNT(*) FROM historico`).Scan(&h1)
	if sideOf(t, s, a) != ParteQuarentena || sideOf(t, s, other) != ParteTeste || h1 != h0 {
		bad = append(bad, fmt.Sprintf("algo mudou: %s %s hist %d->%d", sideOf(t, s, a), sideOf(t, s, other), h0, h1))
	}
	must(t, s.Delete(ctx, d, admin)) // a and b, now apart, stay in quarantine
	n++
	f.add(n)
	dq := adv12EBridge(t, s, f, ja, n) // d2: a quarantined member, erased next
	if sideOf(t, s, dq) != ParteQuarentena {
		bad = append(bad, "fixture: a ponte d2 não entrou na quarentena")
	}
	must(t, s.Delete(ctx, dq, admin))
	if err := s.ReleaseQuarantine(ctx, dq, ParteTreino, admin); !errors.Is(err, ErrTransition) {
		bad = append(bad, fmt.Sprintf("apagada: %v", err))
	}
	if len(bad) > 0 {
		vec12E(t, "E4.2", "FALHA", "Importante partition.go:434-457: "+strings.Join(bad, "; "))
	} else {
		vec12E(t, "E4.2", "OK", "liberar com lado inválido (6 grafias) -> ErrInvalid; fora da quarentena e de ocorrência apagada -> ErrTransition; inexistente -> ErrNotFound; nada muda, nenhum histórico")
	}
	// E4.3 depois de liberar, o retrato e o índice continuam inválidos
	must(t, s.ReleaseQuarantine(ctx, a, ParteTreino, admin))
	must(t, s.ReleaseQuarantine(ctx, b, ParteTeste, admin))
	rtv, _ = s.GetRetrato(ctx, rt.ID)
	im, _ = s.GetIndexMeta(ctx, ix.ID)
	_, _, errCases := s.RetratoCases(ctx, rt.ID)
	_, errLoad := s.LoadIndex(ctx, ix.ID)
	var hp int
	s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE ocorrencia_id=? AND acao='alterou' AND campo='particao' AND depois='treino' AND quem='admin'`, a).Scan(&hp)
	if rtv.Valido || im.Valido || !errors.Is(errCases, ErrRetratoInvalido) || hp != 1 {
		vec12E(t, "E4.3", "FALHA", fmt.Sprintf("Bloqueador retrato.go:216/indice.go:327: depois de liberar: retrato.valido=%v indice.valido=%v cases=%v load=%v hist=%d", rtv.Valido, im.Valido, errCases, errLoad, hp))
	} else {
		vec12E(t, "E4.3", "OK", fmt.Sprintf("liberar não revalida: retrato valido=0 (RetratoCases recusa), índice valido=0 (LoadIndex: %v); histórico 'alterou/particao' por admin", errLoad))
	}
}

// ==== E5: export write-ahead e streaming =========================================================

type adv12EFailW struct {
	bytes.Buffer
	failAt, n int
	after     func(n int)
}

func (w *adv12EFailW) Write(p []byte) (int, error) {
	w.n++
	if w.failAt > 0 && w.n >= w.failAt {
		return 0, errors.New("cliente saiu")
	}
	k, err := w.Buffer.Write(p)
	if w.after != nil {
		w.after(w.n)
	}
	return k, err
}

func adv12ESeed(t *testing.T, s *Store, f *fakeSrc, n int, parte string) []string {
	t.Helper()
	var ids []string
	for i := 1; i <= n; i++ {
		id := adv12EEl(t, s, f, i, "omitiu", nil, true)
		s.db.Exec(`UPDATE ocorrencias SET particao=? WHERE id=?`, parte, id)
		ids = append(ids, id)
	}
	return ids
}

func adv12EDelivered(t *testing.T, b []byte) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" {
			continue
		}
		var x struct {
			Meta exportMeta `json:"meta"`
		}
		if err := json.Unmarshal([]byte(l), &x); err != nil {
			t.Fatal(err)
		}
		out = append(out, x.Meta.OcorrenciaID)
	}
	return out
}

func adv12EExportRows(t *testing.T, s *Store) map[string]int {
	t.Helper()
	rows, err := s.db.Query(`SELECT ocorrencia_id, COUNT(*) FROM historico WHERE acao='exportou' AND campo='treino' AND quem='admin' AND ip='10.0.0.9' GROUP BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var c int
		rows.Scan(&id, &c)
		out[id] = c
	}
	return out
}

func TestAdv12EE5WriteAhead(t *testing.T) {
	const total = 45
	// E5.1 writer que falha na linha k: toda linha entregue tem `exportou`; o excesso fica no lote
	var meas []string
	bad := ""
	for _, k := range []int{1, 2, 20, 21, 22, 40, 41, 45} {
		s, f := openStore(t, Options{}), newFake()
		adv12ESeed(t, s, f, total, ParteTreino)
		w := &adv12EFailW{failAt: k}
		got, err := s.Export(ctx, ParteTreino, admin, w, nil)
		del := adv12EDelivered(t, w.Bytes())
		rows := adv12EExportRows(t, s)
		var rec int
		for _, c := range rows {
			rec += c
		}
		for _, id := range del {
			if rows[id] != 1 {
				bad = fmt.Sprintf("k=%d: linha entregue de %s sem `exportou`", k, id)
			}
		}
		limit := ((k-1)/exportBatch + 1) * exportBatch
		if limit > total {
			limit = total
		}
		if err == nil || got != k-1 || len(del) != k-1 || rec > limit || rec < len(del) {
			bad = fmt.Sprintf("k=%d: n=%d err=%v entregues=%d registros=%d (limite %d)", k, got, err, len(del), rec, limit)
		}
		meas = append(meas, fmt.Sprintf("k=%d:%d/%d", k, len(del), rec))
	}
	if bad != "" {
		vec12E(t, "E5.1", "FALHA", "Importante export.go:147-176: "+bad)
	} else {
		vec12E(t, "E5.1", "OK", "MEDIDA writer falhando na linha k (entregues/registradas): "+strings.Join(meas, " ")+"; nenhuma linha entregue sem `exportou`, excesso só até o fim do lote de 20 (write-ahead)")
	}
	// E5.2 contexto cancelado antes e durante
	{
		s, f := openStore(t, Options{}), newFake()
		adv12ESeed(t, s, f, total, ParteTreino)
		c0, cancel0 := context.WithCancel(context.Background())
		cancel0()
		var b bytes.Buffer
		n0, err0 := s.Export(c0, ParteTreino, admin, &b, nil)
		r0 := len(adv12EExportRows(t, s))
		c1, cancel1 := context.WithCancel(context.Background())
		w := &adv12EFailW{after: func(n int) {
			if n == 5 {
				cancel1()
			}
		}}
		n1, err1 := s.Export(c1, ParteTreino, admin, w, nil)
		cancel1()
		del := adv12EDelivered(t, w.Bytes())
		rows := adv12EExportRows(t, s)
		okRows := true
		for _, id := range del {
			if rows[id] != 1 {
				okRows = false
			}
		}
		if err0 == nil || n0 != 0 || b.Len() != 0 || r0 != 0 || err1 == nil || n1 != len(del) || !okRows || len(rows) != len(del) {
			vec12E(t, "E5.2", "FALHA", fmt.Sprintf("Importante export.go:125/182: cancelado antes n=%d err=%v linhas=%d reg=%d; durante n=%d err=%v entregues=%d reg=%d", n0, err0, b.Len(), r0, n1, err1, len(del), len(rows)))
		} else {
			vec12E(t, "E5.2", "OK", fmt.Sprintf("MEDIDA ctx cancelado antes: 0 linhas, 0 registros, erro; cancelado na 5ª linha: o lote em curso termina (%d linhas) e cada linha entregue tem o seu registro (%d), depois erro %v", len(del), len(rows), err1))
		}
	}
	// E5.3 ExportUntil com prazo já vencido; e C-7.7 em duas exportações seguidas
	{
		s, f := openStore(t, Options{}), newFake()
		ids := adv12ESeed(t, s, f, 3, ParteTreino)
		var b bytes.Buffer
		n0, err0 := s.ExportUntil(context.Background(), ParteTreino, admin, &b, nil, time.Now().Add(-time.Second))
		r0 := len(adv12EExportRows(t, s))
		adv12EExport(t, s, ParteTreino)
		adv12EExport(t, s, ParteTreino)
		rows := adv12EExportRows(t, s)
		var other int
		s.db.QueryRow(`SELECT COUNT(*) FROM historico WHERE acao='exportou' AND (campo!='treino' OR quem!='admin')`).Scan(&other)
		ok := true
		for _, id := range ids {
			if rows[id] != 2 {
				ok = false
			}
		}
		if !errors.Is(err0, ErrExportDeadline) || n0 != 0 || r0 != 0 || !ok || other != 0 {
			vec12E(t, "E5.3", "FALHA", fmt.Sprintf("Menor export.go:178/155: prazo vencido n=%d err=%v reg=%d; duas exportações rows=%v outros=%d", n0, err0, r0, rows, other))
		} else {
			vec12E(t, "E5.3", "OK", "ExportUntil com prazo vencido: 0 linhas, 0 registros, ErrExportDeadline; duas exportações = 2 linhas `exportou` por ocorrência, campo=treino, quem=admin, ip certo (C-7.7)")
		}
	}
}

type adv12ESlowW struct {
	once    sync.Once
	started chan struct{}
	gate    chan struct{}
	bytes.Buffer
}

func (w *adv12ESlowW) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	select {
	case <-w.gate:
	case <-time.After(20 * time.Second):
	}
	return w.Buffer.Write(p)
}

func TestAdv12EE5BigSampleSlowReader(t *testing.T) {
	// E5.4 amostra de ~12 MiB e um cliente lento: o relato e as ações do admin não esperam
	s, f := openStore(t, Options{}), newFake()
	big := strings.Repeat("texto do documento ", 12<<20/19)
	pj, _ := json.Marshal(map[string]any{"model": "m", "prompt": big})
	oc := adv12EEl(t, s, f, 1, "omitiu", func(j *Job, c *Content) { c.PromptJSON = pj }, true)
	w := &adv12ESlowW{started: make(chan struct{}), gate: make(chan struct{})}
	done := make(chan error, 1)
	t0 := time.Now()
	go func() {
		_, err := s.Export(ctx, ParteTeste, admin, w, nil)
		done <- err
	}()
	select {
	case <-w.started:
	case <-time.After(30 * time.Second):
		t.Fatal("export did not start writing")
	}
	build := time.Since(t0)
	t1 := time.Now()
	oc2 := adv12EEl(t, s, f, 2, "inventou", nil, true) // report + confirm while the export is stuck
	_, errU := s.Update(ctx, oc2, Edit{Descricao: func() *string { x := "nova desc"; return &x }()}, admin)
	writeLat := time.Since(t1)
	close(w.gate)
	err := <-done
	line := strings.TrimSuffix(w.String(), "\n")
	ln, errL := adv12ECheckLine(line)
	switch {
	case err != nil || errU != nil || errL != nil:
		vec12E(t, "E5.4", "FALHA", fmt.Sprintf("Importante export.go:147: export=%v update=%v linha=%v", err, errU, errL))
	case writeLat > 5*time.Second:
		vec12E(t, "E5.4", "FALHA", fmt.Sprintf("Importante export.go:147-176: relato+confirmar+editar levaram %v com um export de 12 MiB parado no cliente", writeLat))
	case ln.Messages[0]["content"] != big || ln.Meta["ocorrencia_id"] != oc:
		vec12E(t, "E5.4", "FALHA", "Importante export.go:39: a linha grande saiu diferente")
	default:
		vec12E(t, "E5.4", "OK", fmt.Sprintf("MEDIDA amostra de %d bytes: montar a linha levou %v; com o cliente parado na escrita, relato+confirmar+editar de outra ocorrência levaram %v (a vez de escrever não fica presa); linha de %d bytes íntegra", len(pj), build.Round(time.Millisecond), writeLat.Round(time.Millisecond), len(line)))
	}
}

// ==== E6: retrato ================================================================================

func TestAdv12EE6Retrato(t *testing.T) {
	// E6.1 mudanças fora dos caminhos de escrita (SQL direto) entre criar e ler o retrato
	{
		muts := []struct{ name, sql string }{
			{"amostra.melhora=1", `UPDATE amostras SET melhora=1`},
			{"categoria trocada", `UPDATE ocorrencias SET categoria='inventou'`},
			{"particao treino", `UPDATE ocorrencias SET particao='treino'`},
			{"prompt_json trocado (hmac igual)", `UPDATE amostras SET prompt_json=CAST('{"model":"m","prompt":"outro"}' AS BLOB)`},
			{"categoria sem critério (outro)", `UPDATE ocorrencias SET categoria='outro'`},
			{"job_do_erro trocado", `UPDATE ocorrencias SET job_do_erro='` + mkID(77) + `'`},
		}
		var bad []string
		for _, m := range muts {
			s, f := openStore(t, Options{}), newFake()
			adv12EEl(t, s, f, 1, "omitiu", nil, true)
			rt, err := s.CreateRetrato(ctx, RetratoOptions{})
			if err != nil {
				t.Fatal(err)
			}
			s.db.Exec(m.sql)
			_, cs, err := s.RetratoCases(ctx, rt.ID)
			g, _ := s.GetRetrato(ctx, rt.ID)
			if !errors.Is(err, ErrRetratoInvalido) || cs != nil || g.Valido {
				bad = append(bad, fmt.Sprintf("%s: err=%v casos=%d valido=%v", m.name, err, len(cs), g.Valido))
			}
		}
		if len(bad) > 0 {
			vec12E(t, "E6.1", "FALHA", "Bloqueador retrato.go:227-252: "+strings.Join(bad, "; "))
		} else {
			vec12E(t, "E6.1", "OK", "RetratoCases reconfere cada caso: melhora, categoria, partição, prompt_json trocado, categoria sem critério e job trocado -> ErrRetratoInvalido, valido=0, nenhum conteúdo entregue")
		}
	}
	// E6.2 o mesmo job em duas ocorrências confirmadas (relato humano repetido): unicidade no retrato e no JSONL
	{
		s, f := openStore(t, Options{}), newFake()
		a := adv12EEl(t, s, f, 1, "omitiu", nil, true)
		r := report(jr(1, "unico", "primario"))
		r.RespostaCorreta = "outra resposta certa, diferente"
		b := adv12EIngest(t, s, f, r)
		must(t, s.Confirm(ctx, b, admin))
		rt, err := s.CreateRetrato(ctx, RetratoOptions{})
		if err != nil {
			t.Fatal(err)
		}
		jobs := map[string]int{}
		for _, it := range rt.Itens {
			jobs[it.JobID]++
		}
		_, lines := adv12EExport(t, s, ParteTeste)
		order := sort.SliceIsSorted(rt.Itens, func(i, j int) bool { return rt.Itens[i].OcorrenciaID < rt.Itens[j].OcorrenciaID })
		same := sideOf(t, s, a) == sideOf(t, s, b) && gidOf(t, s, a) == gidOf(t, s, b)
		if !same || !order {
			vec12E(t, "E6.2", "FALHA", fmt.Sprintf("Bloqueador: mesmo job em grupos/lados diferentes (%v) ou ordem (%v)", same, order))
		} else {
			vec12E(t, "E6.2", "OK", fmt.Sprintf("MEDIDA (só leitura, Menor): o mesmo job relatado 2x pela mesma origem (humano; ingest.go:295-327 só deduplica o automático) gera 2 ocorrências confirmáveis com respostas diferentes: mesmo grupo e lado (sem vazamento), mas o retrato tem %d itens do mesmo job_id e o JSONL %d linhas com o mesmo pedido e respostas diferentes (peso dobrado na avaliação, par contraditório no treino); itens ordenados por ocorrencia_id", jobs[mkID(1)], len(lines)))
		}
	}
	// E6.3 descartar um caso enquanto RetratoCases lê: a janela entre conferir e devolver
	{
		cats := []string{"numeracao_errada", "contagem_errada", "calculo_errado", "inventou", "omitiu", "formato_invalido", "resumo_infiel", "traducao_errada", "truncado", "ignorou_instrucao"}
		s, f := openStore(t, Options{}), newFake()
		var ids []string
		for i := 1; i <= 80; i++ {
			ids = append(ids, adv12EEl(t, s, f, i, cats[i%len(cats)], nil, true))
		}
		windows, refused, rounds := 0, 0, 0
		for round := 0; round < 6; round++ {
			rt, err := s.CreateRetrato(ctx, RetratoOptions{})
			if err != nil {
				break
			}
			last := rt.Itens[len(rt.Itens)-1].OcorrenciaID
			var wg sync.WaitGroup
			wg.Add(1)
			go func(d time.Duration) {
				defer wg.Done()
				time.Sleep(d)
				s.Discard(ctx, last, admin)
			}(time.Duration(round) * 2 * time.Millisecond)
			g0, cs, err := s.RetratoCases(ctx, rt.ID)
			wg.Wait()
			g1, _ := s.GetRetrato(ctx, rt.ID)
			rounds++
			if err == nil && g0.Valido && len(cs) == len(rt.Itens) && !g1.Valido {
				windows++
			} else if errors.Is(err, ErrRetratoInvalido) {
				refused++
			}
		}
		vec12E(t, "E6.3", "OK", fmt.Sprintf("MEDIDA %d rodadas descartando o último caso durante RetratoCases: %d vezes a lista saiu inteira com valido=1 e o retrato ficou inválido logo depois (janela entre conferir e devolver, retrato.go:227-253, sem transação), %d recusas; a janela é coberta pelo recheckRetrato do bakeoff antes do finish (cmd/bakeoff/erros.go:481-485)", rounds, windows, refused))
	}
}
