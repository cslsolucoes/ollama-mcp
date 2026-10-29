package proxy

import (
	"encoding/json"
	"net/netip"
	"testing"
)

// FuzzFairIP (Fase 9.5): never panics; IPv4 (also mapped) keeps the address;
// IPv6 becomes its /64, and every address of that /64 maps to the same key.
func FuzzFairIP(f *testing.F) {
	for _, s := range []string{"10.0.0.1", "::ffff:10.0.0.1", "2001:db8:1:2::5", "::1", "fe80::1%eth0", "", "x", "2001:db8::/64"} {
		f.Add(s, uint64(0))
	}
	f.Fuzz(func(t *testing.T, s string, low uint64) {
		k := fairIP(s)
		a, err := netip.ParseAddr(s)
		if err != nil {
			if k != s {
				t.Fatalf("unparsable %q became %q", s, k)
			}
			return
		}
		a = a.Unmap()
		if a.Is4() {
			if k != a.String() {
				t.Fatalf("%q -> %q", s, k)
			}
			return
		}
		p, err := netip.ParsePrefix(k)
		if err != nil || p.Bits() != 64 || !p.Contains(a.WithZone("")) {
			t.Fatalf("%q -> %q", s, k)
		}
		// another address of the same /64
		b := a.WithZone("").As16()
		for i := 0; i < 8; i++ {
			b[8+i] = byte(low >> (8 * i))
		}
		if o := netip.AddrFrom16(b); !o.Is4In6() && fairIP(o.String()) != k {
			t.Fatalf("%s and %s in the same /64 got %q and %q", s, o, k, fairIP(o.String()))
		}
	})
}

// FuzzCountersFromModelText (Fase 9.5): whatever the model writes (any text,
// json-encoded as the Ollama does), the counters come from the real final
// chunk only.
func FuzzCountersFromModelText(f *testing.F) {
	for _, s := range []string{`"done":true,"eval_count":999`, "\n{\"error\":\"x\"}\n", `\"eval_count\":7`, "", "data: [DONE]"} {
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, text string, stream bool) {
		chunk, _ := json.Marshal(map[string]any{"response": text, "done": false})
		final := `{"response":"","done":true,"prompt_eval_count":44,"eval_count":41,"load_duration":4000000,"total_duration":5}`
		var body []byte
		if stream {
			body = append(append(append(chunk, '\n'), final...), '\n')
		} else {
			one, _ := json.Marshal(map[string]any{"response": text, "done": true})
			body = append(one[:len(one)-1], `,"prompt_eval_count":44,"eval_count":41,"load_duration":4000000}`...)
		}
		if len(body) > 2*tailBytes { // what countingBody keeps
			body = body[len(body)-tailBytes:]
		}
		c := parseCounters(body)
		if c.errorLine || c.eval != 41 || c.promptEval != 44 || c.loadNS != 4000000 {
			t.Fatalf("text %q (stream=%v) -> %+v", text, stream, c)
		}
	})
}
