package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func parse(t *testing.T, args []string, e map[string]string) Config {
	t.Helper()
	cfg, err := parseConfig(args, env(e), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseConfig(%v): %v", args, err)
	}
	return cfg
}

func TestDefaults(t *testing.T) {
	c := parse(t, nil, nil)
	if c.Model != "qwen3-ia" || c.EmbedModel != "nomic-embed-text" || c.MaxPromptChars != 128000 ||
		c.ToolsVerbosity != "full" || c.Insecure || c.Debug || c.TraceFile != "" || c.AuditLogPath != "" ||
		c.OllamaURL != "" || c.APIKey != "" || c.Timeout != 120*time.Second {
		t.Fatalf("defaults: %+v", c)
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEnvFallback(t *testing.T) {
	c := parse(t, nil, map[string]string{
		"OLLAMA_HOST": "https://h.invalid:8443", "OLLAMA_API_KEY": "k", "OLLAMA_TLS_CA": "ca.pem",
		"OLLAMA_INSECURE": "1", "OLLAMA_MODEL": "m", "OLLAMA_EMBED_MODEL": "e", "OLLAMA_TIMEOUT": "90s",
		"MCPOLLAMA_DEBUG": "yes", "MCPOLLAMA_TOOLS_VERBOSITY": "compact", "MCPOLLAMA_MAX_PROMPT_CHARS": "1000",
		"MCPOLLAMA_MAX_LINE_BYTES": "2048", "MCPOLLAMA_MAX_FILE_BYTES": "4096",
		"MCPOLLAMA_TRACE": "t.log", "MCPOLLAMA_AUDIT_LOG": "a.log",
	})
	want := Config{OllamaURL: "https://h.invalid:8443", APIKey: "k", TLSCA: "ca.pem", Insecure: true, Model: "m",
		EmbedModel: "e", Timeout: 90 * time.Second, MaxPromptChars: 1000, MaxLineBytes: 2048, MaxFileBytes: 4096,
		Debug: true, ToolsVerbosity: "compact", TraceFile: "t.log", AuditLogPath: "a.log"}
	if c != want {
		t.Fatalf("got  %+v\nwant %+v", c, want)
	}
}

func TestFlagBeatsEnv(t *testing.T) {
	e := map[string]string{"OLLAMA_HOST": "http://env.invalid", "OLLAMA_MODEL": "envm", "OLLAMA_INSECURE": "1",
		"OLLAMA_TIMEOUT": "10s", "MCPOLLAMA_MAX_PROMPT_CHARS": "5", "MCPOLLAMA_DEBUG": "1"}
	c := parse(t, []string{"--ollama-url", "http://flag.invalid", "--model=flagm", "--insecure=false",
		"--timeout", "30s", "--max-prompt-chars", "7", "--debug=0"}, e)
	if c.OllamaURL != "http://flag.invalid" || c.Model != "flagm" || c.Insecure || c.Timeout != 30*time.Second ||
		c.MaxPromptChars != 7 || c.Debug {
		t.Fatalf("%+v", c)
	}
}

func TestBoolRule(t *testing.T) {
	for _, v := range []string{"", "0", "false", "FALSE", "no", "No", "off", "OFF", " 0 "} {
		if parseBool(v) {
			t.Errorf("%q should be false", v)
		}
	}
	for _, v := range []string{"1", "true", "yes", "on", "anything", "2"} {
		if !parseBool(v) {
			t.Errorf("%q should be true", v)
		}
	}
	// the one rule applies to every boolean env
	if !parse(t, nil, map[string]string{"OLLAMA_INSECURE": "maybe"}).Insecure ||
		parse(t, nil, map[string]string{"MCPOLLAMA_DEBUG": "off"}).Debug {
		t.Fatal("env booleans must follow parseBool")
	}
}

func TestBadEnvValues(t *testing.T) {
	for _, e := range []map[string]string{
		{"OLLAMA_TIMEOUT": "soon"}, {"MCPOLLAMA_MAX_LINE_BYTES": "big"},
	} {
		if _, err := parseConfig(nil, env(e), &bytes.Buffer{}); err == nil {
			t.Errorf("%v should fail", e)
		}
	}
}

func TestValidate(t *testing.T) {
	good := parse(t, nil, nil)
	bad := []func(*Config){
		func(c *Config) { c.ToolsVerbosity = "loud" },
		func(c *Config) { c.Timeout = 0 },
		func(c *Config) { c.MaxLineBytes = 0 },
		func(c *Config) { c.MaxPromptChars = -1 },
		func(c *Config) { c.OllamaURL = "not a url" },
		func(c *Config) { c.OllamaURL = "ftp://h" },
		func(c *Config) { c.TLSCA = filepath.Join(t.TempDir(), "missing.pem") },
	}
	for i, mut := range bad {
		c := good
		mut(&c)
		if c.validate() == nil {
			t.Errorf("case %d should fail", i)
		}
	}
	notPEM := filepath.Join(t.TempDir(), "x.pem")
	os.WriteFile(notPEM, []byte("hello"), 0o600)
	c := good
	c.TLSCA = notPEM
	if c.validate() == nil {
		t.Error("non-PEM CA should fail")
	}
}

func TestVersionFlag(t *testing.T) {
	var out, errw bytes.Buffer
	if code := realMain([]string{"--version"}, env(nil), strings.NewReader(""), &out, &errw); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out.String() != "mcpollama-mcp version "+Version+"\n" || errw.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", out.String(), errw.String())
	}
	// --version must not leak the token
	out.Reset()
	realMain([]string{"--version", "--api-key", "SECRETKEY"}, env(nil), strings.NewReader(""), &out, &errw)
	if strings.Contains(out.String()+errw.String(), "SECRETKEY") {
		t.Fatal("token leaked by --version")
	}
}

func TestExitCodes(t *testing.T) {
	var out, errw bytes.Buffer
	// EOF on stdin -> 0
	if code := realMain([]string{"--ollama-url", "https://exemplo.invalid"}, env(nil), strings.NewReader(""), &out, &errw); code != 0 {
		t.Fatalf("EOF exit %d: %s", code, errw.String())
	}
	cases := [][]string{
		{"--bogus"}, {"--tools-verbosity", "x"}, {"--tls-ca", filepath.Join(t.TempDir(), "nope.pem")},
		{"--ollama-url", "::"}, {"stray"},
	}
	for _, args := range cases {
		out.Reset()
		errw.Reset()
		code := realMain(args, env(nil), strings.NewReader(""), &out, &errw)
		if code == 0 || errw.Len() == 0 || out.Len() != 0 {
			t.Errorf("%v: exit %d, stderr %q, stdout %q", args, code, errw.String(), out.String())
		}
	}
}

func TestRealMainServesAndKeepsStdoutClean(t *testing.T) {
	var out, errw bytes.Buffer
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	code := realMain([]string{"--debug", "--ollama-url", "https://exemplo.invalid"}, env(nil), strings.NewReader(in), &out, &errw)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %q", lines)
	}
	for _, l := range lines {
		decode(t, l)
	}
	if !strings.Contains(lines[0], `"version":"`+Version+`"`) {
		t.Fatalf("serverInfo.version must read main.Version: %s", lines[0])
	}
}

