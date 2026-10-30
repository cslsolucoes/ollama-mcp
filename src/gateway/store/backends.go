package store

import (
	"context"
	"time"
)

// BackendRow is one sample of one backend of the pool (Fase 10.4), written
// every --sample-store-interval and kept like the GPU samples.
type BackendRow struct {
	TS        time.Time `json:"ts"`
	BackendID string    `json:"backend_id"`
	State     string    `json:"estado"`
	SlotsUsed int       `json:"slots_usados"`
	Queue     int       `json:"fila"`
	P50MS     int64     `json:"p50_ms"`
}

// Backends queues one round of backend samples. Never blocks. The rows hold
// ids, states and counters only: no URL, no token.
func (s *Store) Backends(rows []BackendRow) {
	if len(rows) > 0 {
		s.push(item{kind: kBackend, backends: rows})
	}
}

// BackendsSince returns the backend samples newer than since, oldest first.
func (s *Store) BackendsSince(ctx context.Context, since time.Time, limit int) ([]BackendRow, error) {
	if limit <= 0 || limit > 100000 {
		limit = 20000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts,backend_id,estado,slots_usados,fila,p50_ms FROM backend_samples WHERE ts >= ? ORDER BY ts LIMIT ?`, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackendRow{}
	for rows.Next() {
		var b BackendRow
		var ts int64
		if err := rows.Scan(&ts, &b.BackendID, &b.State, &b.SlotsUsed, &b.Queue, &b.P50MS); err != nil {
			return nil, err
		}
		b.TS = time.UnixMilli(ts)
		out = append(out, b)
	}
	return out, rows.Err()
}
