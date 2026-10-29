package main

import (
	"bytes"
	"net"
	"testing"
	"time"
)

// Fase 9.5, C1 (Baixa): IPv4-mapped forms in --priority-ips mean the IPv4
// prefix; mapped prefixes shorter than /96 and zones are refused.
func TestRegressParseNetsMapped(t *testing.T) {
	cases := []struct {
		in      string
		hit     []string
		miss    []string
		refused bool
	}{
		{in: "::ffff:10.0.0.5", hit: []string{"10.0.0.5", "::ffff:10.0.0.5"}, miss: []string{"10.0.0.6", "8.8.8.8"}},
		{in: "::ffff:10.0.0.0/104", hit: []string{"10.9.9.9"}, miss: []string{"11.0.0.1"}},
		{in: "::ffff:10.100.2.0/120", hit: []string{"10.100.2.77"}, miss: []string{"10.100.3.1"}},
		{in: "10.100.2.5/24", hit: []string{"10.100.2.200"}, miss: []string{"10.100.3.1"}},
		{in: "2001:db8::/32", hit: []string{"2001:db8:ffff::1"}, miss: []string{"2001:db9::1", "10.0.0.1"}},
		{in: "::ffff:10.0.0.0/8", refused: true},
		{in: "fe80::1%eth0", refused: true},
		{in: "10.0.0.0/33", refused: true},
	}
	for _, c := range cases {
		nets, err := parseNets(c.in)
		if c.refused {
			if err == nil {
				t.Errorf("%s accepted", c.in)
			}
			continue
		}
		if err != nil || len(nets) != 1 {
			t.Errorf("%s: %v %v", c.in, nets, err)
			continue
		}
		for _, h := range c.hit {
			if !nets[0].Contains(net.ParseIP(h)) {
				t.Errorf("%s (%v) does not contain %s", c.in, nets[0], h)
			}
		}
		for _, m := range c.miss {
			if nets[0].Contains(net.ParseIP(m)) {
				t.Errorf("%s (%v) contains %s", c.in, nets[0], m)
			}
		}
	}
}

// Fase 9.5, P6/S2: --run-timeout defaults to --max-wait; --max-request-rows
// defaults to 3,000,000; both are validated.
func TestRegressRunTimeoutAndRowCapFlags(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil || c.RunTimeout != 10*time.Minute || c.MaxReqRows != 3_000_000 {
		t.Fatalf("defaults %v %v %v", c.RunTimeout, c.MaxReqRows, err)
	}
	c, _ = parseConfig([]string{"--max-wait", "3m"}, noEnv, &buf)
	if c.RunTimeout != 3*time.Minute {
		t.Fatalf("run timeout must follow --max-wait: %v", c.RunTimeout)
	}
	env := map[string]string{"MCPGW_RUN_TIMEOUT": "90s", "MCPGW_MAX_REQUEST_ROWS": "5000"}
	c, _ = parseConfig([]string{"--max-wait", "3m"}, func(k string) string { return env[k] }, &buf)
	if c.RunTimeout != 90*time.Second || c.MaxReqRows != 5000 {
		t.Fatalf("env: %v %d", c.RunTimeout, c.MaxReqRows)
	}
	for _, a := range [][]string{{"--run-timeout", "0s"}, {"--run-timeout", "-1s"}, {"--max-request-rows", "10"}} {
		c, err := parseConfig(a, noEnv, &buf)
		if err == nil && c.validate() == nil {
			t.Errorf("accepted %v", a)
		}
	}
}
