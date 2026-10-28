package doctext

import (
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"crlf", "a\r\nb\r\n", "a\nb\n"},
		{"lone cr", "a\rb", "a\nb"},
		{"bom", "\xef\xbb\xbfolá", "olá"},
		{"controls removed", "a\x00b\x01c\x7fd\u0085e\x1b[", "abcde["},
		{"tab and lf kept", "a\tb\nc", "a\tb\nc"},
		{"accents kept", "ção ã õ é", "ção ã õ é"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		got, err := NormalizeText(c.in)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestNormalizeInvalidUTF8(t *testing.T) {
	// 1 invalid byte in 200 runes (0.5%): replaced, accepted.
	in := strings.Repeat("a", 199) + "\xff"
	got, err := NormalizeText(in)
	if err != nil {
		t.Fatalf("0,5%% deveria passar: %v", err)
	}
	if !strings.HasSuffix(got, "�") || strings.ContainsRune(got, 0xFF) {
		t.Fatalf("byte inválido não foi substituído por U+FFFD: %q", got[len(got)-4:])
	}
}

func TestNormalizeFFFDThreshold(t *testing.T) {
	mk := func(bad, total int) string { return strings.Repeat("a", total-bad) + strings.Repeat("\xff", bad) }
	// exactly 1% (1/100): accepted; 2/100: refused.
	if _, err := NormalizeText(mk(1, 100)); err != nil {
		t.Errorf("1%% exato deveria passar: %v", err)
	}
	_, err := NormalizeText(mk(2, 100))
	if r, ok := ReasonOf(err); !ok || r != ReasonCorruptText {
		t.Errorf("2%% deveria recusar com ReasonCorruptText, veio %v", err)
	}
	// genuine U+FFFD characters in valid UTF-8 count as well.
	_, err = NormalizeText(strings.Repeat("a", 90) + strings.Repeat("�", 10))
	if r, _ := ReasonOf(err); r != ReasonCorruptText {
		t.Errorf("U+FFFD literais acima de 1%% deveriam recusar: %v", err)
	}
}

func TestNormalizeDeterministic(t *testing.T) {
	in := "x\r\ny\x00\xffz"
	a, _ := NormalizeText(in)
	for i := 0; i < 20; i++ {
		if b, _ := NormalizeText(in); a != b {
			t.Fatal("saída não determinística")
		}
	}
}
