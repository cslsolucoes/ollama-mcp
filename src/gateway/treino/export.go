package treino

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// exportMeta is the only metadata of an exported line (C-3.15): no free text.
type exportMeta struct {
	OcorrenciaID string `json:"ocorrencia_id"`
	JobID        string `json:"job_id"`
	Papel        string `json:"papel"`
	Tool         string `json:"tool"`
	Categoria    string `json:"categoria"`
	Particao     string `json:"particao"`
}

type exportLine struct {
	Messages []json.RawMessage `json:"messages"`
	Meta     exportMeta        `json:"meta"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// exportBatch is how many exported occurrences are recorded in the history at once.
const exportBatch = 20

var errNotExportable = errors.New("treino: pedido sem forma de conversa")

// buildLine makes the line of C-3.13 and C-3.14 from the canonical request of the
// sample and the correct answer of the occurrence.
func buildLine(r ElRow, promptJSON []byte, parte string) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(promptJSON, &body); err != nil {
		return nil, errNotExportable
	}
	var msgs []json.RawMessage
	add := func(role, content string) error {
		b, err := json.Marshal(chatMsg{Role: role, Content: content})
		if err != nil {
			return err
		}
		msgs = append(msgs, b)
		return nil
	}
	switch r.Endpoint {
	case "/api/chat", "/v1/chat/completions": // the whole history of the conversation (C-3.14)
		var m []json.RawMessage
		if json.Unmarshal(body["messages"], &m) != nil || len(m) == 0 {
			return nil, errNotExportable
		}
		msgs = append(msgs, m...)
	default: // /api/generate, /v1/completions: system (optional) and prompt
		var prompt, system string
		if json.Unmarshal(body["prompt"], &prompt) != nil || prompt == "" {
			return nil, errNotExportable
		}
		if raw, ok := body["system"]; ok {
			if json.Unmarshal(raw, &system) != nil {
				return nil, errNotExportable
			}
		}
		if system != "" {
			if err := add("system", system); err != nil {
				return nil, err
			}
		}
		if err := add("user", prompt); err != nil {
			return nil, err
		}
	}
	if err := add("assistant", r.RespostaCorreta); err != nil {
		return nil, err
	}
	b, err := json.Marshal(exportLine{Messages: msgs, Meta: exportMeta{OcorrenciaID: r.ID, JobID: r.JobDoErro, Papel: r.Papel, Tool: r.Tool, Categoria: r.Categoria, Particao: parte}})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// exportBatchBytes bounds what a batch holds in memory before it is recorded and sent.
const exportBatchBytes = 8 << 20

// historyTimeout is the deadline of the write of a batch of `exportou` rows. It runs
// with a context of its own: an export whose context ended (the client left, the
// deadline passed) still leaves the record of what it delivered.
const historyTimeout = 30 * time.Second

// Export streams the JSONL of one part (C-3.11 to C-3.15, C-7.7), the line of every
// occurrence of that partition that passes the single predicate Eligible. Nothing
// is kept: lines go to w as they are made.
//
// The record is WRITE-AHEAD: the `exportou` rows of a batch (campo = the part, quem
// = who downloads, one per occurrence) are written BEFORE its lines are sent, in
// their own short transaction with a context of their own. A batch whose delivery
// then fails leaves rows for lines that were not (all) delivered: the safe side. A
// line that reached the client always has its row, whatever fails next (the
// database, the context, the connection). Reading uses short queries, one
// occurrence at a time, and the writers' turn is taken only to record, so a slow
// reader never blocks a report or the monitor (C-6.10). flush, when given, is called
// after every line. It returns the number of lines sent.
func (s *Store) Export(ctx context.Context, parte string, a Actor, w io.Writer, flush func()) (int, error) {
	return s.ExportUntil(ctx, parte, a, w, flush, time.Time{})
}

// ErrExportDeadline ends an export that ran past its deadline.
var ErrExportDeadline = errors.New("treino: a exportação passou do prazo")

// ExportUntil is Export with a deadline that is checked between the lines, so the
// caller can pass context.Background() (no Done channel for database/sql to watch,
// pattern of 0.12.6) and still bound the work. A zero deadline means none.
func (s *Store) ExportUntil(ctx context.Context, parte string, a Actor, w io.Writer, flush func(), deadline time.Time) (int, error) {
	if parte != ParteTreino && parte != ParteTeste {
		return 0, ErrInvalid
	}
	// A superset in SQL (only the part asked for); the single predicate decides all the rest.
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM ocorrencias WHERE particao=? ORDER BY id`, parte)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	n := 0
	var batchIDs []string
	var batchLines [][]byte
	batchBytes := 0
	send := func() error {
		if len(batchIDs) == 0 {
			return nil
		}
		hctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
		err := s.withTx(hctx, func(tx *sql.Tx) error {
			now := s.opts.Now()
			for _, id := range batchIDs {
				if err := hist(hctx, tx, now, id, AcaoExportou, parte, "", "", a); err != nil {
					return err
				}
			}
			return nil
		})
		cancel()
		if err != nil {
			return err // nothing of this batch was sent
		}
		for _, l := range batchLines {
			if _, err := w.Write(l); err != nil {
				return err
			}
			if flush != nil {
				flush()
			}
			n++
		}
		batchIDs, batchLines, batchBytes = nil, nil, 0
		return nil
	}
	for _, id := range ids {
		if !deadline.IsZero() && time.Now().After(deadline) {
			_ = send() // what is already built is recorded and delivered
			return n, ErrExportDeadline
		}
		r, err := scanEl(s.db.QueryRowContext(ctx, elSelect+`WHERE o.id=?`, id))
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return n, err
		}
		if r.Particao != parte { // changed since the list was read
			continue
		}
		if ok, _ := Eligible(r, false); !ok {
			continue
		}
		var pj []byte
		if err := s.db.QueryRowContext(ctx, `SELECT prompt_json FROM amostras WHERE job_id=?`, r.JobDoErro).Scan(&pj); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return n, err
		}
		line, err := buildLine(r, pj, parte)
		if err != nil {
			if errors.Is(err, errNotExportable) {
				continue
			}
			return n, err
		}
		pj = nil
		batchIDs, batchLines = append(batchIDs, id), append(batchLines, line)
		batchBytes += len(line)
		if len(batchIDs) >= exportBatch || batchBytes >= exportBatchBytes {
			if err := send(); err != nil {
				return n, err
			}
		}
	}
	return n, send()
}
