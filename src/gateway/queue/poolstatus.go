package queue

import (
	"time"

	"github.com/cslsoftwares/mcpollama/gateway/pool"
)

// PoolStatus is the "Backends" board: the pool's view of each backend plus what
// the line knows (slots in use, waiting requests the backend could take; the
// primary could take all of them).
func (m *Manager) PoolStatus() []pool.Status {
	now := time.Now()
	loads := map[string]pool.Load{}
	m.mu.Lock()
	w := append([]*Ticket(nil), m.waiting[LaneGen]...)
	for id, n := range m.bRun {
		loads[id] = pool.Load{Used: n}
	}
	pr := m.pool.Primary()
	l := loads[pr.ID]
	l.Waiting = len(w)
	loads[pr.ID] = l
	for _, sb := range m.pool.Secondaries() {
		l := loads[sb.ID]
		for _, t := range w {
			if m.pool.Eligible(sb, t.req.Model, m.effectiveLocked(t, now).String(), pool.EstimateTokens(t.req.BytesIn)) {
				l.Waiting++
			}
		}
		loads[sb.ID] = l
	}
	m.mu.Unlock()
	return m.pool.Status(now, loads)
}
