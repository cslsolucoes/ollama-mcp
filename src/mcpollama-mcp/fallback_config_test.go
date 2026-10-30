package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fallback is off unless --ollama-url-fallback is set (Fase 10.4).
func TestFallbackOffByDefault(t *testing.T) {
	c := parse(t, nil, nil)
	if c.FallbackURL != "" || c.FallbackAPIKey != "" || c.FallbackTLSCA != "" {
		t.Fatalf("fallback on by default: %+v", c)
	}
	if o := c.ollamaOptions(); o.FallbackURL != "" || o.FallbackAPIKey != "" {
		t.Fatalf("options %+v", o)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackFlagsAndEnv(t *testing.T) {
	c := parse(t, []string{"--ollama-url-fallback", "https://10.1.1.1:8443", "--fallback-api-key", "kflag"},
		map[string]string{"OLLAMA_HOST_FALLBACK": "https://env", "OLLAMA_FALLBACK_API_KEY": "kenv", "OLLAMA_FALLBACK_TLS_CA": "/x.pem"})
	if c.FallbackURL != "https://10.1.1.1:8443" || c.FallbackAPIKey != "kflag" || c.FallbackTLSCA != "/x.pem" {
		t.Fatalf("flag > env > default: %+v", c)
	}
	c = parse(t, nil, map[string]string{"OLLAMA_HOST_FALLBACK": "https://env:8443"})
	if c.FallbackURL != "https://env:8443" {
		t.Fatalf("env: %+v", c)
	}
}

func TestFallbackValidation(t *testing.T) {
	bad := []Config{
		{FallbackURL: "ftp://x"},
		{FallbackAPIKey: "k"}, // a token without a URL
		{FallbackURL: "http://h", FallbackTLSCA: filepath.Join(t.TempDir(), "nope.pem")}, // unreadable CA
	}
	for i, f := range bad {
		c := parse(t, nil, nil)
		c.FallbackURL, c.FallbackAPIKey, c.FallbackTLSCA = f.FallbackURL, f.FallbackAPIKey, f.FallbackTLSCA
		if err := c.validate(); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	// secrets() lists the fallback token so every log and error is scrubbed
	c := parse(t, []string{"--ollama-url-fallback", "https://h", "--fallback-api-key", "FBKEY"}, nil)
	found := false
	for _, s := range c.secrets() {
		if s == "FBKEY" {
			found = true
		}
	}
	if !found {
		t.Fatal("the fallback token is not among the secrets to scrub")
	}
	// the usage text documents the three flags
	var sb strings.Builder
	sb.WriteString(usageText)
	for _, f := range []string{"--ollama-url-fallback", "--fallback-api-key", "--fallback-tls-ca"} {
		if !strings.Contains(sb.String(), f) {
			t.Errorf("usage lacks %s", f)
		}
	}
	_ = os.Stdout
}
