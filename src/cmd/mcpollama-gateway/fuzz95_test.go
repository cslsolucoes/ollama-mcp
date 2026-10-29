package main

import (
	"net"
	"net/netip"
	"testing"
)

// FuzzParseNets (Fase 9.5): never panics; an accepted entry contains exactly
// the addresses its netip.Prefix meaning contains (IPv4-mapped forms read as
// IPv4), checked on the network address and on a probe address.
func FuzzParseNets(f *testing.F) {
	for _, s := range []string{"10.0.0.0/8", "::ffff:10.0.0.5", "::ffff:10.0.0.0/104", "2001:db8::/32", "10.0.0.1", "fe80::1%eth0", "::/0", "0.0.0.0/0", "::ffff:0:0/96"} {
		f.Add(s, uint32(0x0a000005))
	}
	f.Fuzz(func(t *testing.T, s string, probe uint32) {
		nets, err := parseNets(s)
		if err != nil {
			return
		}
		ref := splitList(s)
		if len(nets) != len(ref) {
			t.Fatalf("%q: %d nets for %d entries", s, len(nets), len(ref))
		}
		for i, n := range nets {
			var want netip.Prefix
			if a, err := netip.ParseAddr(ref[i]); err == nil {
				a = a.Unmap()
				want = netip.PrefixFrom(a, a.BitLen())
			} else {
				p, err := netip.ParsePrefix(ref[i])
				if err != nil {
					t.Fatalf("%q accepted but netip refuses it", ref[i])
				}
				if p.Addr().Is4In6() {
					p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
				}
				want = p.Masked()
			}
			pv4 := netip.AddrFrom4([4]byte{byte(probe >> 24), byte(probe >> 16), byte(probe >> 8), byte(probe)})
			for _, a := range []netip.Addr{want.Addr(), pv4, netip.AddrFrom16(pv4.As16())} {
				got := n.Contains(net.IP(a.AsSlice()))
				exp := want.Contains(a.Unmap())
				if got != exp {
					t.Fatalf("%q: net %v Contains(%s)=%v, want %v", ref[i], n, a, got, exp)
				}
			}
		}
	})
}
