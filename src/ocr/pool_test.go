package ocr

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// spawnLog records the workers a pool started (after their hello).
type spawnLog struct {
	mu   sync.Mutex
	kids []*child
}

func (s *spawnLog) add(c *child) {
	s.mu.Lock()
	s.kids = append(s.kids, c)
	s.mu.Unlock()
}

func (s *spawnLog) list() []*child {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*child(nil), s.kids...)
}

// newTestPool builds a pool over the fake worker in mode.
func newTestPool(t *testing.T, mode string, edit func(*Config)) (*Pool, *spawnLog) {
	t.Helper()
	if !platformSupported {
		t.Skip("OCR pool only on Windows (D-10)")
	}
	t.Setenv("OCRTEST_MODE", mode)
	cfg := fakeConfig(t)
	if edit != nil {
		edit(&cfg)
	}
	p, err := NewPool(cfg)
	if err != nil {
		t.Fatal(err)
	}
	log := &spawnLog{}
	p.onSpawn = log.add
	t.Cleanup(func() { _ = p.Close() })
	return p, log
}

func waitExited(t *testing.T, c *child) {
	t.Helper()
	select {
	case <-c.exited:
	case <-time.After(10 * time.Second):
		t.Fatalf("worker pid %d still running", c.cmd.Process.Pid)
	}
}

