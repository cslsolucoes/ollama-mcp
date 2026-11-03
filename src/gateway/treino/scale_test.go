package treino

// Scale of the recompute (I-2 of the review of 12.6): the groups are rebuilt in the
// writers' turn after every change, so their cost is the cost of every report.

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// seedScale fills a store with n occurrences and about 3n jobs, samples with blobs of a
// few KB, a mix of states, some shared jobs (so the groups are not all singletons) and
// every confirmed one exportable.
func seedScale(t *testing.T, s *Store, n int) {
	t.Helper()
	blob := []byte(strings.Repeat("x", 2048))
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	insO, err := tx.Prepare(`INSERT INTO ocorrencias(id,criada_em,atualizada_em,estado,categoria,severidade,descricao,resposta_correta,origem,tool,job_do_erro,doc_hash,doc_key_id,grupo_id,particao,revisada_em)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	insJ, _ := tx.Prepare(`INSERT INTO ocorrencia_jobs(ocorrencia_id,job_id,papel,backend,ordem) VALUES(?,?,?,?,?)`)
	insA, _ := tx.Prepare(`INSERT OR IGNORE INTO amostras(job_id,copiada_em,endpoint,modelo,think,prompt_json,prompt_texto,resposta,prompt_hmac,conteudo_hmac,key_id,
		prompt_truncado,prompt_json_truncado,tem_imagem,doc_hash,resp_final,resp_truncada,status,http_status) VALUES(?,?,?,?,?,?,?,?,?,?,?,0,0,0,?,1,0,'ok',200)`)
	cats := []string{"omitiu", "inventou", "numeracao_errada", "contagem_errada"}
	states := []string{EstadoConfirmada, EstadoConfirmada, EstadoNova, EstadoDescartada}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%032x", i+1)
		estado := states[i%len(states)]
		job := func(k int) string { return fmt.Sprintf("%032x", 1_000_000+(i*3+k)%(n*3-n/10)) } // a tenth of the jobs are shared
		doc := ""
		if i%7 == 0 {
			doc = fmt.Sprintf("%064x", i%500)
		}
		side := ""
		if estado == EstadoConfirmada {
			side = []string{ParteTreino, ParteTeste}[i%2]
		}
		if _, err := insO.Exec(id, int64(i), int64(i), estado, cats[i%len(cats)], "media", "descricao sintetica", "resposta certa", "claude", "extract", job(2), doc, "k1", "", side, int64(i)); err != nil {
			t.Fatal(err)
		}
		for k := 0; k < 3; k++ {
			j := job(k)
			papel := []string{"map1", "map2", "final"}[k]
			if _, err := insJ.Exec(id, j, papel, "primario", k); err != nil {
				t.Fatal(err)
			}
			if _, err := insA.Exec(j, int64(i), "/api/generate", "qwen3-ia", "off", blob, blob, blob, "ph"+j, "ch"+j, "k1", doc); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestScaleRecomputeAndSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test: 10,000 occurrences")
	}
	if raceEnabled {
		t.Skip("timing targets do not apply under the race detector (20 to 30 times slower)")
	}
	n := 10000
	if v, err := strconv.Atoi(os.Getenv("TREINO_SCALE_N")); err == nil && v > 0 {
		n = v
	}
	s := openStore(t, Options{})
	t0 := time.Now()
	seedScale(t, s, n)
	t.Logf("seeded %d occurrences in %v (%d MiB of live data)", n, time.Since(t0).Round(time.Millisecond), s.Size()>>20)
	best := func(f func() error) time.Duration {
		var min time.Duration
		for i := 0; i < 3; i++ {
			t1 := time.Now()
			if err := f(); err != nil {
				t.Fatal(err)
			}
			if d := time.Since(t1); i == 0 || d < min {
				min = d
			}
		}
		return min
	}
	rec := best(func() error {
		return s.withTx(ctx, func(tx *sql.Tx) error { return s.recompute(ctx, tx, s.opts.Now()) })
	})
	sum := best(func() error { _, err := s.Summary(ctx); return err })
	t.Logf("recompute %v, Summary %v (%d occurrences)", rec.Round(time.Millisecond), sum.Round(time.Millisecond), n)
	if n >= 10000 {
		if rec > 200*time.Millisecond {
			t.Errorf("recompute took %v, the target is 200 ms", rec)
		}
		if sum > 100*time.Millisecond {
			t.Errorf("Summary took %v, the target is 100 ms", sum)
		}
	}
}
