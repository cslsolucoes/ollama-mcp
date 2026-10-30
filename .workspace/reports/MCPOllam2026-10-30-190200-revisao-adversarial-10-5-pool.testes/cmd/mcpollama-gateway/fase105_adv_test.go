package main

// Fase 10.5 - revisao adversarial do pool de backends (entrypoint).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func vec5c(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

func ollamaFake(gens *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch r.URL.Path {
		case "/api/version":
			fmt.Fprint(w, `{"version":"0.40.8"}`)
		case "/api/ps":
			fmt.Fprint(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":100,"size_vram":100,"digest":"sha256-igual"}]}`)
		case "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen3-ia","model":"qwen3-ia","size":1,"digest":"sha256-igual"}]}`)
		default:
			if gens != nil {
				gens.Add(1)
			}
			io.WriteString(w, `{"response":"ok","done":true,"prompt_eval_count":3,"eval_count":4}`+"\n")
		}
	})
}

// N1 (nao regressao sem --backends): com o Ollama fora, o quadro "Modelos"
// precisa mostrar o erro, como fazia o RunLoop do coletor antes da 10.4. Com o
// pool, a checagem para no /api/version e o quadro fica parado no ultimo
// retrato bom (o modelo segue "carregado" com o Ollama morto).
func TestAdv5N1ModelsBoardSeesTheOllamaDown(t *testing.T) {
	srv := httptest.NewServer(ollamaFake(nil))
	base, errb, stop := startGateway(t, "--ollama", srv.URL, "--health-interval", "40ms")
	defer stop()
	type models struct {
		Models struct {
			Loaded []struct {
				Name string `json:"name"`
			} `json:"loaded"`
			Err string `json:"error"`
		} `json:"models"`
	}
	var st models
	waitS := func(ok func() bool) bool {
		dl := time.Now().Add(3 * time.Second)
		for time.Now().Before(dl) {
			st = models{}
			getJSON(t, base+"/monitor/api/state", &st)
			if ok() {
				return true
			}
			time.Sleep(30 * time.Millisecond)
		}
		return false
	}
	if !waitS(func() bool { return len(st.Models.Loaded) == 1 }) {
		t.Fatalf("o modelo nunca apareceu: %s", errb.String())
	}
	srv.Close()
	if !waitS(func() bool { return st.Models.Err != "" && len(st.Models.Loaded) == 0 }) {
		vec5c(t, "N1-quadro-modelos-parado", "Media", "gateway/pool/pool.go:657", fmt.Sprintf("FALHA: Ollama fora ha 3 s e o quadro Modelos segue %d carregado(s), erro=%q (antes da 10.4 mostrava o erro)", len(st.Models.Loaded), st.Models.Err))
		t.Fail()
		return
	}
	vec5c(t, "N1-quadro-modelos-parado", "Media", "gateway/pool/pool.go:657", "OK: sem --backends, o quadro Modelos mostra o erro com o Ollama fora: "+st.Models.Err)
}

// T1: o token do secundario e o do cliente nao aparecem no log (--debug), no
// /monitor/api/*, nem nos arquivos SQLite (gw.db, -wal, content.db).
func TestAdv5T1TokensNowhere(t *testing.T) {
	const secTok, cliTok = "TOKEN-SEC-T1-9f9f", "TOKEN-CLIENTE-T1-8e8e"
	var pg, sg atomic.Int32
	prim := httptest.NewServer(ollamaFake(&pg))
	defer prim.Close()
	var primDown atomic.Bool
	primH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if primDown.Load() && r.URL.Path == "/api/version" {
			http.Error(w, "down", 500)
			return
		}
		ollamaFake(&pg).ServeHTTP(w, r)
	})
	prim.Config.Handler = primH
	sec := httptest.NewServer(ollamaFake(&sg))
	defer sec.Close()
	dir := t.TempDir()
	tok := filepath.Join(dir, "sec.token")
	os.WriteFile(tok, []byte(secTok+"\n"), 0o600)
	reg := filepath.Join(dir, "backends.json")
	os.WriteFile(reg, []byte(fmt.Sprintf(`{"backends":[{"id":"sec-t1","url":%q,"tipo":"remoto","slots":2,"token_file":%q}]}`, sec.URL, filepath.ToSlash(tok))), 0o600)
	db := filepath.Join(dir, "gw-t1.db")
	cdb := filepath.Join(dir, "content-t1.db")
	base, errb, stop := startGateway(t, "--ollama", prim.URL, "--backends", reg, "--health-interval", "40ms", "--gen-slots", "1", "--debug", "--db", db, "--content-db", cdb, "--sample-store-interval", "100ms")
	post := func() {
		req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"oi"}`))
		req.Header.Set("Authorization", "Bearer "+cliTok)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	time.Sleep(200 * time.Millisecond)
	post()
	primDown.Store(true)
	dl := time.Now().Add(3 * time.Second)
	for sg.Load() == 0 && time.Now().Before(dl) {
		time.Sleep(60 * time.Millisecond)
		post()
	}
	var blobs []string
	for _, p := range []string{"/monitor/api/state", "/monitor/api/history", "/monitor/api/jobs", "/monitor/api/jobs/export", "/monitor/api/jobs/export?formato=csv"} {
		if resp, err := http.Get(base + p); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			blobs = append(blobs, p+":"+string(b))
		}
	}
	time.Sleep(300 * time.Millisecond) // rodadas de backend_samples (--sample-store-interval 100ms)
	stop()
	blobs = append(blobs, "log:"+errb.String())
	for _, f := range []string{db, db + "-wal", cdb, cdb + "-wal"} {
		if b, err := os.ReadFile(f); err == nil {
			blobs = append(blobs, filepath.Base(f)+":"+string(b))
		}
	}
	ids := 0
	for _, b := range blobs {
		if strings.HasPrefix(b, "gw-t1.db") {
			ids += strings.Count(b, "sec-t1")
		}
	}
	var bad []string
	if ids == 0 {
		bad = append(bad, "o gw.db nao tem nenhuma linha do sec-t1 (backend_samples/requests nao gravados: vetor nao exercitado)")
	}
	for _, b := range blobs {
		for _, s := range []string{secTok, cliTok} {
			if strings.Contains(b, s) {
				bad = append(bad, fmt.Sprintf("%s em %.30s", s, b))
			}
		}
	}
	if sg.Load() == 0 {
		bad = append(bad, "nenhum pedido chegou ao secundario (vetor nao exercitado)")
	}
	if len(bad) > 0 {
		vec5c(t, "T1-tokens-em-lugar-nenhum", "Alta", "gateway/pool/registry.go:249", "FALHA: "+strings.Join(bad, " | "))
		t.Fail()
		return
	}
	vec5c(t, "T1-tokens-em-lugar-nenhum", "Alta", "gateway/pool/registry.go:249", fmt.Sprintf("OK: %d fontes (5 rotas do monitor, log --debug, gw.db/-wal, content.db/-wal) sem o token do secundario nem o do cliente; %d geracao(oes) no secundario; %d ocorrencias do id sec-t1 no gw.db (backend_samples e requests gravados)", len(blobs), sg.Load(), ids))
}
