package main

import (
	"strings"
	"testing"
)

func TestTreinoIsOpenedWithImmediateTransactions(t *testing.T) {
	if !treinoOptions(testKey(t)).TxLockImmediate {
		t.Error("the bake-off must open treino.db with TxLockImmediate (it writes next to the running gateway)")
	}
}

func TestACaseWhoseCorrectAnswerCannotBeScoredIsNeverReplayed(t *testing.T) {
	bad := synthCase{N: 40, Cat: "contagem_errada", Correct: "1,500 itens (ver seção 3)", Right: "1500", Wrong: "7"}
	good := synthCase{N: 41, Cat: "contagem_errada", Correct: "RESULTADO: 7", Right: "RESULTADO: 7", Wrong: "RESULTADO: 8"}
	cs := append(blindSet(30, 0), bad, good)
	f := newFixture(t, cs, scenarioPolicy(false))
	rid := f.makeRetrato()
	be := f.opts("base")
	be.RetratoID = rid
	code, out := f.run(be)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(out, "inaplicável: resposta_certa_sem_numero_unico") {
		t.Errorf("the case was not reported as inapplicable:\n%s", out)
	}
	for _, r := range f.gw.requests() {
		if r.Key == synthID(40) {
			t.Fatalf("the inapplicable case was replayed: %+v", r)
		}
	}
	if !strings.Contains(out, "| contagem_errada | 1 |") {
		t.Errorf("the applicable case was not measured:\n%s", out)
	}
}
