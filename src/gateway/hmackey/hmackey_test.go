package hmackey

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Key 00 01 02 ... 1f. Every expected value below was computed outside Go with
// openssl (`openssl dgst -sha256 -mac HMAC -macopt hexkey:...`), so the tests do
// not repeat the code under test.
const (
	testKeyHex = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	testKeyID  = "630dcd2966c43366" // first 16 hex of sha256(key bytes)
)

func testKey(t *testing.T) *Key {
	t.Helper()
	k, err := Load(writeKey(t, testKeyHex+"\n", 0o600))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func writeKey(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.key")
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(p, mode)
	return p
}

// C-4.3, C-4.3a: the key id is the first 16 hex of the sha256 of the key bytes.
func TestKeyIDAndMACVector(t *testing.T) {
	k := testKey(t)
	if k.ID() != testKeyID {
		t.Fatalf("key id = %q, want %q", k.ID(), testKeyID)
	}
	if got := k.MACString("ab"); got != "2e2f68fb95e31672b7a745ed580c12454641604e37fc2f1867fb34fae4a03973" {
		t.Fatalf("MAC(ab) = %s", got)
	}
}

// C-4.3: a short key is refused; so is anything that is not hex.
func TestLoadRefusesShortAndNonHexKeys(t *testing.T) {
	for name, content := range map[string]string{
		"31 bytes":  strings.Repeat("ab", 31),
		"empty":     "",
		"not hex":   strings.Repeat("zz", 32),
		"odd":       strings.Repeat("a", 65),
		"raw text":  "esta chave nao e hexadecimal mas tem bastante texto para passar de 32",
		"half line": testKeyHex[:63],
	} {
		t.Run(name, func(t *testing.T) {
			k, err := Load(writeKey(t, content, 0o600))
			if err == nil || k != nil {
				t.Fatalf("a key file with %q was accepted", name)
			}
			if strings.Contains(err.Error(), content) && content != "" {
				t.Fatalf("the error repeats the key content: %v", err)
			}
		})
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing file was accepted")
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("a directory was accepted")
	}
}

// 32 bytes (64 hex) is the minimum and longer keys are fine.
func TestLoadAcceptsMinimumAndLongerKeys(t *testing.T) {
	for _, n := range []int{32, 48, 64} {
		if _, err := Load(writeKey(t, strings.Repeat("a1", n), 0o600)); err != nil {
			t.Fatalf("%d bytes: %v", n, err)
		}
	}
}

// C-4.3: on Unix the file must not be readable by group or others. The mode
// rule is a pure function so it is tested on every OS.
func TestModeRule(t *testing.T) {
	for _, tc := range []struct {
		goos string
		mode os.FileMode
		ok   bool
	}{
		{"linux", 0o600, true}, {"linux", 0o400, true},
		{"linux", 0o640, false}, {"linux", 0o604, false}, {"linux", 0o644, false}, {"linux", 0o660, false}, {"linux", 0o777, false},
		{"windows", 0o666, true},
	} {
		err := checkMode(tc.goos, tc.mode)
		if (err == nil) != tc.ok {
			t.Errorf("checkMode(%s, %v) = %v, want ok=%v", tc.goos, tc.mode, err, tc.ok)
		}
	}
}

// Load applies the rule to the real file (skipped where modes do not exist).
func TestLoadRefusesWorldReadableOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes are not enforced on Windows")
	}
	if _, err := Load(writeKey(t, testKeyHex, 0o644)); err == nil {
		t.Fatal("a 0644 key file was accepted")
	}
}

// Without a key every method answers "" and nothing panics (nil *Key).
func TestNilKeyIsHarmless(t *testing.T) {
	var k *Key
	if k.Configured() || k.ID() != "" || k.MAC([]byte("x")) != "" || k.MACString("x") != "" ||
		k.PromptHMAC([]byte("{}")) != "" || k.DocHash(strings.Repeat("a", 64)) != "" {
		t.Fatal("a nil key produced a value")
	}
	if h, err := k.ContentHMAC([]byte(`{"prompt":"a"}`)); err != nil || h != "" {
		t.Fatalf("nil key ContentHMAC = %q, %v", h, err)
	}
}

