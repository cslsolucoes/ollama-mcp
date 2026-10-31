package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cslsoftwares/mcpollama/ollama"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func privIface(mac string) ifaceInfo {
	hw, _ := net.ParseMAC(mac)
	_, n, _ := net.ParseCIDR("10.100.1.5/24")
	n.IP = net.ParseIP("10.100.1.5")
	return ifaceInfo{Up: true, HardwareAddr: hw, Addrs: []net.Addr{n}}
}

func TestStationPlacaValid(t *testing.T) {
	env := map[string]string{"COMPUTERNAME": "002-1985", "USERDOMAIN": "CSLSOLUCOES", "PLACA": "5CCD5BF40B2D"}
	got := buildStation(envOf(env), "host", `X\claiton.linhares`, nil, nil).header()
	want := "computer=002-1985;user=claiton.linhares;domain=CSLSOLUCOES;mac=5CCD5BF40B2D"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStationPlacaSeparators(t *testing.T) {
	for _, p := range []string{"5c:cd:5b:f4:0b:2d", "5C-CD-5B-F4-0B-2D", "5ccd5bf40b2d"} {
		if s := buildStation(envOf(map[string]string{"PLACA": p}), "h", "u", nil, nil); s.MAC != "5CCD5BF40B2D" {
			t.Errorf("PLACA %q -> %q", p, s.MAC)
		}
	}
}

func TestStationPlacaInvalidFallsBackToInterface(t *testing.T) {
	ifs := []ifaceInfo{
		{Up: true, Loopback: true},
		{Up: false, HardwareAddr: net.HardwareAddr{1, 2, 3, 4, 5, 6}},
		privIface("aa:bb:cc:dd:ee:ff"),
	}
	for _, p := range []string{"", "ZZCD5BF40B2D", "5CCD5BF40B", "5CCD5BF40B2D00", "5CCD;BF40B2D"} {
		if s := buildStation(envOf(map[string]string{"PLACA": p}), "h", "u", nil, ifs); s.MAC != "AABBCCDDEEFF" {
			t.Errorf("PLACA %q -> %q", p, s.MAC)
		}
	}
	// no suitable interface: the field is omitted
	if s := buildStation(envOf(nil), "h", "u", nil, []ifaceInfo{{Up: true}}); s.MAC != "" || strings.Contains(s.header(), "mac=") {
		t.Fatalf("mac must be omitted: %q", s.header())
	}
}

func TestStationDomainFromCurrentUser(t *testing.T) {
	s := buildStation(envOf(map[string]string{}), "meu-host", `CSLSOLUCOES\ana`, nil, nil)
	if s.Computer != "meu-host" || s.User != "ana" || s.Domain != "CSLSOLUCOES" {
		t.Fatalf("%+v", s)
	}
	if h := buildStation(envOf(nil), "", "", nil, nil).header(); h != "" {
		t.Fatalf("empty station must render empty, got %q", h)
	}
}

func TestStationSanitize(t *testing.T) {
	s := buildStation(envOf(map[string]string{
		"COMPUTERNAME": "a;b=c\r\n\"d",
		"USERDOMAIN":   strings.Repeat("x", 200),
	}), "h", "us er", nil, nil)
	if s.Computer != "a_b_c___d" || s.User != "us_er" || len(s.Domain) != 64 {
		t.Fatalf("%+v", s)
	}
}

func TestStationFlag(t *testing.T) {
	parse := func(args []string, env map[string]string) Config {
		var buf bytes.Buffer
		c, err := parseConfig(args, envOf(env), &buf)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := parse(nil, nil); !c.StationInfo || c.ollamaOptions().StationInfo == "" {
		t.Fatalf("default must be on with a header")
	}
	if c := parse([]string{"--station-info=false"}, nil); c.StationInfo || c.ollamaOptions().StationInfo != "" {
		t.Fatalf("flag false")
	}
	if c := parse(nil, map[string]string{"MCPOLLAMA_STATION_INFO": "false"}); c.StationInfo || c.ollamaOptions().StationInfo != "" {
		t.Fatalf("env false")
	}
	if c := parse([]string{"--station-info"}, map[string]string{"MCPOLLAMA_STATION_INFO": "false"}); !c.StationInfo {
		t.Fatalf("flag must win over env")
	}
}

func TestStationHeaderSentOnWire(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-Mcpollama-Station"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	for _, off := range []bool{false, true} {
		args := []string{"--ollama-url", srv.URL}
		if off {
			args = append(args, "--station-info=false")
		}
		var buf bytes.Buffer
		cfg, err := parseConfig(args, envOf(map[string]string{"PLACA": "5CCD5BF40B2D", "COMPUTERNAME": "002-1985", "USERDOMAIN": "CSLSOLUCOES"}), &buf)
		if err != nil {
			t.Fatal(err)
		}
		cl, err := ollama.New(cfg.ollamaOptions())
		if err != nil {
			t.Fatal(err)
		}
		got.Store("")
		_, _ = cl.Tags(context.Background())
		h, _ := got.Load().(string)
		if off {
			if h != "" {
				t.Fatalf("header sent with --station-info=false: %q", h)
			}
			continue
		}
		t.Logf("X-Mcpollama-Station real: %s", h)
		if !strings.HasPrefix(h, "computer=002-1985;user=") || !strings.Contains(h, ";domain=CSLSOLUCOES;mac=") {
			t.Fatalf("unexpected header %q", h)
		}
	}
}

func TestPickMAC(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"", "zz:zz", "5c:cd:5b:f4:0b:2d", "AA:BB:CC:DD:EE:FF"}, "5CCD5BF40B2D"},
		{[]string{"aa-bb-cc-dd-ee-ff"}, "AABBCCDDEEFF"},
		{[]string{"", ""}, ""},
	}
	for _, c := range cases {
		if got := pickMAC(c.in); got != c.want {
			t.Errorf("pickMAC(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStationMACPrecedence(t *testing.T) {
	env := envOf(map[string]string{"PLACA": "111111111111"})
	ifs := []ifaceInfo{privIface("aa:bb:cc:dd:ee:ff")}
	if s := buildStation(env, "h", "u", []string{"22:22:22:22:22:22"}, ifs); s.MAC != "222222222222" {
		t.Fatalf("WMI must win: %q", s.MAC)
	}
	if s := buildStation(env, "h", "u", []string{""}, ifs); s.MAC != "111111111111" {
		t.Fatalf("PLACA second: %q", s.MAC)
	}
	if s := buildStation(envOf(nil), "h", "u", nil, ifs); s.MAC != "AABBCCDDEEFF" {
		t.Fatalf("interface last: %q", s.MAC)
	}
}
