package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/doctext"
	"github.com/cslsoftwares/mcpollama/files"
)

const docSentinel = "SENTINELA-DO-DOCUMENTO-7d1e"

func tinyDocx(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range [][2]string{
		{"[Content_Types].xml", `<Types/>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="` + doctext.NSWordprocessing +
			`"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`},
	} {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e[1]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDocExtractorIsDocument(t *testing.T) {
	e := &docExtractor{}
	for n, want := range map[string]bool{
		"a.docx": true, "A.DOCX": true, "b.pptx": true, "c.pdf": true, "d.PDF": true,
		"e.doc": true, "f.ppt": true, "g.xls": true,
		"h.txt": false, "i.md": false, "j.docm": false, "k.xlsx": false, "docx": false, "": false, "l.docx.txt": false,
	} {
		if got := e.IsDocument(n); got != want {
			t.Errorf("IsDocument(%q) = %v, want %v", n, got, want)
		}
	}
}

func TestDocExtractorExtractAndRefusals(t *testing.T) {
	e := newDocExtractor(Config{MaxPromptChars: 1000, MaxChunks: 4, Timeout: time.Minute}, nil)
	ctx := context.Background()

	r, err := e.Extract(ctx, "a.docx", tinyDocx(t, "Olá mundo"))
	if err != nil || r.Format != "docx" || !strings.Contains(r.Text, "Olá mundo") || r.PendingRevisions {
		t.Fatalf("docx: %+v %v", r, err)
	}

	ole := append([]byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), make([]byte, 32)...)
	for name, want := range map[string]string{
		"velho.doc": "97-2003", "velho.ppt": "97-2003", "planilha.xls": "Excel 97-2003", "cifrado.docx": "protegido por senha",
	} {
		_, err := e.Extract(ctx, name, ole)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (want %q)", name, err, want)
		}
	}
	if _, err := e.Extract(ctx, "x.pdf", tinyDocx(t, "zip como pdf")); err == nil || !strings.Contains(err.Error(), "não corresponde à extensão") {
		t.Errorf("zip named .pdf: %v", err)
	}
	// a legacy extension holding plain text goes back to the text path
	if _, err := e.Extract(ctx, "notas.doc", []byte("texto simples")); !errors.Is(err, files.ErrNotDocument) {
		t.Errorf("text named .doc: %v", err)
	}
}

// Only doctext's own refusals and context errors reach the user; any other
// error (which could carry document text) becomes the generic refusal.
func TestUserSafe(t *testing.T) {
	leaky := errors.New("falha no parser perto de " + docSentinel)
	got := userSafe(leaky)
	if strings.Contains(got.Error(), docSentinel) {
		t.Fatal("an unknown error leaked through")
	}
	if r, ok := doctext.ReasonOf(got); !ok || r != doctext.ReasonMalformed {
		t.Fatalf("not the generic refusal: %v", got)
	}
	if !errors.Is(userSafe(context.Canceled), context.Canceled) {
		t.Fatal("cancellation must pass through")
	}
	typed := &doctext.Error{Reason: doctext.ReasonNoText}
	if userSafe(typed) != error(typed) {
		t.Fatal("a typed refusal must pass through unchanged")
	}
}

// The text ceiling follows what the file tools can process: MaxPromptChars x
// MaxChunks, never above doctext's own ceiling.
func TestDocExtractorTextCeiling(t *testing.T) {
	if got := newDocExtractor(Config{MaxPromptChars: 128000, MaxChunks: 40}, nil).lim.MaxTextRunes; got != 128000*40 {
		t.Errorf("MaxTextRunes = %d, want %d", got, 128000*40)
	}
	if got := newDocExtractor(Config{MaxPromptChars: 1 << 30, MaxChunks: 1 << 20}, nil).lim.MaxTextRunes; got != doctext.DefaultLimits().MaxTextRunes {
		t.Errorf("MaxTextRunes = %d, want the doctext default", got)
	}
}

// The start records the PDF options and never starts the engine: with a cache
// directory the directory exists afterwards and is still EMPTY (the engine
// writes its compilation cache only when it first runs), and the start is
// instant. Without the flag nothing is created.
func TestStartDoesNotInitialisePDFEngine(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	root := rootWithFiles(t)
	cache := filepath.Join(filepath.Dir(root), "wasm-cache", "sub")
	start := time.Now()
	lines, errw, code := runMain(t, []string{"--ollama-url", f.srv.URL, "--allow-root", root, "--pdf-cache-dir", cache}, nil,
		rpc(1, "initialize", `{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"0"}}`),
		rpc(2, "tools/list", ""))
	elapsed := time.Since(start)
	if code != 0 || len(lines) != 2 {
		t.Fatalf("exit %d, %d lines\n%s", code, len(lines), errw)
	}
	ents, err := os.ReadDir(cache)
	if err != nil {
		t.Fatalf("the cache directory was not created: %v", err)
	}
	if len(ents) != 0 {
		t.Fatalf("the engine started at the start: %d entries in the cache dir", len(ents))
	}
	t.Logf("start + initialize + tools/list in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Fatalf("start took %v", elapsed)
	}

	// without the flag: nothing is created, nothing is written
	other := filepath.Join(filepath.Dir(root), "never")
	if _, _, code := runMain(t, []string{"--ollama-url", f.srv.URL, "--allow-root", root}, nil, rpc(1, "tools/list", "")); code != 0 {
		t.Fatal("start without the flag")
	}
	if _, err := os.Stat(other); err == nil {
		t.Fatal("a directory appeared without --pdf-cache-dir")
	}
}

func TestBadDocFlagsStopTheStart(t *testing.T) {
	root := rootWithFiles(t)
	for name, args := range map[string][]string{
		"doc bytes 0":    {"--allow-root", root, "--max-doc-bytes", "0"},
		"relative cache": {"--allow-root", root, "--pdf-cache-dir", "rel/cache"},
	} {
		lines, errw, code := runMain(t, args, nil, rpc(1, "tools/list", ""))
		if code != 2 || len(lines) != 0 || !strings.Contains(errw, "--") {
			t.Fatalf("%s: exit %d, %d lines, %q", name, code, len(lines), errw)
		}
	}
}

// End to end through the binary's own wiring: a .docx is summarised, the model
// gets the extracted text, the footer says docx, and with --debug neither the
// text, the name nor the path reach stderr: only the metadata line.
func TestDocxEndToEndAndDebugLog(t *testing.T) {
	f := newFakeOllama(t, answerOK)
	root := rootWithFiles(t)
	if err := os.WriteFile(filepath.Join(root, "Parecer Confidencial.docx"), tinyDocx(t, docSentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, errw, code := runMain(t, []string{"--ollama-url", f.srv.URL, "--allow-root", root, "--debug"}, map[string]string{"OLLAMA_API_KEY": wiringToken},
		toolCall(1, "summarize_file", `{"path":"Parecer Confidencial.docx"}`))
	if code != 0 || len(lines) != 1 {
		t.Fatalf("exit %d\n%s", code, errw)
	}
	res, _ := resultOf(t, lines[0])
	if res.IsError || !strings.Contains(res.Content[0].Text, "formato=docx revisoes_pendentes=nao") {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(errw, "doc: format=docx bytes=") || !strings.Contains(errw, "ms=") {
		t.Fatalf("the metadata debug line is missing:\n%s", errw)
	}
	for _, bad := range []string{docSentinel, "Parecer", "Confidencial", root, filepath.Dir(root), wiringToken} {
		if strings.Contains(errw, bad) {
			t.Fatalf("stderr contains %q:\n%s", bad, errw)
		}
	}
}
