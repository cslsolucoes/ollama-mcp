package gpu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const smiOut = `0, 00000000:03:00.0, 98.42, 7421, 87, 61
1, 00000000:04:00.0, 101.10, 7380, 85, 63
2, 00000000:05:00.0, [N/A], 7402, 0, 40
3, 00000000:06:00.0, 99.00, 7390, 91, 62
`

func TestParseCSV(t *testing.T) {
	at := time.Now()
	s, err := ParseCSV(smiOut, at)
	if err != nil || len(s) != 4 {
		t.Fatalf("%v %v", err, s)
	}
	if s[0].Index != 0 || s[0].BusID != "00000000:03:00.0" || s[0].PowerW != 98.42 || s[0].MemUsedMiB != 7421 || s[0].UtilPct != 87 || s[0].TempC != 61 {
		t.Fatalf("%+v", s[0])
	}
	if s[2].PowerW != 0 {
		t.Fatalf("[N/A] must become 0, got %v", s[2].PowerW)
	}
}

func TestParseCSVRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "not csv at all", "a, b, c", "x, 1, 2, 3, 4, 5"} {
		if _, err := ParseCSV(in, time.Now()); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestReadAER(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "aer")
	os.WriteFile(p, []byte("RxErr 1\nBadTLP 2\nBadDLLP 0\nRollover 0\nTimeout 0\nNonFatalErr 0\nCorrIntErr 0\nHeaderOF 0\nTOTAL_ERR_COR 3\n"), 0o600)
	if v, err := ReadAER(p); err != nil || v != 3 {
		t.Fatalf("%d %v", v, err)
	}
	os.WriteFile(p, []byte("RxErr 4\nBadTLP 5\n"), 0o600)
	if v, _ := ReadAER(p); v != 9 {
		t.Fatalf("sum = %d", v)
	}
	if _, err := ReadAER(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing file must fail")
	}
}

func TestCollectorWithInjectedRunner(t *testing.T) {
	dir := t.TempDir()
	aer := filepath.Join(dir, "aer")
	os.WriteFile(aer, []byte("TOTAL_ERR_COR 7\n"), 0o600)
	var got []Sample
	var gotAER int64
	c := &Collector{
		AERPath: aer, BootID: "boot-1",
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name != "nvidia-smi" || len(args) != 2 || !strings.Contains(args[0], "power.draw") {
				return nil, fmt.Errorf("unexpected command %s %v", name, args)
			}
			return []byte(smiOut), nil
		},
		OnSamples: func(ts time.Time, s []Sample, a int64) { got, gotAER = s, a },
	}
	snap := c.Once(context.Background())
	if snap.Err != "" || len(snap.Samples) != 4 || snap.AER != 7 || gotAER != 7 || len(got) != 4 || snap.BootID != "boot-1" {
		t.Fatalf("%+v", snap)
	}
	if l := c.Latest(); len(l.Samples) != 4 {
		t.Fatalf("latest %+v", l)
	}
}

func TestCollectorReportsFailures(t *testing.T) {
	called := false
	c := &Collector{
		AERPath: "-",
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exec: nvidia-smi: not found")
		},
		OnSamples: func(time.Time, []Sample, int64) { called = true },
	}
	snap := c.Once(context.Background())
	if !strings.Contains(snap.Err, "nvidia-smi") || called || len(snap.Samples) != 0 {
		t.Fatalf("%+v", snap)
	}
	c2 := &Collector{AERPath: filepath.Join(t.TempDir(), "nope"), Run: func(context.Context, string, ...string) ([]byte, error) { return []byte(smiOut), nil }}
	s2 := c2.Once(context.Background())
	if s2.AER != -1 || !strings.HasPrefix(s2.Err, "aer:") || len(s2.Samples) != 4 {
		t.Fatalf("%+v", s2)
	}
}

func TestCollectorTimeout(t *testing.T) {
	c := &Collector{
		AERPath: "-", Timeout: 30 * time.Millisecond,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	start := time.Now()
	snap := c.Once(context.Background())
	if snap.Err == "" || time.Since(start) > time.Second {
		t.Fatalf("%+v after %v", snap, time.Since(start))
	}
}

// TestHelperProcess is the fake nvidia-smi: the test binary re-executes itself.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GPU_FAKE_SMI") != "1" {
		t.Skip("helper process")
	}
	fmt.Print(smiOut)
	os.Exit(0)
}

// A real exec of a fake binary (the test binary itself).
func TestCollectorRealExecOfFakeBinary(t *testing.T) {
	t.Setenv("GPU_FAKE_SMI", "1")
	c := &Collector{Cmd: os.Args[0], Args: []string{"-test.run=^TestHelperProcess$"}, AERPath: "-"}
	snap := c.Once(context.Background())
	if snap.Err != "" || len(snap.Samples) != 4 {
		t.Fatalf("%+v", snap)
	}
}
