package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// End to end through realMain with --debug: refusals leak nothing to stdout
// or stderr; a hardlink inside the root ships a denylisted file to Ollama.
func TestAttackE2ELeaksAndHardlink(t *testing.T) {
	var hits atomic.Int32
	var bmu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		bmu.Lock()
		bodies = append(bodies, string(b))
		bmu.Unlock()
		answerOK(w, r)
	}))
	t.Cleanup(srv.Close)
	root := rootWithFiles(t) // root/nota.txt, root/.env, ../fora.txt (all carry fileSentinel)
	base := filepath.Dir(root)
	outside := filepath.Join(base, "fora.txt")
	os.Symlink(outside, filepath.Join(root, "link-fora.txt"))
	os.Symlink(filepath.Join(root, ".env"), filepath.Join(root, "link-env.txt"))
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, "jfora"), base).Run()
	}
	hlErr := os.Link(filepath.Join(root, ".env"), filepath.Join(root, "hl-env.txt"))

	reqs := []string{
		"link-fora.txt", "link-env.txt", `jfora\fora.txt`, `..\fora.txt`, outside, ".env", ".ENV.", `\\?\` + outside,
		`\\localhost\c$\Windows\win.ini`, "CON", "nota.txt:x", strings.Repeat("a", 5000),
	}
	var lines []string
	for i, r := range reqs {
		lines = append(lines, toolCall(i+1, "summarize_file", mustJSONString(map[string]string{"path": r})))
	}
	args := []string{"--ollama-url", srv.URL, "--allow-root", root, "--debug"}
	out, errw, code := runMain(t, args, map[string]string{"OLLAMA_API_KEY": wiringToken}, lines...)
	if code != 0 || len(out) != len(reqs) {
		t.Fatalf("exit %d, %d lines", code, len(out))
	}
	m := byID(t, out)
	for i := range reqs {
		res, r := resultOf(t, m[fmt.Sprint(i+1)])
		if r.Error != nil || !res.IsError {
			t.Errorf("VEC|e2e/%d|ESCAPOU|nao recusado: %+v", i+1, res)
			continue
		}
		msg := res.Content[0].Text
		for _, bad := range []string{fileSentinel, base, wiringToken} {
			if strings.Contains(msg, bad) {
				t.Errorf("VEC|e2e/%d|ESCAPOU|mensagem contem %q", i+1, bad)
			}
		}
	}
	for _, bad := range []string{fileSentinel, base, wiringToken, "fora.txt", ".env"} {
		if strings.Contains(errw, bad) {
			t.Errorf("VEC|e2e/stderr|ESCAPOU|stderr contem %q", bad)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("VEC|e2e/no-ollama|ESCAPOU|%d requisicoes ao Ollama", hits.Load())
	}
	t.Logf("VEC|e2e/refusals|BLOQUEADO|%d recusas, stdout/stderr sem conteudo, caminho ou token; 0 chamadas ao Ollama", len(reqs))

	if hlErr != nil {
		t.Logf("VEC|e2e/hardlink|NAO-TESTAVEL|%v", hlErr)
		return
	}
	out, _, _ = runMain(t, args, map[string]string{"OLLAMA_API_KEY": wiringToken},
		toolCall(1, "summarize_file", `{"path":"hl-env.txt"}`))
	res, _ := resultOf(t, out[0])
	sent := false
	bmu.Lock()
	for _, b := range bodies {
		if strings.Contains(b, fileSentinel) {
			sent = true
		}
	}
	bmu.Unlock()
	if !res.IsError && sent {
		t.Errorf("VEC|e2e/hardlink-.env|ESCAPOU|o conteudo do .env (via hardlink hl-env.txt) foi enviado ao Ollama")
	} else {
		t.Logf("VEC|e2e/hardlink-.env|BLOQUEADO|isError=%v sent=%v", res.IsError, sent)
	}
}
