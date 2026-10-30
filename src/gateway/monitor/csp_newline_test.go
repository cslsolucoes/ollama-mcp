package monitor

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// browserHash hashes an inline element the way a browser does for CSP: the HTML
// parser has already folded CRLF (and lone CR) to LF.
func browserHash(b []byte) string {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	b = bytes.ReplaceAll(b, []byte("\r"), []byte("\n"))
	sum := sha256.Sum256(b)
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// Regression (2026-10-30, live): the binary was built from a CRLF checkout of
// index.html; the CSP hashed the raw bytes, the browser hashed the LF-folded
// text, and blocked the page's script and style ("conectando..." forever).
func TestCSPHashesMatchWhatTheBrowserHashes(t *testing.T) {
	if bytes.Contains(indexHTML, []byte("\r")) {
		t.Fatal("served page still has CR bytes")
	}
	csp, err := buildCSP(indexHTML)
	if err != nil {
		t.Fatal(err)
	}
	for _, re := range [...]struct {
		name string
		find func([]byte) [][]byte
	}{{"script", scriptRe.FindSubmatch}, {"style", styleRe.FindSubmatch}} {
		m := re.find(indexHTML)
		if m == nil {
			t.Fatalf("%s not found", re.name)
		}
		if h := browserHash(m[1]); !strings.Contains(csp, h) {
			t.Errorf("%s: CSP lacks the browser's hash %s", re.name, h)
		}
	}
}

// The same holds when the embedded source has CRLF line ends: the fold happens
// before hashing and serving, whatever the checkout looked like.
func TestCSPWithCRLFSource(t *testing.T) {
	crlf := bytes.ReplaceAll(bytes.ReplaceAll(indexHTML, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
	folded := bytes.ReplaceAll(crlf, []byte("\r\n"), []byte("\n"))
	csp, err := buildCSP(folded)
	if err != nil {
		t.Fatal(err)
	}
	if m := scriptRe.FindSubmatch(crlf); m == nil || !strings.Contains(csp, browserHash(m[1])) {
		t.Fatal("CRLF source: script hash would not match the browser")
	}
}
