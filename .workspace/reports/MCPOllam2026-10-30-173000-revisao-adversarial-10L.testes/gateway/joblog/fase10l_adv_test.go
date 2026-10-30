package joblog_test

// Fase 10.L - revisao adversarial do journal (proxy -> store + content).
// Cada vetor imprime VEC|id|severidade|arquivo:linha|resultado e FALHA quando o
// defeito existe.

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/joblog"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

func vecL(t *testing.T, id, sev, where, res string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s|%s", id, sev, where, res)
}

const sentP, sentR = "SENTINELA-PROMPT-10L-k9", "SENTINELA-RESPOSTA-10L-k9"

// TestAdvLKillChild e o processo filho do K1: grava um job ate a metade e espera
// ser morto.
func TestAdvLKillChild(t *testing.T) {
	dir := os.Getenv("ADV10L_CHILD_DIR")
	if dir == "" {
		t.Skip("so roda como filho do K1")
	}
	st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: os.Getenv("ADV10L_CHILD_BOOT")})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := content.Open(filepath.Join(dir, "content.db"), content.Options{FlushEvery: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	jr := &joblog.Journal{Store: st, Content: cs}
	id := os.Getenv("ADV10L_CHILD_ID")
	now := time.Now()
	jr.Enqueued(queue.Record{ID: id, ClientIP: "10.0.0.2", ClientLabel: "filho", Endpoint: "/api/generate", Model: "qwen3-ia", Lane: queue.LaneGen,
		PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: now, Status: queue.StatusQueued}, "[prompt]\n"+sentP)
	jr.Started(id, now.Add(time.Millisecond), "gpu-local")
	k := jr.Response(id)
	k.Write(sentR + "-parte1 ")
	time.Sleep(30 * time.Millisecond)
	k.Write("parte2 ") // passa do FlushEvery: grava parte1+parte2
	fmt.Println("PRONTO")
	os.Stdout.Sync()
	select {} // espera o SIGKILL / TerminateProcess
}

func killChild(t *testing.T, dir, boot, id string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdvLKillChild$", "-test.v")
	cmd.Env = append(os.Environ(), "ADV10L_CHILD_DIR="+dir, "ADV10L_CHILD_BOOT="+boot, "ADV10L_CHILD_ID="+id)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(out)
	ok := false
	deadline := time.AfterFunc(20*time.Second, func() { cmd.Process.Kill() })
	for sc.Scan() {
		if strings.Contains(sc.Text(), "PRONTO") {
			ok = true
			break
		}
	}
	deadline.Stop()
	cmd.Process.Kill() // TerminateProcess no Windows, SIGKILL no Linux
	cmd.Wait()
	if !ok {
		t.Fatal("o filho nao chegou ao ponto de morte")
	}
}

// K1: processo morto no meio do stream. Mesma boot -> interrompido_gateway;
// boot trocada -> interrompido_reboot; prompt e resposta parcial preservados.
func TestAdvLKilledMidStream(t *testing.T) {
	var bad, got []string
	for _, c := range []struct{ childBoot, nextBoot, want string }{
		{"boot-A", "boot-A", queue.StatusInterruptedRestart},
		{"boot-A", "boot-B", queue.StatusInterruptedReboot},
	} {
		dir := t.TempDir()
		id := queue.NewID()
		killChild(t, dir, c.childBoot, id)
		st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: c.nextBoot})
		if err != nil {
			t.Fatal(err)
		}
		cs, err := content.Open(filepath.Join(dir, "content.db"), content.Options{})
		if err != nil {
			t.Fatal(err)
		}
		jr := &joblog.Journal{Store: st, Content: cs}
		jr.RecoverResponses(context.Background())
		j, ok, _ := st.Job(context.Background(), id)
		cc, okc, _ := cs.Get(context.Background(), id)
		evs, _ := st.Events(context.Background(), 50)
		hasEv := false
		for _, e := range evs {
			if e.Kind == "jobs_interrompidos" {
				hasEv = true
			}
		}
		got = append(got, fmt.Sprintf("%s->%s: %s responded=%v resp_bytes=%d prompt=%v parcial=%q final=%v evento=%v", c.childBoot, c.nextBoot, j.Status, j.Responded, j.RespBytes,
			okc && strings.Contains(cc.Prompt, sentP), cc.Response, cc.RespFinal, hasEv))
		if !ok || j.Status != c.want || !okc || !strings.Contains(cc.Prompt, sentP) || !strings.Contains(cc.Response, sentR+"-parte1") || cc.RespFinal || !j.Responded || !hasEv || j.StartedAt == 0 {
			bad = append(bad, c.want)
		}
		cs.Close()
		st.Close()
	}
	if len(bad) > 0 {
		vecL(t, "K1-sigkill-no-stream", "Alta", "gateway/store/jobs.go:94", "FALHA: "+strings.Join(got, " | "))
		t.Error(got)
	} else {
		vecL(t, "K1-sigkill-no-stream", "Alta", "gateway/store/jobs.go:94", "OK (processo filho morto por "+map[bool]string{true: "TerminateProcess", false: "SIGKILL"}[runtime.GOOS == "windows"]+"): "+strings.Join(got, " | "))
	}
}

