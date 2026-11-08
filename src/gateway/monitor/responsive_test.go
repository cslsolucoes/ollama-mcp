package monitor

import (
	"regexp"
	"strings"
	"testing"
)

// Regression (2026-10-30, Firefox at ~570 px): sections grew wider than the window and the cards and
// tables were cut off at the right, because the grid column of <main> could not shrink below its content.
// The page must keep a shrinkable column, make the narrow grids wrap, keep tables stable between SSE
// messages, and never swallow a drawing error.
func TestIndexIsResponsiveAndStable(t *testing.T) {
	page := string(rawIndexHTML)
	for _, want := range []string{
		`grid-template-columns:minmax(0,1fr)`, // main: the column may shrink, the .wrap boxes scroll inside
		`minmax(min(190px,100%),1fr)`,         // GPU / CPU cards wrap on a narrow screen
		`@media (max-width:640px)`,
		`function safeRender`,
		`dataset.k`, // stable keys: buttons survive a redraw
		`Erro ao desenhar o painel`,
		`RUN_PAGE_SIZE = 2`,                   // "Em execução": always 2 rows, the rest is paged (owner, 2026-11-08)
		`WAIT_PAGE_SIZE = 5`,                  // "Fila de espera": always 5 rows, the rest is paged
		`DONE_PAGE_SIZE = 5`,                  // "Concluídos nos últimos N min": always 5 rows, the rest is paged
		`pageOf("done", rows, DONE_PAGE_SIZE`, // the third board goes through the pager too
		`padRows(doneRows, DONE_PAGE_SIZE`,
		`id="donest"`,
		`id="donepg"`,
		`"Rótulo"`, // label column (client_label) next to "Cliente" (client_ip)
		`"Cliente"`,
		`pageOf("running", q.running, RUN_PAGE_SIZE`, // both boards really go through the pager
		`pageOf("waiting", q.waiting, WAIT_PAGE_SIZE`,
		`function padRows(`, // blank rows up to the fixed height (owner, 2026-11-05)
		`padRows(runRows, RUN_PAGE_SIZE`,
		`padRows(waitRows, WAIT_PAGE_SIZE`,
		`function boardStatus(`, // the one-line status of each board is always there
		`id="runst"`,
		`id="waitst"`,
		`function openJob(id) { showTab("joblog");`, // "Detalhes" outside the log tab must open that tab (the box lives there)
	} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	// The pager of the two boards is always drawn (owner, 2026-11-05: the 0.13.1 pager only appeared above the
	// limit and the empty boards shrank to one line, which was not what he asked for).
	if strings.Contains(page, "if (total <= size)") {
		t.Error("the pager is hidden again when the board fits in one page")
	}
	// Both boards must pass blank rows to table(), never an empty list (an empty list hides the table).
	if !regexp.MustCompile(`table\("running", runHeads, padRows\(runRows, RUN_PAGE_SIZE, runHeads\.length\)`).MatchString(page) ||
		!regexp.MustCompile(`table\("waiting", waitHeads, padRows\(waitRows, WAIT_PAGE_SIZE, waitHeads\.length\)`).MatchString(page) {
		t.Error("a paged board is not drawn through padRows")
	}
	// Section order (owner, 2026-11-08): GPUs and CPU / RAM at the top, before "Em execução".
	iGPU, iCPU, iRun := strings.Index(page, "<h2>GPUs</h2>"), strings.Index(page, "<h2>CPU / RAM</h2>"), strings.Index(page, "<h2>Em execu")
	if iGPU < 0 || iCPU < 0 || iRun < 0 || iGPU > iRun || iCPU > iRun {
		t.Errorf("GPUs (%d) and CPU / RAM (%d) must come before Em execução (%d)", iGPU, iCPU, iRun)
	}
	// All three boards are drawn through padRows with a fixed height.
	if !regexp.MustCompile(`table\("donelist", doneHeads, padRows\(doneRows, DONE_PAGE_SIZE, doneHeads\.length\)`).MatchString(page) {
		t.Error("the done board is not drawn through padRows")
	}
	// No empty catch around the render (the old "try { render(...) } catch (x) {}").
	if regexp.MustCompile(`catch\s*\(\w+\)\s*\{\s*\}\s*stopPolling`).MatchString(page) {
		t.Error("es.onmessage swallows render errors again")
	}
	m := regexp.MustCompile(`es\.onmessage = function \(e\) \{[\s\S]*?\n  \};`).FindString(page)
	if m == "" {
		t.Fatal("es.onmessage handler not found")
	}
	if regexp.MustCompile(`catch\s*\(\w+\)\s*\{\s*\}`).MatchString(m) {
		t.Error("es.onmessage has an empty catch")
	}
	if !strings.Contains(m, "safeRender") && !strings.Contains(m, "console.error") {
		t.Error("es.onmessage does not report errors")
	}
	if strings.Contains(page, "setAttribute(\"style\"") || regexp.MustCompile(`<[a-z][^>]* style=`).MatchString(page) {
		t.Error("inline style is blocked by the CSP")
	}
}
