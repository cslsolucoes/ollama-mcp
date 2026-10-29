package main

// Offline reading of the server monitor (srv/mon.sh output, one "P" line per
// second with index,power,temp,mem,util of each GPU; one "S" line every 10 s
// with boot, Xid and the AER counters of 00:1b.4; "PS" lines with ollama ps).
// Used by phase 8.9 to get the power peak, the AER delta and the VRAM of a
// load window: bakeoff -suites monpeak -mon-file F -from HH:MM:SS -to HH:MM:SS

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"
)

type monStats struct {
	Samples           int
	PeakSumW          float64
	PeakSumAt         string
	PeakGPUW          map[string]float64
	MaxMemMiB         map[string]int
	MaxMemSumMiB      int
	AERFirst, AERLast string
	NonFatalOrFatal   bool
	Xid               map[string]bool
	Boots             map[string]bool
	PS                map[string]bool
	Alerts            int
}

// inWindow compares HH:MM:SS strings; a window with from > to wraps midnight.
func inWindow(t, from, to string) bool {
	if from <= to {
		return t >= from && t <= to
	}
	return t >= from || t <= to
}

func kvField(fields []string, key string) string {
	for _, f := range fields {
		if v, ok := strings.CutPrefix(f, key+"="); ok {
			return v
		}
	}
	return ""
}

// parseMon reads the monitor lines inside [from, to].
func parseMon(lines []string, from, to string) monStats {
	s := monStats{PeakGPUW: map[string]float64{}, MaxMemMiB: map[string]int{}, Xid: map[string]bool{}, Boots: map[string]bool{}, PS: map[string]bool{}}
	for _, l := range lines {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "P":
			if !inWindow(f[1], from, to) {
				continue
			}
			s.Samples++
			sum, mem := 0.0, 0
			for _, g := range f[2:] {
				p := strings.Split(g, ",")
				if len(p) < 4 {
					continue
				}
				w, _ := strconv.ParseFloat(p[1], 64)
				m, _ := strconv.Atoi(p[3])
				sum += w
				mem += m
				if w > s.PeakGPUW[p[0]] {
					s.PeakGPUW[p[0]] = w
				}
				if m > s.MaxMemMiB[p[0]] {
					s.MaxMemMiB[p[0]] = m
				}
			}
			if sum > s.PeakSumW {
				s.PeakSumW, s.PeakSumAt = sum, f[1]
			}
			if mem > s.MaxMemSumMiB {
				s.MaxMemSumMiB = mem
			}
		case "S":
			if !inWindow(f[1], from, to) {
				continue
			}
			corr := kvField(f, "corr")
			if s.AERFirst == "" {
				s.AERFirst = corr
			}
			s.AERLast = corr
			if kvField(f, "nonfatal") != "0" || kvField(f, "fatal") != "0" {
				s.NonFatalOrFatal = true
			}
			s.Xid[kvField(f, "xid")] = true
			if len(f) >= 4 {
				s.Boots[strings.TrimPrefix(f[2], "boot=")+" "+f[3]] = true
			}
		case "PS":
			if !inWindow(f[1], from, to) || len(f) < 3 {
				continue
			}
			proc := ""
			if i := strings.Index(l, "% GPU"); i >= 0 {
				j := strings.LastIndex(l[:i], " ")
				proc = strings.TrimSpace(l[j:i]) + "% GPU"
			} else if strings.Contains(l, "CPU/GPU") {
				proc = "CPU/GPU"
			}
			s.PS[f[2]+" "+proc] = true
		case "ALERTA":
			if inWindow(f[1], from, to) {
				s.Alerts++
			}
		}
	}
	return s
}

func runMonPeak(o options, logf func(string, ...any)) int {
	fh, err := os.Open(o.monFile)
	if err != nil {
		logf("FATAL mon-file: %v", err)
		return 2
	}
	defer fh.Close()
	var lines []string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	s := parseMon(lines, o.from, o.to)
	logf("MONPEAK janela=%s-%s amostras=%d pico_soma_W=%.0f em=%s pico_por_gpu_W=%v vram_max_por_gpu_MiB=%v vram_max_soma_MiB=%d aer_1b4=%s->%s nao_corrigivel=%v xid=%v boots=%v ps=%v alertas=%d",
		o.from, o.to, s.Samples, s.PeakSumW, s.PeakSumAt, s.PeakGPUW, s.MaxMemMiB, s.MaxMemSumMiB, s.AERFirst, s.AERLast, s.NonFatalOrFatal, keys(s.Xid), keys(s.Boots), keys(s.PS), s.Alerts)
	return 0
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
