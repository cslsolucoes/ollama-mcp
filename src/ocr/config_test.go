package ocr

import (
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	base := fakeConfig(t)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	upper := base
	upper.LibSHA256 = strings.ToUpper(base.LibSHA256)
	if err := upper.Validate(); err != nil {
		t.Fatalf("upper-case hash: %v", err)
	}
	mut := func(f func(*Config)) Config { c := base; f(&c); return c }
	bad := map[string]Config{
		"relative lib":      mut(func(c *Config) { c.Lib = "libtesseract64.dll" }),
		"dot lib":           mut(func(c *Config) { c.Lib = `.\libtesseract64.dll` }),
		"empty lib":         mut(func(c *Config) { c.Lib = "" }),
		"unc lib":           mut(func(c *Config) { c.Lib = `\\server\share\libtesseract64.dll` }),
		"short hash":        mut(func(c *Config) { c.LibSHA256 = "abc" }),
		"non-hex hash":      mut(func(c *Config) { c.LibSHA256 = strings.Repeat("g", 64) }),
		"empty hash":        mut(func(c *Config) { c.LibSHA256 = "" }),
		"relative tessdata": mut(func(c *Config) { c.Tessdata = "tessdata" }),
		"non-ascii data":    mut(func(c *Config) { c.Tessdata = filepath.Join(c.Tessdata, "ação") }),
		"lang path":         mut(func(c *Config) { c.Lang = `..\por` }),
		"lang empty part":   mut(func(c *Config) { c.Lang = "por+" }),
		"workers":           mut(func(c *Config) { c.Workers = MaxWorkers + 1 }),
		"negative workers":  mut(func(c *Config) { c.Workers = -1 }),
		"timeout":           mut(func(c *Config) { c.PageTimeout = -1 }),
		"memory":            mut(func(c *Config) { c.MaxMem = 1 << 20 }),
		"megapixels":        mut(func(c *Config) { c.MaxMegapixels = MaxMaxMegapixels + 1 }),
		"dpi":               mut(func(c *Config) { c.DPI = MaxDPI + 1 }),
	}
	for name, c := range bad {
		if err := c.Validate(); !errors.Is(err, ErrConfig) {
			t.Errorf("%s: err = %v, want ErrConfig", name, err)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	if c.Lang != "por" || c.PageTimeout != DefaultPageTimeout || c.MaxMem != 1<<30 || c.MaxMegapixels != 40 || c.DPI != 300 {
		t.Fatalf("defaults: %+v", c)
	}
	want := runtime.NumCPU() / 2
	if want > 4 {
		want = 4
	}
	if want < 1 {
		want = 1
	}
	if c.Workers != want || DefaultWorkers() != want {
		t.Fatalf("workers = %d, want %d", c.Workers, want)
	}
}

func TestWorkerArgsRoundTrip(t *testing.T) {
	cfg := fakeConfig(t)
	cfg.Lang = "por+eng"
	cfg.DPI = 200
	cfg.MaxMegapixels = 12
	w := cfg.WorkerConfig()
	args := WorkerArgs(w)
	for _, a := range args {
		if strings.Contains(strings.ToLower(a), "api-key") {
			t.Fatalf("worker args carry %q", a)
		}
	}
	got, err := ParseWorkerArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, w) {
		t.Fatalf("round trip: %+v != %+v", got, w)
	}
	for _, bad := range [][]string{
		nil,
		args[1:], // missing WorkerFlag
		append(append([]string{}, args...), "extra"),
		append(append([]string{}, args...), "--api-key", "x"),
		{WorkerFlag, "--ocr-lib", "rel.dll", "--ocr-lib-sha256", w.LibSHA256, "--ocr-tessdata", w.Tessdata},
	} {
		if _, err := ParseWorkerArgs(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestNewPoolRefusesLibrary(t *testing.T) {
	if !platformSupported {
		if _, err := NewPool(fakeConfig(t)); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Fatalf("err = %v", err)
		}
		return
	}
	cfg := fakeConfig(t)
	rel := cfg
	rel.Lib = "libtesseract64.dll"
	if _, err := NewPool(rel); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "absoluto") {
		t.Errorf("relative path: %v", err)
	}
	wrong := cfg
	wrong.LibSHA256 = strings.Repeat("0", 64)
	if _, err := NewPool(wrong); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("hash mismatch: %v", err)
	}
	missing := cfg
	missing.Lib = filepath.Join(filepath.Dir(cfg.Lib), "absent.dll")
	if _, err := NewPool(missing); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "não encontrada") {
		t.Errorf("missing DLL: %v", err)
	}
	dir := cfg
	dir.Lib = filepath.Dir(cfg.Lib)
	if _, err := NewPool(dir); !errors.Is(err, ErrConfig) {
		t.Errorf("directory as DLL: %v", err)
	}
	noModel := cfg
	noModel.Lang = "eng"
	if _, err := NewPool(noModel); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "eng") {
		t.Errorf("missing model: %v", err)
	}
}
