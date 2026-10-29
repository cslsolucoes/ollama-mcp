package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
)

func open(t *testing.T, o Options) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gw.db")
	s, err := Open(p, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}

func rec(id string, enq time.Time, status string) queue.Record {
	return queue.Record{
		ID: id, ClientIP: "10.0.0.1", ClientLabel: "host/1", Tool: "generate", Endpoint: "/api/generate", Model: "qwen3-ia",
		Lane: queue.LaneGen, PriorityIn: queue.Normal, PriorityFinal: queue.Normal, BytesIn: 100,
		EnqueuedAt: enq, StartedAt: enq.Add(50 * time.Millisecond), FinishedAt: enq.Add(1050 * time.Millisecond),
		WaitMS: 50, RunMS: 1000, Status: status, HTTPStatus: 200, PromptEval: 10, Eval: 20, LoadMS: 3,
	}
}

func TestRoundTripAndWALAnd0600(t *testing.T) {
	s, p := open(t, Options{BootID: "boot-A"})
	now := time.Now()
	s.Record(rec("r1", now, queue.StatusOK))
	s.Flush()
	rows, err := s.History(context.Background(), HistoryQuery{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %v", err, rows)
	}
	r := rows[0]
	if r.ID != "r1" || r.BootID != "boot-A" || r.WaitMS != 50 || r.Eval != 20 || r.PriorityFinal != "normal" || r.Lane != "geracao" {
		t.Fatalf("%+v", r)
	}
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q %v", mode, err)
	}
	if fi, err := os.Stat(p); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("db permission %v", fi.Mode())
	}
}

func TestBatchWriterManyRecords(t *testing.T) {
	s, _ := open(t, Options{BatchSize: 50})
	now := time.Now()
	for i := 0; i < 1000; i++ {
		s.Record(rec(fmt.Sprintf("r%04d", i), now.Add(time.Duration(i)*time.Millisecond), queue.StatusOK))
	}
	s.Flush()
	if s.Dropped() != 0 {
		t.Fatalf("dropped %d", s.Dropped())
	}
	rows, _ := s.History(context.Background(), HistoryQuery{Limit: 1000})
	if len(rows) != 1000 {
		t.Fatalf("rows = %d", len(rows))
	}
	// newest first
	if rows[0].ID != "r0999" {
		t.Fatalf("order: %s", rows[0].ID)
	}
}

func TestDropCounter(t *testing.T) {
	// A closed channel path: push after Close is a no-op, never a panic.
	s, _ := open(t, Options{})
	s.Close()
	s.Record(rec("late", time.Now(), queue.StatusOK))
	s.GPU([]GPURow{{TS: time.Now()}})
	s.Event("k", "d")
	s.Flush()
	// A full buffer: no consumer.
	s2 := &Store{opts: Options{}, ch: make(chan item, 2)}
	for i := 0; i < 5; i++ {
		s2.Record(queue.Record{ID: "z"})
	}
	if s2.Dropped() != 3 {
		t.Fatalf("dropped = %d, want 3", s2.Dropped())
	}
}

func TestBootEventOnlyWhenBootIDChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gw.db")
	count := func(s *Store, kind string) int {
		ev, _ := s.Events(context.Background(), 100)
		n := 0
		for _, e := range ev {
			if e.Kind == kind {
				n++
			}
		}
		return n
	}
	s, _ := Open(p, Options{BootID: "b1"})
	if count(s, "boot_novo") != 1 {
		t.Fatal("first boot not recorded")
	}
	s.Close()
	s, _ = Open(p, Options{BootID: "b1"})
	if count(s, "boot_novo") != 1 || count(s, "gateway_inicio") != 2 {
		t.Fatal("same boot must not add boot_novo")
	}
	s.Close()
	s, _ = Open(p, Options{BootID: "b2"})
	defer s.Close()
	if count(s, "boot_novo") != 2 {
		t.Fatal("new boot id not recorded")
	}
}

