package monitor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/queue"
	"github.com/cslsoftwares/mcpollama/gateway/store"
)

// The four station fields travel in the job API and in both exports.
func TestStationInJobAPIAndExports(t *testing.T) {
	f := newJobFixture(t, time.Minute, false)
	r := queue.Record{ID: idN(7), ClientIP: "10.0.0.9", ClientLabel: "002-1985/claiton.linhares", Tool: "extract", Endpoint: "/api/generate", Model: "qwen3-ia",
		Lane: queue.LaneGen, PriorityIn: queue.Normal, PriorityFinal: queue.Normal, EnqueuedAt: time.Now(), Status: queue.StatusQueued,
		StationComputer: "002-1985", StationUser: "claiton.linhares", StationDomain: "CSLSOLUCOES", StationMAC: "5CCD5BF40B2D"}
	if err := f.st.JobEnqueued(r); err != nil {
		t.Fatal(err)
	}
	r2 := r
	r2.ID = idN(8)
	r2.StationUser = "=cmd" // csvSafe must defuse it in the CSV
	if err := f.st.JobEnqueued(r2); err != nil {
		t.Fatal(err)
	}

	w := do(f.h, "GET", "/monitor/api/jobs/"+idN(7), "", nil)
	var d struct{ Job store.JobRow }
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || d.Job.StationComputer != "002-1985" || d.Job.StationUser != "claiton.linhares" ||
		d.Job.StationDomain != "CSLSOLUCOES" || d.Job.StationMAC != "5CCD5BF40B2D" {
		t.Fatalf("%v %s", err, w.Body)
	}
	for _, k := range []string{`"station_computer"`, `"station_user"`, `"station_domain"`, `"station_mac"`} {
		if !strings.Contains(w.Body.String(), k) {
			t.Fatalf("API lacks %s: %s", k, w.Body)
		}
	}
	// the table column stays the label
	if !strings.Contains(w.Body.String(), `"client_label":"002-1985/claiton.linhares"`) {
		t.Fatalf("client_label changed: %s", w.Body)
	}

	jl := do(f.h, "GET", "/monitor/api/jobs/export?formato=jsonl", "", nil).Body.String()
	if !strings.Contains(jl, `"station_mac":"5CCD5BF40B2D"`) || !strings.Contains(jl, `"station_computer":"002-1985"`) {
		t.Fatalf("jsonl: %s", jl)
	}
	csv := do(f.h, "GET", "/monitor/api/jobs/export?formato=csv", "", nil).Body.String()
	head := strings.SplitN(csv, "\n", 2)[0]
	if !strings.HasSuffix(strings.TrimSpace(head), "maquina,usuario,dominio,mac") {
		t.Fatalf("csv header: %q", head)
	}
	if !strings.Contains(csv, "002-1985,claiton.linhares,CSLSOLUCOES,5CCD5BF40B2D") || !strings.Contains(csv, "'=cmd") || strings.Contains(csv, ",=cmd") {
		t.Fatalf("csv: %s", csv)
	}
}

// Detalhes shows the four lines (built with textContent by the shared loop) and
// the file keeps LF.
func TestIndexDetailHasStationLines(t *testing.T) {
	page := string(rawIndexHTML)
	for _, want := range []string{`"Máquina", j.station_computer`, `"Usuário", j.station_user`, `"Domínio", j.station_domain`, `"MAC", j.station_mac`} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if strings.Contains(page, "\r") {
		t.Fatal("index.html has CR: keep it LF")
	}
}
