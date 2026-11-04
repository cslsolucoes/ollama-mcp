package main

// Revisão adversarial da etapa 12.9 (Fase 12), revisor F: POST /api/feedback com o gateway
// inteiro em processo (proxy real gravando o job, job log e content.db reais). Depende dos
// helpers do pacote: startGW, gw.jobDone, writeDocKey, docKeyHex (fase12_feedback_main_test.go,
// fase12_main_test.go).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func vec12F(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res != "OK" {
		t.Errorf("vetor %s: %s", id, desc)
	}
}

// adv12FCall sends a request through the gateway as if the local nginx relayed it (the test
// client connects from 127.0.0.1, so X-Real-Ip is believed, as in production).
func adv12FCall(t *testing.T, g *gw, path, body, realIP, station string, extra map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", g.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if realIP != "" {
		req.Header.Set("X-Real-Ip", realIP)
	}
	if station != "" {
		req.Header.Set("X-Mcpollama-Station", station)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b
}

func adv12FJobVia(t *testing.T, g *gw, path, body, realIP, station string) string {
	t.Helper()
	_, h, _ := adv12FCall(t, g, path, body, realIP, station, map[string]string{"X-Mcpollama-Tool": "extract"})
	id := h.Get("X-Request-Id")
	if len(id) != 32 {
		t.Fatalf("no job id for %s", path)
	}
	return id
}

