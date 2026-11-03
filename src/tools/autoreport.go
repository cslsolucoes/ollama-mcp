package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cslsoftwares/mcpollama/ollama"
)

// Automatic candidates (Fase 12.5; D12-8, contract C-1.19 and C-1.20). After the
// answer of generate, chat, summarize_file, extract or translate, ONLY three
// objective rules are checked:
//
//	(a) the request declared JSON in `format` ("json" or a schema) and the text
//	    fails json.Valid;
//	(b) the answer is only white space (or empty);
//	(c) fim=length and num_predict is absent or >= 256.
//
// A hit is queued and sent in the background with origem=automatico, no
// resposta_correta and a fixed short description: it never delays or changes
// the answer of the tool and never fails it. No job id (no gateway) means
// nothing is sent. The tools do not expose the Ollama `format` option today, so
// rule (a) has no caller yet; it is implemented and tested for when one exists.

const (
	autoQueueSize     = 16
	autoSendTimeout   = 30 * time.Second
	autoMaxRetries    = 3 // retries after a 409, on top of the first attempt
	autoProject       = "mcpollama-auto"
	autoSeverity      = "baixa"
	autoNumPredictMin = 256
)

// autoInput is what the rules look at: the output of the call that produced the
// answer, never more.
type autoInput struct {
	Text        string
	DoneReason  string
	NumPredict  *int // nil when the request did not set num_predict
	JSONFormat  bool // the request declared format = "json" or a schema
	Jobs        []jobRef
	ErrJobIndex int // the call that produced Text
}

// autoVerdict is the report built by a rule: category and fixed description.
type autoVerdict struct{ categoria, descricao string }

// autoRules applies the three rules, in the order (b) empty, (a) invalid JSON,
// (c) truncated, and returns the first that hits.
func autoRules(in autoInput) (autoVerdict, bool) {
	if strings.TrimSpace(in.Text) == "" {
		return autoVerdict{"outro", "resposta vazia"}, true
	}
	if in.JSONFormat && !json.Valid([]byte(in.Text)) {
		return autoVerdict{"formato_invalido", "JSON inválido: o pedido declarou format JSON e a resposta não passa em json.Valid"}, true
	}
	if in.DoneReason == "length" && (in.NumPredict == nil || *in.NumPredict >= autoNumPredictMin) {
		return autoVerdict{"truncado", "resposta truncada: fim=length"}, true
	}
	return autoVerdict{}, false
}

type autoItem struct {
	rep     ollama.FeedbackReport
	backend string
}

// AutoReporter sends the automatic candidates from one goroutine. It is created
// by the server when --auto-report is on; a nil *AutoReporter is off.
type AutoReporter struct {
	backend func() (Backend, error)
	logf    func(string, ...any)
	timeout time.Duration
	// wait is the pause before retry n (1-based) after a 409: Retry-After, or a
	// growing backoff without it, never above 10 s. Tests replace it.
	wait func(ra time.Duration, attempt int) time.Duration

	queue   chan autoItem
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	closing sync.Once

	dropped atomic.Int64
	sent    atomic.Int64
	failed  atomic.Int64 // reports given up on (error, refusal or no client)
}

// NewAutoReporter starts the sender. logf receives one debug line per drop or
// failure (no content); nil is fine. Close stops it.
func NewAutoReporter(backend func() (Backend, error), logf func(string, ...any)) *AutoReporter {
	return newAutoReporter(backend, logf, autoQueueSize, autoSendTimeout)
}

