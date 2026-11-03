package main

// Fase 12, etapa 12.3: --doc-key-file and the 4 GiB default (contract C-2.8,
// C-4.2, C-4.3, C-4.3a), end to end through run().

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const docKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func writeDocKey(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.key")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// C-4.3: the flag and its environment variable; the flag beats the env; the
// default is "no key"; the path must be absolute; --help documents it.
func TestParseConfigDocKeyFile(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil || c.DocKeyFile != "" {
		t.Fatalf("default: %v %q", err, c.DocKeyFile)
	}
	if err := c.validate(); err != nil {
		t.Fatalf("no key must be valid: %v", err)
	}
	env := map[string]string{"MCPGW_DOC_KEY_FILE": "/etc/mcpollama/doc.key"}
	c, err = parseConfig(nil, func(k string) string { return env[k] }, &buf)
	if err != nil || c.DocKeyFile != "/etc/mcpollama/doc.key" {
		t.Fatalf("env: %v %q", err, c.DocKeyFile)
	}
	c, _ = parseConfig([]string{"--doc-key-file", "/outra/chave"}, func(k string) string { return env[k] }, &buf)
	if c.DocKeyFile != "/outra/chave" {
		t.Fatalf("flag must beat env: %q", c.DocKeyFile)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	c, _ = parseConfig([]string{"--doc-key-file", "relativa/chave"}, noEnv, &buf)
	if err := c.validate(); err == nil {
		t.Fatal("a relative --doc-key-file was accepted")
	}
	var help bytes.Buffer
	parseConfig([]string{"-h"}, noEnv, &help)
	for _, w := range []string{"--doc-key-file", "MCPGW_DOC_KEY_FILE", "32 random bytes", "0600", "4294967296 = 4 GiB", "imagem_hmac"} {
		if !strings.Contains(help.String(), w) {
			t.Errorf("--help lacks %q", w)
		}
	}
}

func runWith(t *testing.T, args ...string) (code int, stderr string) {
	t.Helper()
	var errb bytes.Buffer
	done := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- run(ctx, args, noEnv, strings.NewReader(""), io.Discard, &errb, nil)
	}()
	select {
	case c := <-done:
		return c, errb.String()
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return")
	}
	return -1, ""
}

// C-4.3: a key that is named but unusable stops the start (no silent fallback
// to "no key"), and the message never repeats the key text.
func TestRunRefusesAnUnusableDocKey(t *testing.T) {
	dir := t.TempDir()
	base := []string{"--listen", "127.0.0.1:0", "--ollama", "http://127.0.0.1:1", "--db", filepath.Join(dir, "gw.db"), "--gpu-interval", "0", "--aer-path", "-"}
	cases := map[string]string{
		"short":   writeDocKey(t, strings.Repeat("ab", 16)),
		"not hex": writeDocKey(t, strings.Repeat("zy", 40)),
		"empty":   writeDocKey(t, ""),
		"missing": filepath.Join(dir, "nao-existe"),
	}
	for name, p := range cases {
		code, msg := runWith(t, append(append([]string{}, base...), "--doc-key-file", p)...)
		if code != 2 || !strings.Contains(msg, "--doc-key-file") {
			t.Errorf("%s: exit %d, stderr %q", name, code, msg)
		}
		if strings.Contains(msg, "zyzyzy") || strings.Contains(msg, strings.Repeat("ab", 16)) {
			t.Errorf("%s: the message repeats the key: %q", name, msg)
		}
	}
}

// C-2.5, C-4.2, C-4.3a through run(): with a key the document hash is stored as
// an HMAC with the key id, the image of the stored JSON is an HMAC marker, and
// the plain header value is in no database file. The key id is logged, the key
// never.
func TestRunWithDocKeyStoresHMACsOnly(t *testing.T) {
	var got []byte
	var gotDoc string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/api/generate" {
			got, gotDoc = b, r.Header.Get("X-Mcpollama-Doc")
		}
		io.WriteString(w, `{"response":"ok","done":true,"eval_count":1}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	dbPath, contPath := filepath.Join(dir, "gw.db"), filepath.Join(dir, "content.db")
	keyPath := writeDocKey(t, docKeyHex+"\n")
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", dbPath, "--content-db", contPath,
			"--gpu-interval", "0", "--aer-path", "-", "--doc-key-file", keyPath},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	base := "http://" + <-addr
	doc := strings.Repeat("0123456789abcdef", 4)
	req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"p","images":["QUJDREVGRw=="]}`))
	req.Header.Set("X-Mcpollama-Doc", doc)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	id := resp.Header.Get("X-Request-Id")
	time.Sleep(300 * time.Millisecond)
	cancel()
	if c := <-done; c != 0 {
		t.Fatalf("exit %d: %s", c, errb.String())
	}
	if gotDoc != "" || !bytes.Contains(got, []byte("QUJDREVGRw==")) {
		t.Fatalf("the Ollama got doc=%q body=%s", gotDoc, got)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var h, k string
	if err := db.QueryRow(`SELECT COALESCE(doc_hash,''), COALESCE(doc_key_id,'') FROM requests WHERE id=?`, id).Scan(&h, &k); err != nil {
		t.Fatal(err)
	}
	// expected value computed outside Go: openssl dgst -sha256 -mac HMAC -macopt hexkey:<key>
	if k != "630dcd2966c43366" || h != "ca7eb12f3689793fd47c65dcd0f01941299e641cc5db4f838f5c9f38e75c423b" {
		t.Fatalf("doc_hash=%q key_id=%q", h, k)
	}
	cdb, err := sql.Open("sqlite", contPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	var pj []byte
	var img int
	if err := cdb.QueryRow(`SELECT prompt_json, tem_imagem FROM job_content WHERE job_id=?`, id).Scan(&pj, &img); err != nil {
		t.Fatal(err)
	}
	if img != 1 || !bytes.Contains(pj, []byte(`{"imagem_hmac":"`)) || bytes.Contains(pj, []byte("QUJD")) || bytes.Contains(pj, []byte(doc)) {
		t.Fatalf("tem_imagem=%d prompt_json=%s", img, pj)
	}
	for _, p := range []string{dbPath, dbPath + "-wal", contPath, contPath + "-wal"} {
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(doc)) {
			t.Fatalf("the plain header value is in %s", filepath.Base(p))
		}
	}
	if !strings.Contains(errb.String(), "630dcd2966c43366") {
		t.Errorf("the key id is not in the log: %s", errb.String())
	}
	if strings.Contains(errb.String(), docKeyHex[:16]) {
		t.Error("the key is in the log")
	}
}

