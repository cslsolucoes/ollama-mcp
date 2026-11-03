// Package hmackey holds the HMAC key of the training-data contract (Fase 12,
// docs/CONTRATO-DADOS-TREINO.md, C-4.3) and the pure functions that use it.
//
// What the key protects: the gateway never stores a plain sha256 of a document,
// of an image or of a request (C-2.5, C-2.12, C-4.2); it stores an HMAC-SHA256
// under this key, so a leaked database does not let anyone test a guess against
// the hashes. All gateways that compute and store HMACs share one key; every
// stored hash carries the key id (the first 16 hex digits of the sha256 of the
// key bytes), and hashes are compared only when the key ids are equal (C-4.3a).
//
// The key file holds at least 32 random bytes written as hex (64 characters or
// more). On Unix it must not be readable by group or others. Nothing in this
// package ever returns, logs or formats the key bytes.
package hmackey

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
)

// MinKeyBytes is the shortest key accepted (256 bits).
const MinKeyBytes = 32

// Key is an HMAC key. A nil *Key is valid and means "no key configured": every
// method then returns "".
type Key struct {
	key []byte
	id  string
}

// New builds a Key from raw bytes (at least MinKeyBytes).
func New(raw []byte) (*Key, error) {
	if len(raw) < MinKeyBytes {
		return nil, fmt.Errorf("chave HMAC curta demais: %d bytes (mínimo %d)", len(raw), MinKeyBytes)
	}
	sum := sha256.Sum256(raw)
	return &Key{key: append([]byte(nil), raw...), id: hex.EncodeToString(sum[:])[:16]}, nil
}

// Load reads the key file written as hex. On Unix the file must be 0600 (no
// access for group or others), like --admin-file. Errors never carry the key.
func Load(path string) (*Key, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("--doc-key-file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat() // the handle, not the path: no swap between check and read
	if err != nil {
		return nil, fmt.Errorf("--doc-key-file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("--doc-key-file %s não é um arquivo comum", path)
	}
	if err := checkMode(runtime.GOOS, fi.Mode()); err != nil {
		return nil, fmt.Errorf("--doc-key-file %s: %w", path, err)
	}
	if fi.Size() > 4096 {
		return nil, errors.New("--doc-key-file: arquivo grande demais para uma chave em hex")
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(f, 4097)); err != nil { // the file may have grown after Stat
		return nil, fmt.Errorf("--doc-key-file: %w", err)
	}
	if buf.Len() > 4096 {
		return nil, errors.New("--doc-key-file: arquivo grande demais para uma chave em hex")
	}
	txt := strings.TrimSpace(buf.String())
	raw, err := hex.DecodeString(txt)
	if err != nil {
		return nil, errors.New("--doc-key-file: o conteúdo precisa ser a chave em hexadecimal (64 caracteres ou mais)")
	}
	k, err := New(raw)
	if err != nil {
		return nil, fmt.Errorf("--doc-key-file: %w", err)
	}
	return k, nil
}

// checkMode refuses a key file that group or others can access, except on
// Windows (where modes do not exist). It is a function of goos so a test can
// exercise the Linux rule on any host.
func checkMode(goos string, mode os.FileMode) error {
	if goos != "windows" && mode.Perm()&0o077 != 0 {
		return fmt.Errorf("tem permissão %v: use 0600", mode.Perm())
	}
	return nil
}

// String, GoString and Format make sure no print verb ever shows the key: all
// of %v, %+v, %#v, %s and %x give only hmackey(<key id>).
func (k *Key) String() string {
	if !k.Configured() {
		return "hmackey(none)"
	}
	return "hmackey(" + k.id + ")"
}

// GoString implements fmt.GoStringer.
func (k *Key) GoString() string { return k.String() }

// Format implements fmt.Formatter for every verb.
func (k *Key) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, k.String()) }

// Configured tells whether a key is present.
func (k *Key) Configured() bool { return k != nil && len(k.key) > 0 }

// ID returns the key id (16 hex digits of the sha256 of the key), "" without key.
func (k *Key) ID() string {
	if !k.Configured() {
		return ""
	}
	return k.id
}

// MAC returns hex(HMAC-SHA256(key, data)), "" without key.
func (k *Key) MAC(data []byte) string {
	if !k.Configured() {
		return ""
	}
	m := hmac.New(sha256.New, k.key)
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}

// MACString is MAC for a string.
func (k *Key) MACString(s string) string {
	if !k.Configured() {
		return ""
	}
	return k.MAC([]byte(s))
}

// ValidDocHeader reports whether v is a value the X-Mcpollama-Doc header may
// carry: exactly 64 lower-case hex digits (a sha256). Anything else is ignored.
func ValidDocHeader(v string) bool {
	if len(v) != 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// DocHash is HMAC-SHA256(key, value of X-Mcpollama-Doc) in hex (C-4.2); "" when
// there is no key or the header value is not 64 lower-case hex digits.
func (k *Key) DocHash(header string) string {
	if !ValidDocHeader(header) {
		return ""
	}
	return k.MACString(header)
}

// PromptHMAC is HMAC-SHA256(key, sha256(prompt_json)) in hex (C-2.12). The
// message is the 32 raw bytes of the digest. promptJSON is the value stored in
// the content database, with the image markers already in place.
func (k *Key) PromptHMAC(promptJSON []byte) string {
	if !k.Configured() {
		return ""
	}
	sum := sha256.Sum256(promptJSON)
	return k.MAC(sum[:])
}

// ContentHMAC is HMAC-SHA256(key, sha256(canonical JSON of system, prompt and
// messages)) in hex (C-2.12). The canonical JSON is an object holding only the
// keys "messages", "prompt" and "system" that the request has, with every
// object's keys sorted and no spaces; model, options, format, think and every
// other key are left out, so the same text asked with other options gives the
// same value. The message is the 32 raw bytes of the digest. It fails when
// promptJSON is not a JSON object (for instance a truncated one).
func (k *Key) ContentHMAC(promptJSON []byte) (string, error) {
	canon, err := CanonicalContent(promptJSON)
	if err != nil {
		return "", err
	}
	if !k.Configured() {
		return "", nil
	}
	sum := sha256.Sum256(canon)
	return k.MAC(sum[:]), nil
}

// CanonicalContent returns the canonical JSON that ContentHMAC hashes.
func CanonicalContent(promptJSON []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(promptJSON, &top); err != nil || top == nil {
		return nil, errors.New("prompt_json não é um objeto JSON válido")
	}
	names := make([]string, 0, len(top))
	for name := range top {
		names = append(names, name)
	}
	sort.Strings(names) // names that differ only in case: a fixed order, so the result is deterministic
	picked := map[string]any{}
	for _, name := range names {
		raw := top[name]
		// The Ollama's decoder matches names by Unicode case folding
		// (strings.EqualFold), so "meſſages" is "messages" for it and for us.
		low := ""
		for _, want := range [...]string{"system", "prompt", "messages"} {
			if strings.EqualFold(name, want) {
				low = want
				break
			}
		}
		if low == "" {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber() // numbers keep their text
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, errors.New("prompt_json: valor inválido em " + low)
		}
		picked[low] = v
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(picked); err != nil { // map keys come out sorted
		return nil, err
	}
	return bytes.TrimRight(out.Bytes(), "\n"), nil
}