func adv12FWait(t *testing.T, g *gw, id string, needContent bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if needContent && g.jobDone(id) {
			return
		}
		if !needContent {
			db, err := sql.Open("sqlite", filepath.Join(g.dir, "gw.db"))
			if err == nil {
				var st string
				e := db.QueryRow(`SELECT status FROM requests WHERE id=?`, id).Scan(&st)
				db.Close()
				if e == nil && st != "na_fila" && st != "em_execucao" {
					return
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
}

func adv12FFb(id string) string {
	b, _ := json.Marshal(map[string]any{"projeto": "P", "categoria": "omitiu", "descricao": "faltou", "severidade": "baixa", "origem": "claude",
		"resposta_correta": "1, 2, 3", "jobs": []map[string]string{{"id": id, "papel": "unico", "backend": "primario"}}})
	return string(b)
}

func TestAdv12FEndToEndOrigin(t *testing.T) {
	g := startGW(t, "--doc-key-file", writeDocKey(t, docKeyHex), "--treino-db", filepath.Join(t.TempDir(), "treino.db"))
	gen := `{"model":"qwen3-ia","prompt":"conte os itens"}`
	const stA = "computer=PC01;user=ana"
	jA := adv12FJobVia(t, g, "/api/generate", gen, "10.0.0.1", stA)
	j6 := adv12FJobVia(t, g, "/api/generate", gen, "2001:db8::1", "") // no station, IPv6
	jEmb := adv12FJobVia(t, g, "/api/embed", `{"model":"nomic-embed-text","input":"x"}`, "10.0.0.1", stA)
	adv12FWait(t, g, jA, true)
	adv12FWait(t, g, j6, true)
	adv12FWait(t, g, jEmb, false)

	type res struct {
		Estado           string `json:"estado"`
		AmostrasCopiadas int    `json:"amostras_copiadas"`
		JobsOutraOrigem  int    `json:"jobs_outra_origem"`
		OcorrenciaID     string `json:"ocorrencia_id"`
	}
	fb := func(id, ip, station string, extra map[string]string) (int, res, http.Header) {
		code, h, b := adv12FCall(t, g, "/api/feedback", adv12FFb(id), ip, station, extra)
		var r res
		json.Unmarshal(b, &r)
		return code, r, h
	}
	for _, c := range []struct {
		id, desc, ip, station string
		extra                 map[string]string
	}{
		{"F1e2e-a", "estação A declarada, X-Real-Ip 10.0.0.2", "10.0.0.2", stA, nil},
		{"F1e2e-b", "X-Forwarded-For 10.0.0.1 com X-Real-Ip 10.0.0.2", "10.0.0.2", stA, map[string]string{"X-Forwarded-For": "10.0.0.1"}},
		{"F1e2e-c", "estação vazia, mesmo IP 10.0.0.1", "10.0.0.1", "", nil},
		{"F1e2e-d", "estação com outro usuário, mesmo IP", "10.0.0.1", "computer=PC01;user=bob", nil},
	} {
		code, r, _ := fb(jA, c.ip, c.station, c.extra)
		if code == 200 && r.Estado == "sem_pedido" && r.AmostrasCopiadas == 0 && r.JobsOutraOrigem == 1 {
			vec12F(t, c.id, "OK", "gateway inteiro: "+c.desc+": sem_pedido, 0 amostras")
		} else {
			vec12F(t, c.id, "FALHA", fmt.Sprintf("Bloqueador treino/ingest.go:36: gateway inteiro: %s: %d %+v", c.desc, code, r))
		}
	}
	// IPv6: the job log keeps the /64; a neighbour in the same /64 is "the same IP" (same calculation)
	code1, r1, _ := fb(j6, "2001:db8:0:1::1", "", nil)
	code2, r2, _ := fb(j6, "2001:db8::2", "", nil)
	if code1 == 200 && r1.Estado == "sem_pedido" && code2 == 200 && r2.Estado == "nova" && r2.AmostrasCopiadas == 1 {
		vec12F(t, "F1e2e-v6", "OK", "MEDIDA: IPv6: outro /64 não copia; um vizinho do MESMO /64 (2001:db8::2 para o job de 2001:db8::1) copia, porque o job log guarda o /64 (proxy.fairIP) e a C-1.11a manda usar o mesmo cálculo")
	} else {
		vec12F(t, "F1e2e-v6", "FALHA", fmt.Sprintf("Menor: IPv6 outro /64 %d %+v, mesmo /64 %d %+v", code1, r1, code2, r2))
	}
	// control: the true origin copies
	code, r, h := fb(jA, "10.0.0.1", stA, nil)
	if code == 200 && r.Estado == "nova" && r.AmostrasCopiadas == 1 {
		vec12F(t, "F1e2e-ctl", "OK", "controle no gateway inteiro: mesma estação e mesmo IP copiam")
	} else {
		vec12F(t, "F1e2e-ctl", "FALHA", fmt.Sprintf("Menor fixture: origem verdadeira %d %+v", code, r))
	}
	// F8: the feedback call itself is not a job; its ocorrencia_id used as a job is unknown
	db, _ := sql.Open("sqlite", filepath.Join(g.dir, "gw.db"))
	var nfb int
	db.QueryRow(`SELECT COUNT(*) FROM requests WHERE endpoint LIKE '%feedback%'`).Scan(&nfb)
	db.Close()
	codeSelf, _, _ := fb(r.OcorrenciaID, "10.0.0.1", stA, nil)
	if h.Get("X-Request-Id") == "" && nfb == 0 && codeSelf == 404 {
		vec12F(t, "F8e2e-a", "OK", "o próprio /api/feedback não vira job (sem X-Request-Id, nenhuma linha em requests); usar o ocorrencia_id como job_do_erro dá 404")
	} else {
		vec12F(t, "F8e2e-a", "FALHA", fmt.Sprintf("Importante: feedback como job: X-Request-Id=%q linhas=%d status=%d", h.Get("X-Request-Id"), nfb, codeSelf))
	}
	// F8: an embedding job can be reported (statistics); export excludes it (C-3.5, tested in the package)
	codeE, rE, _ := fb(jEmb, "10.0.0.1", stA, nil)
	if codeE == 200 && (rE.Estado == "nova" || rE.Estado == "sem_pedido") {
		vec12F(t, "F8e2e-b", "OK", fmt.Sprintf("MEDIDA: relato de job de embed aceito no gateway inteiro (estado %s, %d amostra); a exportação o exclui pela C-3.5 (vetor F8b do pacote treino)", rE.Estado, rE.AmostrasCopiadas))
	} else {
		vec12F(t, "F8e2e-b", "FALHA", fmt.Sprintf("Menor: relato de embed: %d %+v", codeE, rE))
	}
}
