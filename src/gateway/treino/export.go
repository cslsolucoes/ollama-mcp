package treino

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"
	"unicode/utf8"
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

// requestView holds the keys of the canonical request that make the pair. The keys are resolved by the
// decoder of encoding/json, which is the one the Ollama uses: case does not matter (with Unicode simple
// folding, so "ſystem" is "system") and the last repeated key wins. The JSONL therefore never says a
// request had a content other than the one the model was given (I-3 of the review of 12.9).
type requestView struct {
	Messages json.RawMessage `json:"messages"`
	Prompt   json.RawMessage `json:"prompt"`
	System   json.RawMessage `json:"system"`
}

// surrogateEscape is how a JSON text can carry a lone surrogate while still being valid UTF-8.
var surrogateEscape = regexp.MustCompile(`\\[uU][dD][89a-fA-F][0-9a-fA-F]{2}`)

// utf8Message makes a message of the chat valid UTF-8 (I-4): the Ollama decodes an invalid byte as U+FFFD, so the
// message is decoded and written again, which gives the text the model was given. A message that is already
// valid is copied as it is.
func utf8Message(raw json.RawMessage) (json.RawMessage, error) {
	if utf8.Valid(raw) && !surrogateEscape.Match(raw) {
		return raw, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errNotExportable
	}
	return json.Marshal(v)
}

// conversation returns the messages of the request before the answer (C-3.13, C-3.14), or errNotExportable
// when the request has no form of a conversation. It is the ONE place that decides the form: the sample
// records the result (amostras.forma_ok) and the predicate reads it (C-3.1), so a case that counts as
// exportable always has its line.
func conversation(endpoint string, promptJSON []byte) ([]json.RawMessage, error) {
	var v requestView
	if err := json.Unmarshal(promptJSON, &v); err != nil {
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
	switch endpoint {
	case "/api/chat", "/v1/chat/completions": // the whole history of the conversation (C-3.14)
		var m []json.RawMessage
		if json.Unmarshal(v.Messages, &m) != nil || len(m) == 0 {
			return nil, errNotExportable
		}
		for _, raw := range m {
			ok, err := utf8Message(raw)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, ok)
		}
	default: // /api/generate, /v1/completions: system (optional) and prompt
		var prompt, system string
		if json.Unmarshal(v.Prompt, &prompt) != nil || prompt == "" {
			return nil, errNotExportable
		}
		if len(v.System) > 0 {
			if json.Unmarshal(v.System, &system) != nil {
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
	return msgs, nil
}

// FormOK says whether the request of a sample has the form of a conversation (the part of C-3.1 that needs the
// text of the request).
func FormOK(endpoint string, promptJSON []byte) bool {
	_, err := conversation(endpoint, promptJSON)
	return err == nil
}

// buildLine makes the line of C-3.13 and C-3.14 from the canonical request of the
// sample and the correct answer of the occurrence.
func buildLine(r ElRow, promptJSON []byte, parte string) ([]byte, error) {
	msgs, err := conversation(r.Endpoint, promptJSON)
	if err != nil {
		return nil, err
	}
	ans, err := json.Marshal(chatMsg{Role: "assistant", Content: r.RespostaCorreta})
	if err != nil {
		return nil, err
	}
	msgs = append(msgs, ans)
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
