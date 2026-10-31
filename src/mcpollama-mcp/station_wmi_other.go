//go:build !windows

package main

// wmiMACs has no WMI outside Windows; the MAC comes from PLACA or the interfaces.
func wmiMACs() []string { return nil }