func TestAuditSyncAndOrder(t *testing.T) {
	s, _ := open(t, Options{})
	for i := 0; i < 3; i++ {
		if err := s.AddAudit(AuditEntry{User: "admin", IP: "10.0.0.5", Action: "promote", RequestID: fmt.Sprint(i), Before: "a", After: "b", Reason: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	a, err := s.Audits(context.Background(), 2)
	if err != nil || len(a) != 2 || a[0].RequestID != "2" {
		t.Fatalf("%v %+v", err, a)
	}
}

func TestStatsAndPercentile(t *testing.T) {
	s, _ := open(t, Options{})
	now := time.Now()
	for i := 1; i <= 20; i++ {
		r := rec(fmt.Sprintf("s%d", i), now.Add(-time.Minute), queue.StatusOK)
		r.WaitMS, r.RunMS, r.FinishedAt = int64(i*100), int64(i*1000), now.Add(-30*time.Second)
		s.Record(r)
	}
	bad := rec("e1", now.Add(-time.Minute), queue.StatusError)
	bad.ErrorClass, bad.FinishedAt = "upstream_502", now.Add(-20*time.Second)
	s.Record(bad)
	rej := rec("j1", now.Add(-time.Minute), queue.StatusRejected)
	rej.StartedAt, rej.FinishedAt = time.Time{}, now
	s.Record(rej)
	s.Flush()
	st, err := s.Stats(context.Background(), 15*time.Minute, "15m")
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 22 || st.OK != 20 || st.Errors != 1 || st.ByClass["upstream_502"] != 1 || st.ByStatus["rejeitado"] != 1 {
		t.Fatalf("%+v", st)
	}
	if st.RunP50 != 10000 || st.RunP95 != 19000 {
		t.Fatalf("run percentiles %d %d", st.RunP50, st.RunP95)
	}
	if st.EvalTokens != 22*20 || st.TokensPerSec <= 0 {
		t.Fatalf("tokens %+v", st)
	}
	if got := Percentile([]int64{5, 1, 3}, 50); got != 3 {
		t.Fatalf("p50 = %d", got)
	}
	if Percentile(nil, 95) != 0 {
		t.Fatal("empty percentile")
	}
}

func TestHistoryFilters(t *testing.T) {
	s, _ := open(t, Options{})
	now := time.Now()
	a := rec("a", now, queue.StatusOK)
	b := rec("b", now.Add(-48*time.Hour), queue.StatusOK)
	b.ClientLabel, b.Model = "other", "nomic-embed-text"
	s.Record(a)
	s.Record(b)
	s.Flush()
	ctx := context.Background()
	if r, _ := s.History(ctx, HistoryQuery{Client: "other"}); len(r) != 1 || r[0].ID != "b" {
		t.Fatalf("client filter %+v", r)
	}
	if r, _ := s.History(ctx, HistoryQuery{Model: "qwen3-ia"}); len(r) != 1 || r[0].ID != "a" {
		t.Fatalf("model filter %+v", r)
	}
	if r, _ := s.History(ctx, HistoryQuery{Since: now.Add(-time.Hour)}); len(r) != 1 {
		t.Fatalf("since filter %+v", r)
	}
	if r, _ := s.History(ctx, HistoryQuery{Limit: 1, Offset: 1}); len(r) != 1 || r[0].ID != "b" {
		t.Fatalf("paging %+v", r)
	}
	// SQL injection through a filter value is only data.
	if r, err := s.History(ctx, HistoryQuery{Client: "x' OR '1'='1"}); err != nil || len(r) != 0 {
		t.Fatalf("injection %v %v", err, r)
	}
}

func TestRetentionKeepsAuditAndEvents(t *testing.T) {
	s, _ := open(t, Options{ReqKeep: 90 * 24 * time.Hour, GPUKeep: 7 * 24 * time.Hour})
	now := time.Now()
	s.Record(rec("old", now.Add(-100*24*time.Hour), queue.StatusOK))
	s.Record(rec("new", now.Add(-10*24*time.Hour), queue.StatusOK))
	s.GPU([]GPURow{{TS: now.Add(-8 * 24 * time.Hour), GPU: 0}, {TS: now.Add(-1 * time.Hour), GPU: 0}})
	s.AddAudit(AuditEntry{TS: now.Add(-400 * 24 * time.Hour), User: "admin", Action: "remove"})
	s.Flush()
	r, g, err := s.Prune(context.Background(), now)
	if err != nil || r != 1 || g != 1 {
		t.Fatalf("pruned %d %d %v", r, g, err)
	}
	if rows, _ := s.History(context.Background(), HistoryQuery{}); len(rows) != 1 || rows[0].ID != "new" {
		t.Fatalf("%+v", rows)
	}
	if a, _ := s.Audits(context.Background(), 10); len(a) != 1 {
		t.Fatal("audit must never expire")
	}
	if ev, _ := s.Events(context.Background(), 10); len(ev) == 0 {
		t.Fatal("events must never expire")
	}
}

func TestMaintainRunsOncePerDueDate(t *testing.T) {
	s, _ := open(t, Options{VacuumEvery: time.Hour})
	now := time.Now()
	if err := s.Maintain(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	first := s.metaTime("last_prune")
	if first.IsZero() || s.metaTime("last_vacuum").IsZero() {
		t.Fatal("maintenance did not record its dates")
	}
	s.Maintain(context.Background(), now.Add(time.Minute))
	if !s.metaTime("last_prune").Equal(first) {
		t.Fatal("prune ran again before it was due")
	}
}

func TestBootIDReader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "boot_id")
	os.WriteFile(p, []byte("1234-abcd\n"), 0o600)
	if BootID(p) != "1234-abcd" {
		t.Fatalf("%q", BootID(p))
	}
	if BootID(filepath.Join(t.TempDir(), "nope")) != "unknown" {
		t.Fatal("missing file must give unknown")
	}
}

func TestGPURows(t *testing.T) {
	s, _ := open(t, Options{BootID: "b"})
	now := time.Now()
	s.GPU([]GPURow{{TS: now, GPU: 1, BusID: "00000000:04:00.0", PowerW: 99.5, MemUsedMiB: 7000, UtilPct: 80, TempC: 60, AER: 3}})
	s.Flush()
	g, err := s.GPUSince(context.Background(), now.Add(-time.Minute), 10)
	if err != nil || len(g) != 1 || g[0].PowerW != 99.5 || g[0].AER != 3 || g[0].GPU != 1 {
		t.Fatalf("%v %+v", err, g)
	}
}
