package main

import (
	"net"
	"os"
	"os/user"
	"strings"
)

// Station identification (mcpollama 0.9.2). The workstation declares who is
// asking in the X-Mcpollama-Station header; the server stores it in the job
// log. It is declared by the client and is NOT authenticated.
//
// Origin of the values on CSL domain machines: the AD logon script
// (LoginScriptV4.00.vbs, v4.10) writes them. PLACA is the MAC of the FIRST
// adapter of Win32_NetworkAdapter where PhysicalAdapter=True with a non-null
// MACAddress, without the ':' separators, stored in the USER environment
// (HKCU\Environment). USERDOMAIN, USERNAME and COMPUTERNAME carry the same
// values as WScript.Network.UserDomain/UserName/ComputerName.
//
// The exe asks WMI itself (pure Go over syscall, Windows only, 3 s limit) with
// the same query and order as the script. Precedence of the mac field: WMI,
// then PLACA, then the active interface with a private IPv4 address (machine
// outside the domain, no script, WMI failing), which is not necessarily the
// "first physical adapter" of WMI.

// stationMaxValue is the ceiling of each station value, in characters.
const stationMaxValue = 64

// stationInfo is what this workstation declares about itself.
type stationInfo struct {
	Computer string
	User     string
	Domain   string
	MAC      string // 12 upper-case hex digits, no separators
}

// header renders the X-Mcpollama-Station value:
// computer=<v>;user=<v>;domain=<v>;mac=<v>, with empty fields omitted.
func (s stationInfo) header() string {
	var parts []string
	for _, f := range []struct{ k, v string }{
		{"computer", s.Computer}, {"user", s.User}, {"domain", s.Domain}, {"mac", s.MAC},
	} {
		if f.v != "" {
			parts = append(parts, f.k+"="+f.v)
		}
	}
	return strings.Join(parts, ";")
}

// stationValue keeps [A-Za-z0-9._-], turns anything else into '_' and cuts at
// stationMaxValue characters.
func stationValue(v string) string {
	v = strings.TrimSpace(v)
	out := make([]rune, 0, len(v))
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			r = '_'
		}
		out = append(out, r)
		if len(out) == stationMaxValue {
			break
		}
	}
	return string(out)
}

// normalizeMAC returns the 12 upper-case hex digits of s after dropping '-' and
// ':', or "" when s is not exactly 12 hex digits.
func normalizeMAC(s string) string {
	s = strings.NewReplacer("-", "", ":", "").Replace(strings.TrimSpace(s))
	if len(s) != 12 {
		return ""
	}
	s = strings.ToUpper(s)
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return ""
		}
	}
	return s
}

// ifaceInfo is the part of a network interface the MAC fallback needs.
type ifaceInfo struct {
	Up, Loopback bool
	HardwareAddr net.HardwareAddr
	Addrs        []net.Addr
}

// privateIPv4 reports whether any address is a private IPv4 one.
func privateIPv4(addrs []net.Addr) bool {
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil && ip4.IsPrivate() {
			return true
		}
	}
	return false
}

// interfaceMAC is the MAC of the first interface (system order) that is up, is
// not loopback, has a 6-byte hardware address and a private IPv4 address.
func interfaceMAC(ifs []ifaceInfo) string {
	for _, i := range ifs {
		if !i.Up || i.Loopback || len(i.HardwareAddr) != 6 || !privateIPv4(i.Addrs) {
			continue
		}
		return normalizeMAC(i.HardwareAddr.String())
	}
	return ""
}

// systemInterfaces lists the interfaces of this machine; an error yields none.
func systemInterfaces() []ifaceInfo {
	list, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := make([]ifaceInfo, 0, len(list))
	for _, i := range list {
		addrs, _ := i.Addrs()
		out = append(out, ifaceInfo{
			Up:           i.Flags&net.FlagUp != 0,
			Loopback:     i.Flags&net.FlagLoopback != 0,
			HardwareAddr: i.HardwareAddr,
			Addrs:        addrs,
		})
	}
	return out
}

// buildStation assembles the station from an injectable environment, host name,
// the user.Current() name (possibly DOMAIN\user) and interface list.
func buildStation(getenv func(string) string, hostname, currentName string, wmi []string, ifs []ifaceInfo) stationInfo {
	var s stationInfo

	computer := getenv("COMPUTERNAME")
	if strings.TrimSpace(computer) == "" {
		computer = hostname
	}
	s.Computer = stationValue(computer)

	usr, dom := currentName, ""
	if i := strings.LastIndexAny(usr, `\/`); i >= 0 {
		dom, usr = usr[:i], usr[i+1:]
	}
	s.User = stationValue(usr)

	if d := getenv("USERDOMAIN"); strings.TrimSpace(d) != "" {
		dom = d
	}
	s.Domain = stationValue(dom)

	// MAC precedence: WMI (same rule as the logon script), then PLACA, then the
	// active interface.
	if s.MAC = pickMAC(wmi); s.MAC == "" {
		if s.MAC = normalizeMAC(getenv("PLACA")); s.MAC == "" {
			s.MAC = interfaceMAC(ifs)
		}
	}
	return s
}

// pickMAC is the first value of the WMI answer that is a valid MAC (12 hex
// after dropping ':' and '-'), upper-cased; null or malformed entries are skipped.
func pickMAC(values []string) string {
	for _, v := range values {
		if m := normalizeMAC(v); m != "" {
			return m
		}
	}
	return ""
}

// collectStationHeader reads the real machine once and returns the ready
// header text (empty when nothing is known).
func collectStationHeader(getenv func(string) string) string {
	host, _ := os.Hostname()
	return buildStation(getenv, host, currentUserFull(), wmiMACs(), systemInterfaces()).header()
}

// currentUserFull is user.Current().Username (DOMAIN\user on Windows), falling
// back to the USERNAME/USER variables.
func currentUserFull() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