// Without --doc-key-file the gateway works as before: no doc_hash, markers
// without a hash, a log line that says so.
func TestRunWithoutDocKeyKeepsNoHash(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"response":"ok","done":true,"eval_count":1}`+"\n")
	}))
	defer up.Close()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gw.db")
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan string, 1)
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--ollama", up.URL, "--db", dbPath, "--gpu-interval", "0", "--aer-path", "-"},
			noEnv, strings.NewReader(""), io.Discard, &errb, func(a string) { addr <- a })
	}()
	base := "http://" + <-addr
	req, _ := http.NewRequest("POST", base+"/api/generate", strings.NewReader(`{"model":"qwen3-ia","prompt":"p"}`))
	req.Header.Set("X-Mcpollama-Doc", strings.Repeat("ab", 32))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	id := resp.Header.Get("X-Request-Id")
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done
	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()
	var h, k string
	if err := db.QueryRow(`SELECT COALESCE(doc_hash,''), COALESCE(doc_key_id,'') FROM requests WHERE id=?`, id).Scan(&h, &k); err != nil {
		t.Fatal(err)
	}
	if h != "" || k != "" {
		t.Fatalf("a hash was kept without a key: %q %q", h, k)
	}
	if !strings.Contains(errb.String(), "sem --doc-key-file") {
		t.Errorf("no log line about the missing key: %s", errb.String())
	}
}