func TestTokenNotInUsageOrErrors(t *testing.T) {
	const token = "SUPERSECRETTOKEN"
	e := env(map[string]string{"OLLAMA_API_KEY": token})
	for _, args := range [][]string{{"--bogus"}, {"-h"}, {"--tools-verbosity", "x"}, {"--ollama-url", "::"},
		{"--api-key", token, "--tls-ca", token + ".pem"}, {"--api-key", token, "--timeout", "zz"}} {
		var out, errw bytes.Buffer
		realMain(args, e, strings.NewReader(""), &out, &errw)
		if strings.Contains(out.String()+errw.String(), token) {
			t.Errorf("%v leaked the token:\n%s", args, errw.String())
		}
	}
}

func TestNoLiteralContextLimits(t *testing.T) {
	for _, f := range []string{"config.go", "server.go", "main.go", "logger.go", "../mcp/types.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range []string{"32768", "65536", "131072"} {
			if strings.Contains(string(b), lit) {
				t.Errorf("%s contains literal %s", f, lit)
			}
		}
	}
}

func TestOllamaOptionsFromConfig(t *testing.T) {
	c := Config{OllamaURL: "https://h.invalid:8443", APIKey: "k", TLSCA: "ca.pem", Insecure: true, Model: "m", EmbedModel: "e",
		Timeout: 7 * time.Second, MaxPromptChars: 99, MaxLineBytes: 1234}
	o := c.ollamaOptions()
	if o.BaseURL != c.OllamaURL || o.APIKey != "k" || o.TLSCA != "ca.pem" || !o.Insecure || o.Model != "m" || o.EmbedModel != "e" ||
		o.Timeout != 7*time.Second || o.MaxPromptChars != 99 || o.MaxLineBytes != 1234 {
		t.Fatalf("%+v", o)
	}
}