// C-2.11, C-4.2: only 64 lower-case hex digits are a document header.
func TestValidDocHeader(t *testing.T) {
	good := strings.Repeat("0123456789abcdef", 4)
	if !ValidDocHeader(good) {
		t.Fatal("a valid header was refused")
	}
	for _, bad := range []string{
		"", good[:63], good + "a", strings.ToUpper(good), strings.Repeat("g", 64), " " + good[1:],
		good[:63] + "\n", good[:32] + " " + good[33:],
	} {
		if ValidDocHeader(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// C-4.2: doc_hash is the HMAC of the header value, never the value itself.
func TestDocHashIsHMACAndNeverPlain(t *testing.T) {
	k := testKey(t)
	v := strings.Repeat("ab", 32)
	got := k.DocHash(v)
	if got != "583630234f7e60ecfa2a6ae2e98ee37876e3ff6344457723cb4b42fcf77f7fc0" {
		t.Fatalf("DocHash = %s", got)
	}
	if got == v {
		t.Fatal("the plain value was returned")
	}
	if k.DocHash("not a sha256") != "" || k.DocHash(strings.ToUpper(v)) != "" {
		t.Fatal("an invalid header produced a hash")
	}
}

// C-2.12: prompt_hmac = HMAC(key, sha256(prompt_json)), fixed vector.
func TestPromptHMACVector(t *testing.T) {
	k := testKey(t)
	got := k.PromptHMAC([]byte(`{"model":"m","prompt":"abc"}`))
	if got != "be53cf29f24020947c1474d8405919af39e5f7c5c81cb78850625b27b56b5d78" {
		t.Fatalf("PromptHMAC = %s", got)
	}
}

// C-2.12: conteudo_hmac hashes the canonical JSON of system, prompt and messages.
func TestContentHMACVectorsAndCanonicalForm(t *testing.T) {
	k := testKey(t)
	// The canonical text itself: keys sorted, no spaces, only the three keys.
	c, err := CanonicalContent([]byte(`{ "system" : "s", "options":{"seed":1}, "prompt":"abc", "model":"m", "format":"json", "think":true }`))
	if err != nil || string(c) != `{"prompt":"abc","system":"s"}` {
		t.Fatalf("canonical = %q, %v", c, err)
	}
	h, err := k.ContentHMAC([]byte(`{"model":"m","options":{"seed":1},"prompt":"abc","system":"s"}`))
	if err != nil || h != "b22d94c28a77e7de67a966630bead5d3f4c797927163b1eeca8a6fb276adf1a0" {
		t.Fatalf("ContentHMAC = %s, %v", h, err)
	}
	// Inner objects are sorted too.
	h, err = k.ContentHMAC([]byte(`{"messages":[{"role":"user","content":"oi"}],"model":"m"}`))
	if err != nil || h != "3717fea966da0b63ea5ecbb89f01c3d1a0a14c4e17bbc925e1b30c0e7584a48d" {
		t.Fatalf("ContentHMAC(messages) = %s, %v", h, err)
	}
}

// C-2.12: the same text asked with other model, options, format or think gives
// the same conteudo_hmac; other text gives another one.
func TestContentHMACIgnoresModelOptionsFormatThink(t *testing.T) {
	k := testKey(t)
	base, _ := k.ContentHMAC([]byte(`{"model":"a","prompt":"p","system":"s","options":{"seed":1}}`))
	for _, other := range []string{
		`{"model":"b","prompt":"p","system":"s","options":{"seed":2,"temperature":0.9}}`,
		`{"prompt":"p","system":"s","format":"json","think":true}`,
		`{"system":"s","think":false,"prompt":"p","keep_alive":"5m","stream":false}`,
	} {
		if h, _ := k.ContentHMAC([]byte(other)); h != base {
			t.Errorf("%s: conteudo_hmac changed", other)
		}
	}
	for _, diff := range []string{
		`{"prompt":"p2","system":"s"}`, `{"prompt":"p","system":"s2"}`, `{"prompt":"p"}`,
		`{"prompt":"p","system":"s","messages":[]}`,
	} {
		if h, _ := k.ContentHMAC([]byte(diff)); h == base {
			t.Errorf("%s: conteudo_hmac did not change", diff)
		}
	}
}

// Numbers keep their text, strings keep their characters, and a value that is
// not an object is an error (a truncated prompt_json cannot give a hash).
func TestContentHMACNumbersEscapesAndErrors(t *testing.T) {
	k := testKey(t)
	a, _ := k.ContentHMAC([]byte(`{"messages":[{"role":"user","content":"a<b>&é"}]}`))
	b, _ := k.ContentHMAC([]byte(`{"messages":[{"content":"a<b>&é","role":"user"}]}`))
	if a != b {
		t.Error("the same string written with an escape gave another hash")
	}
	if c, _ := CanonicalContent([]byte(`{"prompt":"a<b>&"}`)); string(c) != `{"prompt":"a<b>&"}` {
		t.Errorf("HTML characters were escaped: %s", c)
	}
	n1, _ := k.ContentHMAC([]byte(`{"messages":[{"x":1.50}]}`))
	n2, _ := k.ContentHMAC([]byte(`{"messages":[{"x":1.5}]}`))
	if n1 == n2 {
		t.Error("1.50 and 1.5 were treated as the same number text")
	}
	for _, bad := range []string{``, `[]`, `"x"`, `{"prompt":`, `null`, `{"prompt":"cortado`} {
		if _, err := k.ContentHMAC([]byte(bad)); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	// Names that differ only in case: a fixed result, whatever the map order.
	first, _ := k.ContentHMAC([]byte(`{"System":"a","system":"b","prompt":"p"}`))
	for i := 0; i < 20; i++ {
		if h, _ := k.ContentHMAC([]byte(`{"System":"a","system":"b","prompt":"p"}`)); h != first {
			t.Fatal("the result depends on the map order")
		}
	}
}

// I1: the Ollama matches names by Unicode case folding; "meſſages" (long s) is
// "messages" for it, so it must not vanish from the canonical content.
func TestContentHMACFoldedNames(t *testing.T) {
	k := testKey(t)
	c, err := CanonicalContent([]byte(`{"meſſages":[{"role":"user","content":"oi"}],"ſystem":"s","PROMPT":"p","model":"m"}`))
	if err != nil || string(c) != `{"messages":[{"content":"oi","role":"user"}],"prompt":"p","system":"s"}` {
		t.Fatalf("canonical = %q, %v", c, err)
	}
	a, _ := k.ContentHMAC([]byte(`{"meſſages":[{"role":"user","content":"oi"}]}`))
	b, _ := k.ContentHMAC([]byte(`{"messages":[{"content":"oi","role":"user"}]}`))
	if a == "" || a != b {
		t.Fatal("a folded name gave another conteudo_hmac")
	}
}

// M3: no print verb shows the key.
func TestKeyNeverPrints(t *testing.T) {
	k := testKey(t)
	out := fmt.Sprintf("%v|%+v|%#v|%s|%x|%q|%d", k, k, k, k, k, k, k) + fmt.Sprint(k) + fmt.Sprintf("%v", []*Key{k}) + fmt.Sprintf("%+v", struct{ K *Key }{k})
	for _, secret := range []string{testKeyHex, testKeyHex[:32], "\x00\x01\x02\x03", "0001020304", "[0 1 2 3"} {
		if strings.Contains(out, secret) {
			t.Fatalf("a print verb shows the key (%q): %s", secret, out)
		}
	}
	if !strings.Contains(out, "hmackey("+testKeyID+")") {
		t.Fatalf("expected the key id: %s", out)
	}
	var none *Key
	if fmt.Sprintf("%v", none) == "" {
		t.Fatal("nil key prints nothing")
	}
}

// M2: a key file over 4 KiB is refused (the read is bounded too).
func TestLoadRefusesOversizeFile(t *testing.T) {
	if _, err := Load(writeKey(t, strings.Repeat("ab", 2600), 0o600)); err == nil {
		t.Fatal("a 5200-byte key file was accepted")
	}
	if _, err := Load(writeKey(t, strings.Repeat("ab", 2048), 0o600)); err != nil {
		t.Fatalf("4096 bytes of hex must still load: %v", err)
	}
}
