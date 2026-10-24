package tools

import (
	"context"
	"time"
)

// Queue is the GPU queue (PERF-002): a counting semaphore placed before the
// Ollama client, because the server has one resident model behind four GPUs
// and parallel generations only wait in line there. A nil *Queue never blocks
// (used by the offline catalog dump). ping and tools/list never reach it.
type Queue struct{ sem chan struct{} }

// NewQueue returns a queue with n slots (n < 1 means 1).
func NewQueue(n int) *Queue {
	if n < 1 {
		n = 1
	}
	return &Queue{sem: make(chan struct{}, n)}
}

// Acquire waits for a slot, honouring ctx (a cancelled call leaves the line:
// PERF-011). waited is the time spent in line, reported in the usage line.
func (q *Queue) Acquire(ctx context.Context) (release func(), waited time.Duration, err error) {
	if q == nil {
		return func() {}, 0, nil
	}
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	select {
	case q.sem <- struct{}{}:
		return func() { <-q.sem }, time.Since(start), nil
	case <-ctx.Done():
		return nil, time.Since(start), ctx.Err()
	}
}