func newAutoReporter(backend func() (Backend, error), logf func(string, ...any), size int, timeout time.Duration) *AutoReporter {
	ctx, cancel := context.WithCancel(context.Background())
	r := &AutoReporter{
		backend: backend, logf: logf, timeout: timeout,
		wait: func(ra time.Duration, attempt int) time.Duration {
			if ra <= 0 {
				ra = time.Duration(attempt) * 2 * time.Second
			}
			return min(ra, maxFeedbackWait)
		},
		queue: make(chan autoItem, size), ctx: ctx, cancel: cancel, done: make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *AutoReporter) debugf(format string, a ...any) {
	if r != nil && r.logf != nil {
		r.logf(format, a...)
	}
}

// Close stops the sender: the report in flight is cancelled, the queued ones are
// dropped, and the goroutine ends before Close returns (bounded by 2 s).
func (r *AutoReporter) Close() {
	if r == nil {
		return
	}
	r.closing.Do(func() {
		r.cancel()
		select {
		case <-r.done:
		case <-time.After(2 * time.Second):
		}
	})
}

// Done is closed when the sender goroutine has ended (after Close).
func (r *AutoReporter) Done() <-chan struct{} { return r.done }

// submit queues a report without ever blocking; a full queue drops it.
func (r *AutoReporter) submit(it autoItem) {
	if r == nil || r.ctx.Err() != nil {
		return
	}
	select {
	case r.queue <- it:
	default:
		r.dropped.Add(1)
		r.debugf("auto-report: fila cheia (%d), candidato descartado", cap(r.queue))
	}
}

func (r *AutoReporter) run() {
	defer close(r.done)
	for {
		select {
		case <-r.ctx.Done():
			return
		case it := <-r.queue:
			r.send(it)
		}
	}
}

func (r *AutoReporter) send(it autoItem) {
	defer func() {
		if p := recover(); p != nil { // never let a bug here kill the server
			r.failed.Add(1)
			r.debugf("auto-report: erro interno ao enviar: %v", p)
		}
	}()
	be, err := r.backend()
	if err != nil {
		r.failed.Add(1)
		r.debugf("auto-report: sem cliente: %v", err)
		return
	}
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(r.ctx, r.timeout)
		res, err := be.Feedback(ctx, it.rep, it.backend)
		cancel()
		if err == nil {
			r.sent.Add(1)
			r.debugf("auto-report: ocorrência %s (%s)", res.OcorrenciaID, res.Estado)
			return
		}
		var fe *ollama.FeedbackError
		if !errors.As(err, &fe) || fe.Status() != 409 || attempt >= autoMaxRetries {
			r.failed.Add(1)
			r.debugf("auto-report: não enviado: %v", err)
			return
		}
		t := time.NewTimer(r.wait(fe.RetryAfter, attempt+1))
		select {
		case <-t.C:
		case <-r.ctx.Done():
			t.Stop()
			return
		}
	}
}

// autoCheck runs the rules for one finished answer and queues the candidate.
// Cheap and non-blocking: it is called on the path of the tool's answer.
func (d Deps) autoCheck(in autoInput) {
	if d.Auto == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil {
			d.Auto.debugf("auto-report: erro interno na checagem: %v", p)
		}
	}()
	v, hit := autoRules(in)
	if !hit {
		return
	}
	if in.ErrJobIndex < 0 || in.ErrJobIndex >= len(in.Jobs) || in.Jobs[in.ErrJobIndex].ID == "" {
		return // no gateway job id: nothing to point at
	}
	errJob := in.Jobs[in.ErrJobIndex]
	var jobs []ollama.FeedbackJob
	for _, j := range in.Jobs {
		if j.ID != "" {
			jobs = append(jobs, ollama.FeedbackJob{ID: j.ID, Papel: j.Role, Backend: j.Backend})
		}
	}
	if len(jobs) > maxReportJobs {
		d.Auto.debugf("auto-report: %d chamadas, acima de %d; não relatado", len(jobs), maxReportJobs)
		return
	}
	d.Auto.submit(autoItem{
		rep: ollama.FeedbackReport{
			Projeto: autoProject, Jobs: jobs, JobDoErro: errJob.ID, Categoria: v.categoria, Descricao: v.descricao,
			Severidade: autoSeverity, Origem: "automatico",
		},
		backend: errJob.Backend,
	})
}

// String lets a debug line name the counters without a getter per field.
func (r *AutoReporter) String() string {
	return fmt.Sprintf("auto-report enviados=%d descartados=%d", r.sent.Load(), r.dropped.Load())
}
