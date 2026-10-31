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
		`function openJob(id) { showTab("joblog");`, // "Detalhes" outside the log tab must open that tab (the box lives there)
	} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
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
