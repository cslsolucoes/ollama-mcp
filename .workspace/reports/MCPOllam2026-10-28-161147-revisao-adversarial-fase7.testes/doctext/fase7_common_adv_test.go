package doctext

// Revisao adversarial da Fase 7 (testes de ataque, nao fazem parte da suite).
// Cada vetor imprime uma linha VEC|<area>/<nome>|<resultado>|<detalhe>.
// ESCAPOU e SILENCIOSO falham o teste (sao achados); o resto so registra.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"compress/flate"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func vec(t *testing.T, area, name, res, detail string) {
	t.Helper()
	line := fmt.Sprintf("VEC|%s/%s|%s|%s", area, name, res, detail)
	if res == "ESCAPOU" || res == "SILENCIOSO" {
		t.Error(line)
	} else {
		t.Log(line)
	}
}

// advDocx extracts a DOCX with the default limits.
func advDocx(data []byte) (string, error) {
	r, err := Extract(context.Background(), FormatDOCX, data, Limits{})
	if err != nil {
		return "", err
	}
	return r.Text, nil
}

func reasonStr(err error) string {
	if err == nil {
		return "ok"
	}
	if r, ok := ReasonOf(err); ok {
		return fmt.Sprintf("reason=%d", r)
	}
	return "nao-tipado:" + fmt.Sprintf("%T", err)
}

// ---- process private bytes (Windows) ----

type procMemCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

var (
	psapi               = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemInf = psapi.NewProc("GetProcessMemoryInfo")
)

// privateBytes returns the commit charge (private bytes) of this process.
func privateBytes() uint64 {
	var c procMemCounters
	c.cb = uint32(unsafe.Sizeof(c))
	h, _ := syscall.GetCurrentProcess()
	r, _, _ := procGetProcessMemInf.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	if r == 0 {
		return 0
	}
	return uint64(c.pagefileUsage)
}

// peakPrivate samples the private bytes while f runs and returns the highest
// growth above the value before.
func peakPrivate(f func()) uint64 {
	base := privateBytes()
	var stop atomic.Bool
	done := make(chan uint64)
	go func() {
		var max uint64
		for !stop.Load() {
			if v := privateBytes(); v > max {
				max = v
			}
			time.Sleep(2 * time.Millisecond)
		}
		done <- max
	}()
	f()
	stop.Store(true)
	m := <-done
	if v := privateBytes(); v > m {
		m = v
	}
	if m > base {
		return m - base
	}
	return 0
}

func mib(n uint64) string { return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20)) }

// ---- overlapping ZIP (several central-directory names, one local entry) ----

func deflateBytes(tb testing.TB, data []byte) []byte {
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.BestCompression)
	w.Write(data)
	w.Close()
	return b.Bytes()
}

type zipPart struct {
	name string
	data []byte
}

// overlapZip writes the normal parts, then ONE local entry with shared data,
// referenced by every name in shared from the central directory.
func overlapZip(tb testing.TB, normal []zipPart, shared []string, sharedData []byte) []byte {
	tb.Helper()
	var out bytes.Buffer
	type cd struct {
		name             string
		crc, csz, usz    uint32
		off              uint32
	}
	var cds []cd
	writeLocal := func(name string, data []byte) cd {
		comp := deflateBytes(tb, data)
		c := cd{name: name, crc: crc32.ChecksumIEEE(data), csz: uint32(len(comp)), usz: uint32(len(data)), off: uint32(out.Len())}
		binary.Write(&out, binary.LittleEndian, uint32(0x04034b50))
		binary.Write(&out, binary.LittleEndian, uint16(20))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(8))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(0x21))
		binary.Write(&out, binary.LittleEndian, c.crc)
		binary.Write(&out, binary.LittleEndian, c.csz)
		binary.Write(&out, binary.LittleEndian, c.usz)
		binary.Write(&out, binary.LittleEndian, uint16(len(name)))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		out.WriteString(name)
		out.Write(comp)
		return c
	}
	for _, p := range normal {
		cds = append(cds, writeLocal(p.name, p.data))
	}
	if len(shared) > 0 {
		c := writeLocal(shared[0], sharedData)
		for _, n := range shared {
			cc := c
			cc.name = n
			cds = append(cds, cc)
		}
	}
	cdStart := out.Len()
	for _, c := range cds {
		binary.Write(&out, binary.LittleEndian, uint32(0x02014b50))
		binary.Write(&out, binary.LittleEndian, uint16(20))
		binary.Write(&out, binary.LittleEndian, uint16(20))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(8))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(0x21))
		binary.Write(&out, binary.LittleEndian, c.crc)
		binary.Write(&out, binary.LittleEndian, c.csz)
		binary.Write(&out, binary.LittleEndian, c.usz)
		binary.Write(&out, binary.LittleEndian, uint16(len(c.name)))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint16(0))
		binary.Write(&out, binary.LittleEndian, uint32(0))
		binary.Write(&out, binary.LittleEndian, c.off)
		out.WriteString(c.name)
	}
	cdSize := out.Len() - cdStart
	binary.Write(&out, binary.LittleEndian, uint32(0x06054b50))
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(len(cds)))
	binary.Write(&out, binary.LittleEndian, uint16(len(cds)))
	binary.Write(&out, binary.LittleEndian, uint32(cdSize))
	binary.Write(&out, binary.LittleEndian, uint32(cdStart))
	binary.Write(&out, binary.LittleEndian, uint16(0))
	return out.Bytes()
}

func count(s, sub string) int { return strings.Count(s, sub) }
