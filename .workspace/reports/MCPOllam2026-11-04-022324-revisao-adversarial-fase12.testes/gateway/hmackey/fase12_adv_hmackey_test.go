package hmackey

// Revisão adversarial da etapa 12.9 (Fase 12), revisor P: a chave HMAC (C-4.3, C-4.3a).
// Cada vetor imprime uma linha VEC. Sem dependência de outros _test.go do pacote.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func vec12P(t *testing.T, id, res, desc string) {
	t.Helper()
	t.Logf("VEC|%s|%s|%s", id, res, desc)
	if res == "FALHA" {
		t.Errorf("%s: %s", id, desc)
	}
}

const adv12PHex = "c0ffee00112233445566778899aabbccddeeff00112233445566778899aabbcc"

func adv12PWrite(t *testing.T, content []byte, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.key")
	if err := os.WriteFile(p, content, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// adv12PLeaks says whether s shows the key: its hex (any 8-char window), or its raw bytes.
func adv12PLeaks(s string, raw []byte) bool {
	low := strings.ToLower(s)
	for i := 0; i+8 <= len(adv12PHex); i += 8 {
		if strings.Contains(low, adv12PHex[i:i+8]) {
			return true
		}
	}
	if bytes.Contains([]byte(s), raw[:8]) {
		return true
	}
	// fmt of a []byte with %v prints the decimal bytes
	dec := strings.Trim(fmt.Sprint(raw[:6]), "[]")
	return strings.Contains(s, dec)
}

func TestAdv12PKeyFile(t *testing.T) {
	raw, _ := hex.DecodeString(adv12PHex)
	want, _ := New(raw)

	// P5.perm: the Unix rule exercised on any host; on Windows the rule does not apply.
	bad := 0
	for _, m := range []os.FileMode{0o640, 0o604, 0o660, 0o644, 0o700 | 0o001, 0o666} {
		if checkMode("linux", m) == nil {
			bad++
		}
	}
	if bad == 0 && checkMode("linux", 0o600) == nil && checkMode("linux", 0o400) == nil {
		note := "no Linux recusa qualquer bit de grupo/outros e aceita 0600/0400"
		if runtime.GOOS == "windows" {
			note += "; NESTA MÁQUINA (Windows) o teste de 0600 não vale: checkMode ignora o modo (hmackey.go:96) e o Load aceita o arquivo com qualquer ACL"
		}
		vec12P(t, "P5.perm", "OK", note)
	} else {
		vec12P(t, "P5.perm", "FALHA", fmt.Sprintf("Importante gateway/hmackey/hmackey.go:96 checkMode aceitou %d modos abertos", bad))
	}

	type lc struct {
		id      string
		content []byte
		ok      bool
	}
	cases := []lc{
		{"P5.curta63", []byte(adv12PHex[:63]), false},
		{"P5.curta62", []byte(adv12PHex[:62]), false},
		{"P5.vazia", []byte(""), false},
		{"P5.espacos", []byte("  \t" + adv12PHex + "  \r\n\r\n"), true},
		{"P5.crlf", []byte(adv12PHex + "\r\n"), true},
		{"P5.maiusculas", []byte(strings.ToUpper(adv12PHex)), true},
		{"P5.espacoNoMeio", []byte(adv12PHex[:32] + " " + adv12PHex[32:]), false},
		{"P5.bom", append([]byte{0xEF, 0xBB, 0xBF}, adv12PHex...), false},
		{"P5.utf16", []byte("\xff\xfe" + strings.Join(strings.Split(adv12PHex, ""), "\x00") + "\x00"), false},
		{"P5.prefixo0x", []byte("0x" + adv12PHex), false},
		{"P5.maiorQueLimit", []byte(strings.Repeat("ab", 2049)), false},
		{"P5.limiteExato", []byte(strings.Repeat("ab", 2048)), true},
		{"P5.naoHexComChave", []byte(adv12PHex + "zz"), false},
	}
	for _, c := range cases {
		p := adv12PWrite(t, c.content, 0o600)
		k, err := Load(p)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		switch {
		case err != nil && adv12PLeaks(msg, raw):
			vec12P(t, c.id, "FALHA", "Bloqueador gateway/hmackey/hmackey.go:51 a mensagem de erro do Load mostra a chave: "+msg)
		case c.ok && err != nil:
			vec12P(t, c.id, "FALHA", "Menor gateway/hmackey/hmackey.go:78 arquivo válido recusado: "+msg)
		case !c.ok && err == nil:
			vec12P(t, c.id, "FALHA", fmt.Sprintf("Importante gateway/hmackey/hmackey.go:78 arquivo inválido aceito (key_id %s)", k.ID()))
		case c.ok && c.id != "P5.limiteExato" && k.ID() != want.ID():
			vec12P(t, c.id, "FALHA", "Importante gateway/hmackey/hmackey.go:78 chave lida diferente da escrita")
		case c.ok:
			vec12P(t, c.id, "OK", "aceito com o key_id certo ("+k.ID()+")")
		default:
			vec12P(t, c.id, "OK", "recusado sem mostrar a chave: "+msg)
		}
	}

	// P5.diretorio / P5.symlink
	if _, err := Load(t.TempDir()); err != nil && !adv12PLeaks(err.Error(), raw) {
		vec12P(t, "P5.diretorio", "OK", "diretório recusado: "+err.Error())
	} else {
		vec12P(t, "P5.diretorio", "FALHA", fmt.Sprintf("Menor gateway/hmackey/hmackey.go:61 diretório aceito: %v", err))
	}
	target := adv12PWrite(t, []byte(adv12PHex), 0o600)
	link := filepath.Join(t.TempDir(), "link.key")
	if err := os.Symlink(target, link); err != nil {
		vec12P(t, "P5.symlink", "OK", "não exercitado nesta máquina (sem privilégio de symlink no Windows: "+err.Error()+"); por leitura, hmackey.go:52-61 segue o link e valida o ALVO pelo handle (f.Stat), sem TOCTOU")
	} else if k, err := Load(link); err == nil && k.ID() == want.ID() {
		vec12P(t, "P5.symlink", "OK", "symlink para arquivo comum 0600 é seguido e o alvo é validado pelo handle (f.Stat): mesmo comportamento do --admin-file")
	} else {
		vec12P(t, "P5.symlink", "OK", fmt.Sprintf("symlink recusado: %v", err))
	}
}

type adv12PHolder struct {
	Name string
	K    Key // by value: what a careless struct would do
}

type adv12PHolderPtr struct {
	Name string
	K    *Key
	k    *Key
}

func TestAdv12PKeyNeverPrinted(t *testing.T) {
	raw, _ := hex.DecodeString(adv12PHex)
	k, _ := New(raw)
	outs := map[string]string{}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%p"} {
		outs["ptr "+verb] = fmt.Sprintf(verb, k)
	}
	b, _ := json.Marshal(k)
	outs["json"] = string(b)
	b, _ = json.Marshal(adv12PHolderPtr{Name: "x", K: k, k: k})
	outs["json struct"] = string(b)
	outs["struct ptr %+v"] = fmt.Sprintf("%+v", adv12PHolderPtr{Name: "x", K: k, k: k})
	outs["struct ptr %#v"] = fmt.Sprintf("%#v", adv12PHolderPtr{Name: "x", K: k, k: k})
	outs["errorf %v"] = fmt.Errorf("falhou com %v: %w", k, errors.New("x")).Error()
	outs["slice"] = fmt.Sprint([]*Key{k, nil})
	outs["map"] = fmt.Sprint(map[string]*Key{"a": k})
	var lb bytes.Buffer
	log.New(&lb, "", 0).Printf("chave %v %+v", k, k)
	slog.New(slog.NewTextHandler(&lb, nil)).Info("x", "k", k, "kk", slog.AnyValue(k))
	slog.New(slog.NewJSONHandler(&lb, nil)).Info("x", "k", k)
	outs["log+slog"] = lb.String()
	func() {
		defer func() { outs["panic"] = fmt.Sprint(recover()) }()
		panic(k)
	}()
	var leaked []string
	for name, s := range outs {
		if adv12PLeaks(s, raw) {
			leaked = append(leaked, name+": "+s)
		}
	}
	if len(leaked) == 0 {
		vec12P(t, "P5.fmtPonteiro", "OK", fmt.Sprintf("%d formas (verbos, json, slog Text/JSON, log, errors, panic, struct com *Key exportado e não exportado) mostram só hmackey(<key_id>)", len(outs)))
	} else {
		vec12P(t, "P5.fmtPonteiro", "FALHA", "Bloqueador gateway/hmackey/hmackey.go:103 a chave aparece: "+strings.Join(leaked, " | "))
	}

	// the same Key as a VALUE (dereferenced, or embedded by value in a struct): the methods are on *Key,
	// so fmt walks the unexported fields with reflection.
	val := map[string]string{
		"*k %v":      fmt.Sprintf("%v", *k),
		"*k %+v":     fmt.Sprintf("%+v", *k),
		"struct %+v": fmt.Sprintf("%+v", adv12PHolder{Name: "x", K: *k}),
		"struct %#v": fmt.Sprintf("%#v", adv12PHolder{Name: "x", K: *k}),
		"slog struct": func() string {
			var b bytes.Buffer
			slog.New(slog.NewTextHandler(&b, nil)).Info("x", "h", adv12PHolder{K: *k})
			return b.String()
		}(),
		"panic *k":      func() (s string) { defer func() { s = fmt.Sprint(recover()) }(); panic(*k) }(),
		"errorf *k %+v": fmt.Errorf("%+v", *k).Error(),
	}
	var vleak []string
	for name, s := range val {
		if adv12PLeaks(s, raw) {
			vleak = append(vleak, name)
		}
	}
	if len(vleak) == 0 {
		vec12P(t, "P5.fmtValor", "OK", "Key por valor também não mostra a chave")
	} else {
		sample := fmt.Sprintf("%v", *k)
		if len(sample) > 60 {
			sample = sample[:60] + "..."
		}
		vec12P(t, "P5.fmtValor", "FALHA", fmt.Sprintf("Menor gateway/hmackey/hmackey.go:36 (latente: nenhum código de produção guarda Key por valor) String/GoString/Format estão no receptor *Key; "+
			"um Key desreferenciado ou embutido por valor imprime os bytes da chave por reflexão em %d formas (%s), ex.: %q", len(vleak), strings.Join(vleak, ", "), sample))
	}
}
