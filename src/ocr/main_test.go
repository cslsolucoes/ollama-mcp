package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the worker executable: the pool starts
// os.Executable() with WorkerFlag, and TestMain turns that run into a worker
// before the testing flags are parsed. OCRTEST_MODE (a name the environment
// filter keeps) picks the behavior of the fake engine.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == WorkerFlag {
		os.Exit(testWorkerMain(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func testWorkerMain(args []string) int {
	cfg, err := ParseWorkerArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	mode := os.Getenv("OCRTEST_MODE")
	switch mode {
	case "noise-unprotected":
		emitNoise()
		return 0
	case "noise-protected":
		if _, _, err := WorkerStdio(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		emitNoise()
		return 0
	}
	if mode == "real" || mode == "real-noisy" {
		return realWorkerMain(cfg, mode == "real-noisy")
	}
	open := func(WorkerConfig) (workerEngine, string, error) {
		switch mode {
		case "init-fail":
			return nil, "", fmt.Errorf("%w: falha simulada", ErrConfig)
		case "hello-crash":
			os.Exit(7)
		}
		return &fakeEngine{mode: mode}, "fake-1.0", nil
	}
	if err := runWorker(os.Stdin, os.Stdout, cfg, open); err != nil {
		return 1
	}
	return 0
}

// fakeEngine answers deterministic pages; some modes misbehave on purpose.
type fakeEngine struct {
	mode string
}

func (f *fakeEngine) Recognize(_ context.Context, img *image.Gray, dpi int) (Page, error) {
	switch f.mode {
	case "sleep":
		time.Sleep(time.Hour)
	case "exit":
		os.Exit(3)
	case "exit-once":
		marker := os.Getenv("OCRTEST_STATE")
		if _, err := os.Stat(marker); err != nil {
			_ = os.WriteFile(marker, []byte("1"), 0o600)
			os.Exit(3)
		}
	case "env":
		env := os.Environ()
		sort.Strings(env)
		return Page{Text: strings.Join(env, "\n"), Conf: 100}, nil
	case "go-alloc":
		var keep [][]byte
		for i := 0; i < 8; i++ {
			b := make([]byte, 64<<20)
			for j := 0; j < len(b); j += 4096 {
				b[j] = 1
			}
			keep = append(keep, b)
		}
		_ = keep
	case "engine-error":
		return Page{}, fmt.Errorf("%w: simulada", ErrEngine)
	case "alloc":
		// Commit 512 MiB outside the Go heap: past the Job Object ceiling
		// the commit is refused (a Go heap allocation would abort the
		// runtime instead, see TestJobMemoryCeiling).
		if err := commitMemory(512 << 20); err != nil {
			return Page{}, fmt.Errorf("%w: %v", ErrEngine, err)
		}
	}
	var sum int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sum += int(img.GrayAt(x, y).Y)
		}
	}
	return Page{Text: fmt.Sprintf("fake w=%d h=%d dpi=%d sum=%d", b.Dx(), b.Dy(), dpi, sum), Conf: 91, Rotated: 90}, nil
}

func (f *fakeEngine) Close() error { return nil }

// fakeLib creates a stand-in DLL and tessdata directory and returns a pool
// configuration that passes NewPool's checks (the fake worker never loads
// the file).
func fakeConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	lib := filepath.Join(dir, "libfake.dll")
	content := []byte("not a real dll")
	if err := os.WriteFile(lib, content, 0o600); err != nil {
		t.Fatal(err)
	}
	td := filepath.Join(dir, "tessdata")
	if err := os.Mkdir(td, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "por.traineddata"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return Config{
		Lib:         lib,
		LibSHA256:   hex.EncodeToString(sum[:]),
		Tessdata:    td,
		Workers:     2,
		PageTimeout: 20 * time.Second,
	}
}

func testImage(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	return img
}

func imageSum(img *image.Gray) int {
	var sum int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			sum += int(img.GrayAt(x, y).Y)
		}
	}
	return sum
}

// realWorkerMain is the worker as the server will run it in 7.9b:
// WorkerStdio first, then RunWorker with the real engine. In the noisy
// variant it also prints to every standard output it can reach after the
// redirection, the way a chatty native library would.
func realWorkerMain(cfg WorkerConfig, noisy bool) int {
	in, out, err := WorkerStdio()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if noisy {
		emitNoise()
	}
	if err := RunWorker(in, out, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
