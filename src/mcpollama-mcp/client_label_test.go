package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestClientLabelDefaultAndOverride(t *testing.T) {
	var buf bytes.Buffer
	cfg, err := parseConfig(nil, func(string) string { return "" }, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientLabel != "" {
		t.Fatalf("default stored label %q", cfg.ClientLabel)
	}
	if got := cfg.clientLabel(); !strings.HasSuffix(got, fmt.Sprintf("/%d", os.Getpid())) {
		t.Fatalf("default label %q must end in /<pid>", got)
	}
	cfg, _ = parseConfig([]string{"--client-label", "agente-7"}, func(string) string { return "" }, &buf)
	if cfg.clientLabel() != "agente-7" || cfg.ollamaOptions().ClientLabel != "agente-7" {
		t.Fatalf("flag not applied: %q", cfg.clientLabel())
	}
	cfg, _ = parseConfig(nil, func(k string) string {
		if k == "MCPOLLAMA_CLIENT_LABEL" {
			return "do-ambiente"
		}
		return ""
	}, &buf)
	if cfg.clientLabel() != "do-ambiente" {
		t.Fatalf("env not applied: %q", cfg.clientLabel())
	}
}