// slowJournal atrasa o Finish para provar que o gravador nao trava a fila.
type slowJournal struct {
	*joblog.Journal
	d time.Duration
}

func (s *slowJournal) Finish(r queue.Record) { time.Sleep(s.d); s.Journal.Finish(r) }

func rigWith(t *testing.T, up http.HandlerFunc, withJournal bool, slowFinish time.Duration, logs *syncBuf) (*httptest.Server, *store.Store, *queue.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "gw.db"), store.Options{BootID: "b", Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := content.Open(filepath.Join(dir, "content.db"), content.Options{Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	jr := &joblog.Journal{Store: st, Content: cs, Logf: logs.logf}
	var onRec func(queue.Record) = st.Record
	if withJournal {
		onRec = jr.Finish
		if slowFinish > 0 {
			sj := &slowJournal{jr, slowFinish}
			onRec = sj.Finish
		}
	}
	q := queue.New(queue.Config{OnRecord: onRec, GenSlots: 1, QueueMax: 64, PerClient: 64, PerIP: 64})
	jr.Running = q.RunningIDs
	upsrv := httptest.NewServer(up)
	uu, _ := url.Parse(upsrv.URL)
	cfg := proxy.Config{Upstream: uu, Queue: q, Logf: logs.logf}
	if withJournal {
		cfg.Journal = jr
	}
	h, err := proxy.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	gw := httptest.NewServer(h)
	t.Cleanup(func() { gw.Close(); upsrv.Close(); cs.Close(); st.Close() })
	return gw, st, q, dir
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) logf(f string, a ...any) {
	s.mu.Lock()
	fmt.Fprintf(&s.b, f+"\n", a...)
	s.mu.Unlock()
}
func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func quick(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	io.WriteString(w, `{"response":"ok","done":false}`+"\n"+`{"response":"","done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
}

func postN(t *testing.T, base string, n, par int, body string) []time.Duration {
	t.Helper()
	var mu sync.Mutex
	var lat []time.Duration
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(body))
			req.Header.Set(proxy.HeaderClient, fmt.Sprintf("rajada/%d", i%4))
			st := time.Now()
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			mu.Lock()
			lat = append(lat, time.Since(st))
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	return lat
}

func pct(l []time.Duration, p int) time.Duration {
	return l[(len(l)-1)*p/100].Round(100 * time.Microsecond)
}

// F1: custo do fsync sincrono (2-3 commits FULL por pedido). Rajada de 600
// pedidos de um cliente, 10 em paralelo, com e sem o journal; e um Finish
// lento (500 ms) nao pode travar a fila.
func TestAdvLFsyncBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	body := `{"model":"qwen3-ia","prompt":"` + strings.Repeat("x", 4096) + `"}`
	var lb syncBuf
	gw0, _, _, _ := rigWith(t, quick, false, 0, &lb)
	gw1, st1, _, _ := rigWith(t, quick, true, 0, &lb)
	postN(t, gw0.URL, 20, 10, body) // aquece
	postN(t, gw1.URL, 20, 10, body)
	s0 := time.Now()
	l0 := postN(t, gw0.URL, 600, 10, body)
	d0 := time.Since(s0)
	s1 := time.Now()
	l1 := postN(t, gw1.URL, 600, 10, body)
	d1 := time.Since(s1)
	js, _ := st1.Jobs(context.Background(), store.JobQuery{Limit: 1000})
	res := fmt.Sprintf("600 pedidos/10 paralelos: sem journal %s (p50 %s p95 %s); com journal %s (p50 %s p95 %s); +%.2f ms/pedido; %d jobs gravados",
		d0.Round(time.Millisecond), pct(l0, 50), pct(l0, 95), d1.Round(time.Millisecond), pct(l1, 50), pct(l1, 95), float64(d1-d0)/600/1e6, len(js))

	// Finish de 500 ms: 6 pedidos com 1 slot nao podem somar 6 x 500 ms
	gw2, _, q2, _ := rigWith(t, quick, true, 500*time.Millisecond, &lb)
	s2 := time.Now()
	postN(t, gw2.URL, 6, 6, body)
	d2 := time.Since(s2)
	// o snapshot da fila responde enquanto um Finish dorme
	go postN(t, gw2.URL, 1, 1, body)
	time.Sleep(100 * time.Millisecond)
	s3 := time.Now()
	q2.Snapshot()
	d3 := time.Since(s3)
	res += fmt.Sprintf("; Finish de 500 ms: 6 pedidos em %s, Snapshot em %s", d2.Round(time.Millisecond), d3.Round(time.Microsecond))
	if len(js) < 620 || d2 > 1500*time.Millisecond || d3 > 50*time.Millisecond {
		vecL(t, "F1-fsync-rajada", "Media", "gateway/joblog/joblog.go:37", "FALHA: "+res)
		t.Error(res)
	} else {
		vecL(t, "F1-fsync-rajada", "Media", "gateway/joblog/joblog.go:37", "OK/MEDIDA: "+res)
	}
}

// E1: erros e logs nao ecoam o prompt; o gw.db (e o -wal) nao tem o texto.
func TestAdvLNoEchoNoMetadataText(t *testing.T) {
	var lb syncBuf
	gw, st, _, dir := rigWith(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"`+sentR+`","done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
	}, true, 0, &lb)
	var bad []string
	cases := []struct{ path, body string }{
		{"/api/generate", `{"model":"qwen3-ia","prompt":"` + sentP + `"}`},                         // normal
		{"/api/generate", `{"model":"qwen3-ia","prompt":"` + sentP},                                // JSON quebrado
		{"/api/generate", `{"model":"modelo-proibido","prompt":"` + sentP + `"}`},                  // modelo fora da lista
		{"/api/chat", `{"model":"qwen3-ia","messages":"` + sentP + `"}`},                           // messages nao-lista
		{"/api/generate", `{"model":"qwen3-ia","prompt":"` + sentP + `","keep_alive":"-1"}`},       // opcao barrada
		{"/api/generate", `{"model":"qwen3-ia","prompt":"` + sentP + `","options":{"num_ctx":1}}`}, // num_ctx
		{"/api/pull", `{"name":"` + sentP + `"}`},                                                  // rota bloqueada
	}
	for _, c := range cases {
		resp, err := http.Post(gw.URL+c.path, "application/json", strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 && strings.Contains(string(b), sentP) {
			bad = append(bad, fmt.Sprintf("%s %d ecoa o prompt", c.path, resp.StatusCode))
		}
	}
	time.Sleep(300 * time.Millisecond)
	st.Flush()
	if strings.Contains(lb.String(), sentP) || strings.Contains(lb.String(), sentR) {
		bad = append(bad, "log tem o texto")
	}
	for _, f := range []string{"gw.db", "gw.db-wal"} {
		b, _ := os.ReadFile(filepath.Join(dir, f))
		if bytes.Contains(b, []byte(sentP)) || bytes.Contains(b, []byte(sentR)) {
			bad = append(bad, f+" tem o texto")
		}
	}
	cb, _ := os.ReadFile(filepath.Join(dir, "content.db"))
	cw, _ := os.ReadFile(filepath.Join(dir, "content.db-wal"))
	inContent := bytes.Contains(append(cb, cw...), []byte(sentP))
	if len(bad) > 0 || !inContent {
		vecL(t, "E1-eco-log-sentinela", "Alta", "gateway/proxy/journal.go:50", fmt.Sprintf("FALHA: %v (texto no content.db: %v)", bad, inContent))
		t.Error(bad)
	} else {
		vecL(t, "E1-eco-log-sentinela", "Alta", "gateway/proxy/journal.go:50", "OK: 7 pedidos (JSON quebrado, modelo proibido, rota bloqueada...) sem eco; log, gw.db e -wal sem o texto; o texto esta so no content.db")
	}
}

// M1: memoria de um prompt de 32 MiB (o teto do corpo) pelo caminho do
// journal, comparado sem journal.
func TestAdvLMemoryBigPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("lento")
	}
	body := `{"model":"qwen3-ia","prompt":"` + strings.Repeat("y", 32<<20-64) + `"}`
	meas := func(withJ bool) uint64 {
		var lb syncBuf
		gw, _, _, _ := rigWith(t, quick, withJ, 0, &lb)
		postN(t, gw.URL, 1, 1, `{"model":"qwen3-ia","prompt":"a"}`)
		runtime.GC()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		postN(t, gw.URL, 1, 1, body)
		runtime.ReadMemStats(&m1)
		return m1.TotalAlloc - m0.TotalAlloc
	}
	a0, a1 := meas(false), meas(true)
	res := fmt.Sprintf("alocado por 1 pedido de 32 MiB: sem journal %.0f MiB, com journal %.0f MiB (+%.0f MiB)", float64(a0)/(1<<20), float64(a1)/(1<<20), float64(a1-a0)/(1<<20))
	vecL(t, "M1-memoria-prompt-grande", "Baixa", "gateway/proxy/proxy.go:364", "MEDIDA: "+res)
}
