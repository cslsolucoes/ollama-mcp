package ocr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Pool runs the OCR in worker processes ("<exe> --ocr-worker ...", the same
// executable) started on demand, up to Config.Workers at a time. It
// implements Engine, so a caller does not know whether the text came from a
// child process or from an in-process engine.
//
// Per page: the request goes to an idle worker (or a new one); if the page
// takes longer than Config.PageTimeout the worker is killed and the page
// fails with ErrPageTimeout (*PageError); if the worker dies, a new one is
// started and the page is sent once more; a second death in the same call
// answers ErrUnavailable. The server process stays alive in every case.
type Pool struct {
	cfg  Config
	wcfg WorkerConfig
	exe  string
	args []string
	env  []string
	job  *jobObject
	sem  chan struct{}

	mu      sync.Mutex
	idle    []*child
	live    map[*child]struct{}
	closed  bool
	version string

	onSpawn func(*child) // test hook, called after a worker answered its hello
}

// child is one worker process and the parent's ends of its pipes.
type child struct {
	cmd      *exec.Cmd
	in       *os.File // write end of the worker's stdin
	out      *os.File // read end of the worker's stdout
	rd       *bufio.Reader
	stderr   *capBuffer
	exited   chan struct{} // closed once the process was reaped
	killOnce sync.Once
}

// stderrCap bounds the stderr kept per worker (diagnosis only).
var stderrCap = 4 << 10

// errTimedOut is internal: the page (or the hello) took too long.
var errTimedOut = errors.New("ocr: tempo esgotado")

// NewPool validates cfg, checks the DLL hash and the language models and
// prepares the pool. No worker starts before the first Recognize or Probe.
func NewPool(cfg Config) (*Pool, error) {
	if !platformSupported {
		return nil, ErrUnsupportedPlatform
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	wcfg := cfg.WorkerConfig()
	if err := checkLibFile(wcfg.Lib, wcfg.LibSHA256); err != nil {
		return nil, err
	}
	if err := checkModels(wcfg); err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("%w: executável do servidor não localizado", ErrUnavailable)
	}
	job, err := newJob(cfg.MaxMem)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return &Pool{
		cfg:  cfg,
		wcfg: wcfg,
		exe:  exe,
		args: WorkerArgs(wcfg),
		env:  workerEnv(os.Environ()),
		job:  job,
		sem:  make(chan struct{}, cfg.Workers),
		live: make(map[*child]struct{}),
	}, nil
}

// Version is the engine version reported by the first worker ("" before).
func (p *Pool) Version() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.version
}

