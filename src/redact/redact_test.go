package redact

import "testing"

func TestNew(t *testing.T) {
	if got := New("")("abc"); got != "abc" {
		t.Fatalf("empty secret must be ignored: %q", got)
	}
	got := New("tok", "", "pw")("a tok b pw c tok")
	if got != "a "+Mask+" b "+Mask+" c "+Mask {
		t.Fatalf("got %q", got)
	}
	if got := New()("x"); got != "x" {
		t.Fatalf("no secrets: %q", got)
	}
}
