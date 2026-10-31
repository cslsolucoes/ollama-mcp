//go:build windows

package main

import (
	"os"
	"testing"
)

// Calls the real WMI of this machine. Skipped when WMI is unavailable.
func TestWMIRealMACs(t *testing.T) {
	macs := wmiMACs()
	if len(macs) == 0 {
		t.Skip("WMI returned nothing (unavailable or timed out)")
	}
	got := pickMAC(macs)
	if got == "" || len(got) != 12 {
		t.Fatalf("WMI answered %q but no valid 12-hex MAC", macs)
	}
	t.Logf("WMI physical adapters: %q -> first valid %s", macs, got)
	if placa := normalizeMAC(os.Getenv("PLACA")); placa != "" {
		t.Logf("PLACA=%s, WMI first=%s, equal=%v", placa, got, placa == got)
	}
}