// Probe starts a worker (or reuses an idle one) and returns the engine
// version: it proves the DLL loads and the language initializes, without a
// page. The worker stays idle in the pool.
func (p *Pool) Probe(ctx context.Context) (string, error) {
	if err := p.acquire(ctx); err != nil {
		return "", err
	}
	defer p.release()
	c, err := p.get(ctx)
	if errors.Is(err, ErrWorkerCrashed) {
		c, err = p.get(ctx)
		if errors.Is(err, ErrWorkerCrashed) {
			return "", fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
	}
	if err != nil {
		return "", err
	}
	p.put(c)
	return p.Version(), nil
}

// Recognize implements Engine.
func (p *Pool) Recognize(ctx context.Context, img *image.Gray, dpi int) (Page, error) {
	if dpi == 0 {
		dpi = p.cfg.DPI
	}
	if dpi < MinDPI || dpi > MaxDPI {
		return Page{}, &PageError{Err: ErrBadImage}
	}
	if err := checkImage(img, p.wcfg.maxPixels()); err != nil {
		return Page{}, &PageError{Err: err}
	}
	if err := p.acquire(ctx); err != nil {
		return Page{}, err
	}
	defer p.release()
	crashes := 0
	for {
		c, err := p.get(ctx)
		if errors.Is(err, ErrWorkerCrashed) {
			if crashes++; crashes >= 2 {
				return Page{}, fmt.Errorf("%w: %w", ErrUnavailable, ErrWorkerCrashed)
			}
			continue
		}
		if err != nil {
			return Page{}, err
		}
		resp, err := c.roundTrip(ctx, p.cfg.PageTimeout, func(w io.Writer) error { return writeRequest(w, img, dpi) })
		switch {
		case err == nil && resp.kind == kindResult:
			switch resp.status {
			case statusOK:
				p.put(c)
				return Page{Text: resp.text, Conf: resp.conf, Rotated: resp.rotated}, nil
			case statusEngine:
				p.put(c)
				return Page{}, &PageError{Err: fmt.Errorf("%w: %s", ErrEngine, resp.text)}
			case statusTooLarge:
				p.put(c)
				return Page{}, &PageError{Err: ErrTextTooLarge}
			case statusBadImage:
				p.put(c)
				return Page{}, &PageError{Err: ErrBadImage}
			default: // statusBadFrame: the worker stopped reading
				p.discard(c)
				return Page{}, &PageError{Err: fmt.Errorf("%w: %s", errFrame, resp.text)}
			}
		case errors.Is(err, errTimedOut):
			p.discard(c)
			return Page{}, &PageError{Err: ErrPageTimeout}
		case ctx.Err() != nil:
			p.discard(c)
			return Page{}, ctx.Err()
		default: // the worker died or answered garbage
			p.discard(c)
			if p.isClosed() {
				return Page{}, ErrClosed
			}
			if crashes++; crashes >= 2 {
				return Page{}, fmt.Errorf("%w: %w", ErrUnavailable, ErrWorkerCrashed)
			}
		}
	}
}

// Close stops every worker: idle ones get end-of-input and exit on their
// own (killed after 2 s), busy ones are killed. Closing the job then ends
// anything left. Recognize answers ErrClosed afterwards.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	idle := p.idle
	p.idle = nil
	isIdle := make(map[*child]bool, len(idle))
	for _, c := range idle {
		isIdle[c] = true
	}
	var busy []*child
	for c := range p.live {
		if !isIdle[c] {
			busy = append(busy, c)
		}
	}
	for _, c := range idle {
		delete(p.live, c)
	}
	p.mu.Unlock()

	for _, c := range busy {
		c.kill() // their Recognize discards them
	}
	for _, c := range idle {
		_ = c.in.Close()
	}
	for _, c := range idle {
		select {
		case <-c.exited:
		case <-time.After(2 * time.Second):
			c.kill()
		}
		c.release()
	}
	p.job.close()
	return nil
}

func (p *Pool) acquire(ctx context.Context) error {
	if p.isClosed() {
		return ErrClosed
	}
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) release() { <-p.sem }

func (p *Pool) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// get returns an idle live worker or starts a new one.
func (p *Pool) get(ctx context.Context) (*child, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrClosed
		}
		n := len(p.idle)
		if n == 0 {
			p.mu.Unlock()
			return p.spawn(ctx)
		}
		c := p.idle[n-1]
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		select {
		case <-c.exited: // died while idle (memory ceiling, external kill)
			p.discard(c)
		default:
			return c, nil
		}
	}
}

// put returns a healthy worker to the idle list.
func (p *Pool) put(c *child) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.discard(c)
		return
	}
	p.idle = append(p.idle, c)
	p.mu.Unlock()
}

// discard kills a worker that is no longer trusted and releases its pipes.
func (p *Pool) discard(c *child) {
	c.kill()
	select {
	case <-c.exited:
	case <-time.After(5 * time.Second):
	}
	c.release()
	p.mu.Lock()
	delete(p.live, c)
	p.mu.Unlock()
}

