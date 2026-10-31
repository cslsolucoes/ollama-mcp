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
	host, _ := os.Hostname()
	if got := cfg.clientLabel(); got != defaultLabel(host, currentUser(), os.Getpid()) || !strings.Contains(got, "/") {
		t.Fatalf("default label %q must be <hostname>/<user>", got)
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

func TestDefaultLabelMachineAndUser(t *testing.T) {
	cases := []struct {
		host, user string
		pid        int
		want       string
	}{
		{"002-1985", `CSL\claiton.linhares`, 42, "002-1985/claiton.linhares"}, // Windows DOMAIN\user
		{"lx-ai01", "root", 42, "lx-ai01/root"},
		{"002-1985", "", 4242, fmt.Sprintf("002-1985/%d", 4242)}, // no user: keep the pid
		{"", "maria", 1, "mcpollama/maria"},
		{"pc", `DOM\`, 9, "pc/9"},
	}
	for _, c := range cases {
		if got := defaultLabel(c.host, c.user, c.pid); got != c.want {
			t.Errorf("defaultLabel(%q,%q,%d) = %q, want %q", c.host, c.user, c.pid, got, c.want)
		}
	}
}
