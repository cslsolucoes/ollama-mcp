package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func TestParseStation(t *testing.T) {
	long := strings.Repeat("a", 65)
	cases := []struct {
		name string
		in   []string
		want Station
	}{
		{"all four", []string{"computer=002-1985;user=claiton.linhares;domain=CSLSOLUCOES;mac=5CCD5BF40B2D"},
			Station{"002-1985", "claiton.linhares", "CSLSOLUCOES", "5CCD5BF40B2D"}},
		{"only computer", []string{"computer=PC1"}, Station{Computer: "PC1"}},
		{"any order", []string{"mac=AABBCCDDEEFF;computer=X"}, Station{Computer: "X", MAC: "AABBCCDDEEFF"}},
		{"absent", nil, Station{}},
		{"empty", []string{""}, Station{}},
		{"two header lines", []string{"computer=A", "computer=B"}, Station{}},
		{"unknown key", []string{"computer=A;role=admin"}, Station{}},
		{"repeated key", []string{"computer=A;computer=B"}, Station{}},
		{"value 65 chars", []string{"computer=" + long}, Station{}},
		{"value 64 chars", []string{"computer=" + strings.Repeat("a", 64)}, Station{Computer: strings.Repeat("a", 64)}},
		{"mac lower case", []string{"mac=aabbccddeeff"}, Station{}},
		{"mac 11 hex", []string{"mac=AABBCCDDEEF"}, Station{}},
		{"mac 13 hex", []string{"mac=AABBCCDDEEFF0"}, Station{}},
		{"mac with colon", []string{"mac=AA:BB:CC:DD:EE:FF"}, Station{}},
		{"mac not hex", []string{"mac=GGBBCCDDEEFF"}, Station{}},
		{"extra equals", []string{"computer=a=b"}, Station{}},
		{"quote", []string{`computer=a"b`}, Station{}},
		{"newline", []string{"computer=a\nuser=b"}, Station{}},
		{"carriage return", []string{"computer=a\r"}, Station{}},
		{"space", []string{"computer=a b"}, Station{}},
		{"empty value", []string{"computer=;user=x"}, Station{}},
		{"trailing semicolon", []string{"computer=A;"}, Station{}},
		{"semicolon injection", []string{"computer=A;user=B;;mac=AABBCCDDEEFF"}, Station{}},
		{"over 512 bytes", []string{"computer=A;" + strings.Repeat("x", 520)}, Station{}},
		{"one bad value voids all", []string{"computer=OK;user=b@d"}, Station{}},
		{"non ascii", []string{"computer=máquina"}, Station{}},
	}
	for _, c := range cases {
		if got := parseStation(c.in); got != c.want {
			t.Errorf("%s: parseStation(%q) = %+v, want %+v", c.name, c.in, got, c.want)
		}
	}
}

// The header reaches the record, is never refused when bad, and never reaches
// the backend.
func TestStationHeaderReachesRecordNotBackend(t *testing.T) {
	var got http.Header
	e := setup(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		io.WriteString(w, finalChunk)
	}), queue.Config{}, nil)

	resp, _ := post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{
		HeaderStation: "computer=002-1985;user=claiton.linhares;domain=CSLSOLUCOES;mac=5CCD5BF40B2D",
	})
	rec := e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.StationComputer != "002-1985" || rec.StationUser != "claiton.linhares" || rec.StationDomain != "CSLSOLUCOES" || rec.StationMAC != "5CCD5BF40B2D" {
		t.Fatalf("record: %+v", rec)
	}
	if len(got.Values(HeaderStation)) != 0 {
		t.Fatalf("%s leaked to the backend: %q", HeaderStation, got.Values(HeaderStation))
	}
	e.waitIdle(t)

	// bad header: 200 anyway, empty fields
	resp, _ = post(t, e, "/api/generate", `{"model":"m"}`, map[string]string{HeaderStation: `computer=a";user=b`})
	if resp.StatusCode != 200 {
		t.Fatalf("a bad station header refused the request: %d", resp.StatusCode)
	}
	rec = e.waitRecord(t, resp.Header.Get(HeaderRequestID))
	if rec.StationComputer != "" || rec.StationUser != "" || rec.StationDomain != "" || rec.StationMAC != "" {
		t.Fatalf("bad header must give empty fields: %+v", rec)
	}
	if len(got.Values(HeaderStation)) != 0 {
		t.Fatalf("bad header leaked to the backend: %q", got.Values(HeaderStation))
	}
}
