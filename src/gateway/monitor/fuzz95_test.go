package monitor

import (
	"testing"
	"time"
)

// FuzzLoginLimiter (Fase 9.5): random scripts of tryBegin/end/abort on a few
// IPs with a moving clock. Invariants: no IP ever has more than loginMaxIP
// attempts reserved or failed inside the window at once, pending never goes
// negative, a refused IP is not counted, and a success clears the IP.
func FuzzLoginLimiter(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x20, 0x30})
	f.Fuzz(func(t *testing.T, ops []byte) {
		var l limiter
		now := time.Unix(1_700_000_000, 0)
		open := map[string]int{} // reservations held by the script
		ips := []string{"a", "b", "c"}
		for _, op := range ops {
			ip := ips[int(op>>6)%len(ips)]
			switch op & 7 {
			case 0, 1, 2:
				if ok, retry := l.tryBegin(ip, now); ok {
					open[ip]++
				} else if retry <= 0 || retry > loginMaxLock {
					t.Fatalf("retry %v", retry)
				}
			case 3, 4:
				if open[ip] > 0 {
					open[ip]--
					l.end(ip, now, false)
				}
			case 5:
				if open[ip] > 0 {
					open[ip]--
					l.end(ip, now, true)
					if l.byIP[ip] != nil {
						t.Fatal("success did not clear the IP")
					}
					open[ip] = 0 // a success drops the other reservations of the IP too
				}
			case 6:
				if open[ip] > 0 {
					open[ip]--
					l.abort(ip)
				}
			case 7:
				now = now.Add(time.Duration(op>>3&7) * 4 * time.Minute)
			}
			for k, a := range l.byIP {
				if a.pending < 0 {
					t.Fatalf("%s pending %d", k, a.pending)
				}
				if a.pending+len(prune(a.fails, now.Add(-loginWindow))) > loginMaxIP {
					t.Fatalf("%s: pending %d + fails %d above %d", k, a.pending, len(a.fails), loginMaxIP)
				}
			}
		}
	})
}
