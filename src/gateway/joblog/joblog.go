// Package joblog wires the durable job log (Fase 10.L): the metadata rows in the
// store, the full text in the content database, and the per-second GPU extract
// of the running jobs.
package joblog

import (
	"context"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/content"
	"github.com/cslsoftwares/mcpollama/gateway/cpu"
	"github.com/cslsoftwares/mcpollama/gateway/gpu"
	"github.com/cslsoftwares/mcpollama/gateway/proxy"
	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// Journal implements proxy.Journal.
type Journal struct {
	Store   *store.Store
	Content *content.Store // nil: the text is not recorded (--log-content=false)
	Backend string         // name stored at the start; default proxy.DefaultBackend
	// Running lists the ids of the jobs holding a slot (queue.Manager.RunningIDs).
	Running func() []string
	Logf    func(format string, args ...any)
}

var _ proxy.Journal = (*Journal)(nil)

func (j *Journal) logf(format string, a ...any) {
	if j.Logf != nil {
		j.Logf(format, a...)
	}
}

// Enqueued writes the entry of the job and its prompt, durably. It keeps no
// prompt_json: the proxy uses EnqueuedJSON, which writes the two together.
func (j *Journal) Enqueued(rec queue.Record, prompt string) {
	j.enqueue(rec, prompt, proxy.PromptJSON{}, false)
}

var _ proxy.JSONJournal = (*Journal)(nil)

// WantsPromptJSON is false when the content is not recorded: the proxy then
// builds neither the flat text nor the prompt JSON.
func (j *Journal) WantsPromptJSON() bool { return j.Content != nil }

// EnqueuedJSON writes the entry of the job, the flat prompt and the canonical
// request (prompt_json, tem_imagem) in the same call, at the entry of the line
// (Fase 12, C-2.1).
func (j *Journal) EnqueuedJSON(rec queue.Record, prompt string, pj proxy.PromptJSON) {
	j.enqueue(rec, prompt, pj, true)
}

func (j *Journal) enqueue(rec queue.Record, prompt string, pj proxy.PromptJSON, withJSON bool) {
	if err := j.Store.JobEnqueued(rec); err != nil {
		j.logf("joblog: entrada de %s: %v", rec.ID, err)
	}
	if j.Content == nil {
		return
	}
	var err error
	switch {
	case withJSON && pj.Failed:
		// The body could not be scanned for images: keep no JSON (fail closed).
		_, err = j.Content.SavePromptJSONFailed(rec.ID, prompt, time.Now())
	case withJSON && pj.Melhora:
		_, err = j.Content.SavePromptJSONMelhora(rec.ID, prompt, pj.JSON, pj.HasImage, content.MelhoraTrace{Original: pj.Original,
			InstrucoesID: pj.InstrucoesID, IndiceID: pj.IndiceID, Exemplos: pj.Exemplos}, time.Now())
	case withJSON:
		_, err = j.Content.SavePromptJSON(rec.ID, prompt, pj.JSON, pj.HasImage, time.Now())
	default:
		_, _, err = j.Content.SavePrompt(rec.ID, prompt, time.Now())
	}
	if err != nil {
		j.logf("joblog: prompt de %s: %v", rec.ID, err)
	}
}

// Started marks the job as running, durably.
func (j *Journal) Started(id string, at time.Time, backend string) {
	if backend == "" {
		backend = j.Backend // the ticket names the backend; this is the default
	}
	if err := j.Store.JobStarted(id, at, backend); err != nil {
		j.logf("joblog: início de %s: %v", id, err)
	}
}

// Response returns the sink of the answer.
func (j *Journal) Response(id string) proxy.ResponseSink {
	if j.Content == nil {
		return &countSink{}
	}
	return j.Content.NewSink(id)
}

// Finish writes the end of the job durably; if that fails the async writer
// gets it, so the end is not lost for a transient lock.
func (j *Journal) Finish(r queue.Record) {
	if err := j.Store.Finish(r); err != nil {
		j.logf("joblog: fim de %s: %v (indo pelo gravador em lote)", r.ID, err)
		j.Store.Record(r)
	}
}

// countSink is the sink when the text is not recorded: it only counts.
type countSink struct{ n int64 }

func (c *countSink) Write(text string) { c.n += int64(len(text)) }
func (c *countSink) Finish(note string) (bool, int64) {
	return c.n > 0 || note != "", c.n
}

// Sample writes one second of the per-job extract: every running job gets one
// row per GPU, plus a CPU line (gpu = -1) when cpuSample is not nil. Nothing is
// written when no job runs. Call it from the 1 s GPU collector callback.
func (j *Journal) Sample(ts time.Time, ss []gpu.Sample, cpuSample *cpu.Sample) {
	if j.Running == nil {
		return
	}
	ids := j.Running()
	if len(ids) == 0 {
		return
	}
	rows := make([]store.JobSample, 0, len(ids)*(len(ss)+1))
	for _, id := range ids {
		for _, s := range ss {
			rows = append(rows, store.JobSample{TS: ts, JobID: id, GPU: s.Index, PowerW: s.PowerW, PowerLimitW: s.PowerLimitW,
				ClockMHz: s.ClockMHz, UtilPct: s.UtilPct, MemUsedMiB: s.MemUsedMiB, TempC: s.TempC})
		}
		if cpuSample != nil {
			rows = append(rows, store.JobSample{TS: ts, JobID: id, GPU: store.CPUGPUIndex, PowerW: cpuSample.PowerW, PowerLimitW: -1, ClockMHz: cpuSample.FreqMHz,
				UtilPct: int(cpuSample.UtilPct), MemUsedMiB: int(cpuSample.RAMUsedMiB), TempC: int(cpuSample.TempC)})
		}
	}
	if err := j.Store.AddJobSamples(rows); err != nil {
		j.logf("joblog: extrato de GPU: %v", err)
	}
}

// RecoverResponses completes the jobs Open closed as interrupted with what the
// content database knows: whether a partial answer had already arrived.
func (j *Journal) RecoverResponses(ctx context.Context) {
	if j.Content == nil {
		return
	}
	for _, r := range j.Store.Recovered() {
		in, ok, err := j.Content.Info(ctx, r.ID)
		if err != nil || !ok || in.RespBytes == 0 {
			continue
		}
		if err := j.Store.SetResponded(r.ID, in.RespBytes); err != nil {
			j.logf("joblog: resposta parcial de %s: %v", r.ID, err)
		}
	}
}