// spawn starts a worker, puts it in the job before it runs, and waits for
// its hello.
func (p *Pool) spawn(ctx context.Context) (*child, error) {
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	cmd := exec.Command(p.exe, p.args...)
	cmd.Env = p.env
	cmd.Dir = p.wcfg.Tessdata
	cmd.Stdin = inR
	cmd.Stdout = outW
	eb := &capBuffer{max: stderrCap}
	cmd.Stderr = eb
	p.job.prepare(cmd)
	err = cmd.Start()
	inR.Close()
	outW.Close()
	if err != nil {
		inW.Close()
		outR.Close()
		return nil, fmt.Errorf("%w: não foi possível iniciar o worker de OCR: %v", ErrUnavailable, err)
	}
	c := &child{cmd: cmd, in: inW, out: outR, rd: bufio.NewReaderSize(outR, 64<<10), stderr: eb, exited: make(chan struct{})}
	if err := p.job.adopt(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		c.release()
		return nil, fmt.Errorf("%w: worker de OCR fora do Job Object: %v", ErrUnavailable, err)
	}
	go func() {
		_ = cmd.Wait()
		close(c.exited)
	}()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.discard(c)
		return nil, ErrClosed
	}
	p.live[c] = struct{}{}
	p.mu.Unlock()

	resp, err := c.roundTrip(ctx, p.cfg.PageTimeout, nil)
	switch {
	case err == nil && resp.kind == kindHello && resp.status == statusOK:
		p.mu.Lock()
		p.version = resp.text
		hook := p.onSpawn
		p.mu.Unlock()
		if hook != nil {
			hook(c)
		}
		return c, nil
	case err == nil && resp.kind == kindHello:
		p.discard(c)
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, resp.text)
	case errors.Is(err, errTimedOut):
		p.discard(c)
		return nil, fmt.Errorf("%w: o worker de OCR não respondeu a tempo", ErrUnavailable)
	case ctx.Err() != nil:
		p.discard(c)
		return nil, ctx.Err()
	default:
		p.discard(c)
		return nil, ErrWorkerCrashed
	}
}

// roundTrip optionally sends a request and reads one response, giving up
// after timeout or when ctx ends; in both cases the worker is killed (a
// native call cannot be interrupted any other way) before returning.
func (c *child) roundTrip(ctx context.Context, timeout time.Duration, send func(io.Writer) error) (frameResponse, error) {
	type result struct {
		resp frameResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		if send != nil {
			if err := send(c.in); err != nil {
				ch <- result{err: err}
				return
			}
		}
		resp, err := readResponse(c.rd)
		ch <- result{resp: resp, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-timer.C:
		c.kill()
		<-ch
		return frameResponse{}, errTimedOut
	case <-ctx.Done():
		c.kill()
		<-ch
		return frameResponse{}, ctx.Err()
	}
}

func (c *child) kill() {
	c.killOnce.Do(func() { _ = c.cmd.Process.Kill() })
}

// release closes the parent's pipe ends; only called when no goroutine uses
// them any more.
func (c *child) release() {
	_ = c.in.Close()
	_ = c.out.Close()
}

// capBuffer keeps the first max bytes written (the worker's stderr, for
// diagnosis only; never copied to the server's stdout).
type capBuffer struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.max - len(c.b); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		c.b = append(c.b, p[:room]...)
	}
	return len(p), nil
}

func (c *capBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.b)
}

// workerEnv is the parent's environment without anything that may hold a
// credential or configure the server: OLLAMA_*, MCPOLLAMA_*, any name with
// KEY, TOKEN, SECRET, PASS, CREDENTIAL or AUTH, and TESSDATA_PREFIX (the
// worker gets its model directory explicitly).
func workerEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if name == "" {
			// Windows per-drive entries ("=C:=C:\dir") start with "=".
			out = append(out, kv)
			continue
		}
		if sensitiveEnvName(name) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func sensitiveEnvName(name string) bool {
	u := strings.ToUpper(name)
	if strings.HasPrefix(u, "OLLAMA") || strings.HasPrefix(u, "MCPOLLAMA") || u == "TESSDATA_PREFIX" {
		return true
	}
	for _, w := range []string{"KEY", "TOKEN", "SECRET", "PASS", "CREDENTIAL", "AUTH"} {
		if strings.Contains(u, w) {
			return true
		}
	}
	return false
}