func TestPoolRecognizeFake(t *testing.T) {
	p, log := newTestPool(t, "fake", nil)
	v, err := p.Probe(context.Background())
	if err != nil || v != "fake-1.0" {
		t.Fatalf("Probe = %q, %v", v, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			img := testImage(50+i, 20+i)
			page, err := p.Recognize(context.Background(), img, 0)
			if err != nil {
				errs <- err
				return
			}
			want := fmt.Sprintf("fake w=%d h=%d dpi=300 sum=%d", 50+i, 20+i, imageSum(img))
			if page.Text != want || page.Conf != 91 || page.Rotated != 90 {
				errs <- fmt.Errorf("page %d = %+v, want %q", i, page, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n := len(log.list()); n < 1 || n > 2 {
		t.Fatalf("%d workers started, want 1..2 (Workers=2)", n)
	}
	// A sub-image with an explicit dpi goes through the compacted path.
	sub := testImage(300, 40).SubImage(image.Rect(100, 10, 140, 30)).(*image.Gray)
	page, err := p.Recognize(context.Background(), sub, 150)
	if err != nil || page.Text != fmt.Sprintf("fake w=40 h=20 dpi=150 sum=%d", imageSum(sub)) {
		t.Fatalf("sub-image: %+v %v", page, err)
	}
}

func TestPoolWorkerEnvironmentIsClean(t *testing.T) {
	secrets := map[string]string{
		"OLLAMA_API_KEY":        "sk-test-secret-1",
		"OLLAMA_HOST":           "https://example.invalid",
		"MCPOLLAMA_ALLOW_ROOT":  `C:\secret-root`,
		"MY_SERVICE_TOKEN":      "tok-test-secret-2",
		"DB_PASSWORD":           "pw-test-secret-3",
		"AWS_SECRET_ACCESS_KEY": "aws-test-secret-4",
		"Github_Auth":           "gh-test-secret-5",
		"TESSDATA_PREFIX":       `C:\elsewhere`,
	}
	for k, v := range secrets {
		t.Setenv(k, v)
	}
	t.Setenv("OCRTEST_KEEP", "visible")
	p, _ := newTestPool(t, "env", nil)
	page, err := p.Recognize(context.Background(), testImage(4, 4), 0)
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(page.Text)
	for k, v := range secrets {
		if strings.Contains(upper, strings.ToUpper(k)+"=") || strings.Contains(page.Text, v) {
			t.Errorf("worker sees %s", k)
		}
	}
	if !strings.Contains(page.Text, "OCRTEST_KEEP=visible") {
		t.Error("harmless variable OCRTEST_KEEP did not reach the worker")
	}
	if !strings.Contains(upper, "SYSTEMROOT=") && platformSupported {
		t.Error("SystemRoot missing from the worker environment")
	}
	for _, a := range p.args {
		if strings.Contains(strings.ToLower(a), "api-key") || strings.Contains(a, "sk-test-secret") {
			t.Errorf("worker argument %q", a)
		}
	}
}

func TestWorkerEnvFilter(t *testing.T) {
	in := []string{"=C:=C:\\dir", "PATH=x", "SystemRoot=C:\\Windows", "ollama_api_key=s", "OLLAMA=1",
		"MCPOLLAMA_X=1", "SSH_AUTH_SOCK=s", "npm_config_password=s", "KEYRING=1", "TEMP=t"}
	got := strings.Join(workerEnv(in), "|")
	if got != `=C:=C:\dir|PATH=x|SystemRoot=C:\Windows|TEMP=t` {
		t.Fatalf("workerEnv = %s", got)
	}
}

func TestPoolPageTimeoutKillsWorker(t *testing.T) {
	p, log := newTestPool(t, "sleep", func(c *Config) { c.PageTimeout = 1500 * time.Millisecond })
	start := time.Now()
	_, err := p.Recognize(context.Background(), testImage(8, 8), 0)
	var pe *PageError
	if !errors.As(err, &pe) || !errors.Is(err, ErrPageTimeout) {
		t.Fatalf("err = %v, want *PageError ErrPageTimeout", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("timeout took %v", d)
	}
	kids := log.list()
	if len(kids) != 1 {
		t.Fatalf("%d workers", len(kids))
	}
	waitExited(t, kids[0])
	if kids[0].cmd.ProcessState.ExitCode() == 0 {
		t.Fatal("worker was not killed")
	}
}

func TestPoolContextCancelKillsWorker(t *testing.T) {
	p, log := newTestPool(t, "sleep", nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := p.Recognize(ctx, testImage(8, 8), 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	var pe *PageError
	if errors.As(err, &pe) {
		t.Fatal("context end reported as a page failure")
	}
	for _, c := range log.list() {
		waitExited(t, c)
	}
}

func TestPoolWorkerCrashRecreatedOnce(t *testing.T) {
	t.Setenv("OCRTEST_STATE", filepath.Join(t.TempDir(), "crashed"))
	p, log := newTestPool(t, "exit-once", nil)
	page, err := p.Recognize(context.Background(), testImage(5, 5), 0)
	if err != nil || !strings.HasPrefix(page.Text, "fake w=5 h=5") {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	if n := len(log.list()); n != 2 {
		t.Fatalf("%d workers started, want 2 (one crash, one retry)", n)
	}
	waitExited(t, log.list()[0])
}

func TestPoolWorkerCrashTwiceUnavailable(t *testing.T) {
	p, log := newTestPool(t, "exit", nil)
	_, err := p.Recognize(context.Background(), testImage(5, 5), 0)
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrWorkerCrashed) {
		t.Fatalf("err = %v, want ErrUnavailable+ErrWorkerCrashed", err)
	}
	var pe *PageError
	if errors.As(err, &pe) {
		t.Fatal("unavailable reported as a page failure")
	}
	if n := len(log.list()); n != 2 {
		t.Fatalf("%d workers started, want 2", n)
	}
	for _, c := range log.list() {
		waitExited(t, c)
	}
}

func TestPoolHelloFailures(t *testing.T) {
	p, _ := newTestPool(t, "init-fail", nil)
	_, err := p.Recognize(context.Background(), testImage(5, 5), 0)
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "falha simulada") {
		t.Fatalf("init failure: %v", err)
	}
	p2, _ := newTestPool(t, "hello-crash", nil)
	if _, err := p2.Probe(context.Background()); !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrWorkerCrashed) {
		t.Fatalf("crash before hello: %v", err)
	}
}

func TestPoolEngineErrorKeepsWorker(t *testing.T) {
	p, log := newTestPool(t, "engine-error", nil)
	for i := 0; i < 3; i++ {
		_, err := p.Recognize(context.Background(), testImage(5, 5), 0)
		var pe *PageError
		if !errors.As(err, &pe) || !errors.Is(err, ErrEngine) {
			t.Fatalf("err = %v", err)
		}
	}
	if n := len(log.list()); n != 1 {
		t.Fatalf("%d workers, want the same one reused", n)
	}
}

func TestPoolImageLimits(t *testing.T) {
	p, log := newTestPool(t, "fake", func(c *Config) { c.MaxMegapixels = 1 })
	big := image.NewGray(image.Rect(0, 0, 1001, 1000))
	if _, err := p.Recognize(context.Background(), big, 0); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	if _, err := p.Recognize(context.Background(), nil, 0); !errors.Is(err, ErrBadImage) {
		t.Fatalf("nil: %v", err)
	}
	if _, err := p.Recognize(context.Background(), testImage(2, 2), MaxDPI+1); !errors.Is(err, ErrBadImage) {
		t.Fatalf("dpi: %v", err)
	}
	if n := len(log.list()); n != 0 {
		t.Fatalf("a worker was started for refused images")
	}
}

func TestPoolClose(t *testing.T) {
	p, log := newTestPool(t, "fake", nil)
	if _, err := p.Recognize(context.Background(), testImage(3, 3), 0); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range log.list() {
		waitExited(t, c)
		if code := c.cmd.ProcessState.ExitCode(); code != 0 {
			t.Errorf("idle worker exit code %d, want 0 (end of input)", code)
		}
	}
	if _, err := p.Recognize(context.Background(), testImage(3, 3), 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("after Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPoolCloseWhileBusy(t *testing.T) {
	p, log := newTestPool(t, "sleep", nil)
	done := make(chan error, 1)
	go func() {
		_, err := p.Recognize(context.Background(), testImage(3, 3), 0)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for len(log.list()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	_ = p.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("busy Recognize after Close: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Recognize did not return after Close")
	}
	for _, c := range log.list() {
		waitExited(t, c)
	}
}
