package files

// Revisao adversarial da Fase 7: o guard com um extrator falso hostil.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func advVec(t *testing.T, area, name, res, detail string) {
	t.Helper()
	line := fmt.Sprintf("VEC|%s/%s|%s|%s", area, name, res, detail)
	if res == "ESCAPOU" || res == "SILENCIOSO" {
		t.Error(line)
	} else {
		t.Log(line)
	}
}

type advExtractor struct {
	fn    func(ctx context.Context, name string, data []byte) (*Extracted, error)
	calls int
}

func (a *advExtractor) IsDocument(name string) bool {
	return (&fakeExtractor{}).IsDocument(name)
}

func (a *advExtractor) Extract(ctx context.Context, name string, data []byte) (*Extracted, error) {
	a.calls++
	return a.fn(ctx, name, data)
}

func TestAdvGuardHostileExtractor(t *testing.T) {
	w := newWorld(t)
	os.WriteFile(filepath.Join(w.root, "a.pdf"), []byte("%PDF-1.4 conteudo"), 0o644)

	// 1. ctx cancelado com o extrator bloqueado: a leitura volta logo.
	{
		ex := &advExtractor{fn: func(ctx context.Context, _ string, _ []byte) (*Extracted, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		g := docGuard(t, w, ex, 1<<20, 1<<20)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := g.ReadContext(ctx, "a.pdf")
		el := time.Since(start)
		advVec(t, "guard", "ctx-extrator-bloqueado", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[errors.Is(err, context.DeadlineExceeded) && el < 2*time.Second],
			fmt.Sprintf("%v em %v", err, el.Round(time.Millisecond)))
	}
	// 2. extrator devolve UTF-8 invalido / nil,nil.
	for name, fn := range map[string]func(context.Context, string, []byte) (*Extracted, error){
		"utf8-invalido": func(context.Context, string, []byte) (*Extracted, error) { return &Extracted{Text: "ok\xff\xfe", Format: "pdf"}, nil },
		"nil-nil":       func(context.Context, string, []byte) (*Extracted, error) { return nil, nil },
	} {
		g := docGuard(t, w, &advExtractor{fn: fn}, 1<<20, 1<<20)
		_, err := g.Read("a.pdf")
		advVec(t, "guard", "extrator-"+name, map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[err != nil], fmt.Sprint(err))
	}
	// 3. mensagem do extrator e mostrada como esta (contrato): o filtro fica no adaptador.
	{
		g := docGuard(t, w, &advExtractor{fn: func(context.Context, string, []byte) (*Extracted, error) {
			return nil, errors.New("falha perto de " + w.root)
		}}, 1<<20, 1<<20)
		_, err := g.Read("a.pdf")
		advVec(t, "guard", "mensagem-do-extrator-repassada", "INFO", fmt.Sprintf("caminho_repassado=%v (o guard confia no contrato; userSafe do binario filtra: ver leak/*)", err != nil && strings.Contains(err.Error(), w.root)))
	}
	// 4. denylist antes de ler, com extensoes de documento.
	for _, rel := range []string{"credentials.docx", "secrets/contrato.pdf", "minha-secret.pptx", ".env.pdf"} {
		p := filepath.Join(w.root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("PK\x03\x04"), 0o644)
		ex := &advExtractor{fn: func(context.Context, string, []byte) (*Extracted, error) { return &Extracted{Text: "x", Format: "docx"}, nil }}
		g := docGuard(t, w, ex, 1<<20, 1<<20)
		_, err := g.Read(rel)
		res := "BLOQUEADO"
		if err == nil || ex.calls > 0 {
			res = "INFO"
			if rel != "x.pem.docx" {
				res = "ESCAPOU"
			}
		}
		advVec(t, "guard", "denylist-"+rel, res, fmt.Sprintf("err=%v chamadas_ao_extrator=%d", err, ex.calls))
	}
	// 5. extensao de documento so no nome intermediario: "relatorio.pdf.txt" vai pelo texto.
	{
		os.WriteFile(filepath.Join(w.root, "relatorio.pdf.txt"), []byte("texto"), 0o644)
		ex := &advExtractor{fn: func(context.Context, string, []byte) (*Extracted, error) { return &Extracted{Text: "x", Format: "pdf"}, nil }}
		g := docGuard(t, w, ex, 1<<20, 1<<30)
		f, err := g.Read("relatorio.pdf.txt")
		advVec(t, "guard", "pdf-txt-vai-pelo-texto", map[bool]string{true: "BLOQUEADO", false: "ESCAPOU"}[err == nil && f.Format == "txt" && ex.calls == 0], fmt.Sprintf("formato=%v chamadas=%d", f, ex.calls))
	}
}
