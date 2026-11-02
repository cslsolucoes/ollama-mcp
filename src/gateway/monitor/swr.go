package monitor

import (
	"sync"
	"time"
)

// swrCache is a stale-while-revalidate cache for the database-backed parts of
// the state. Incident of 2026-11-01 (gateway 0.12.5): a read that never
// returned, made while the cache mutex was held, froze every panel request.
// The rules here are therefore:
//   - the mutex only guards the value and the flags, never the query;
//   - a caller never waits for the database, except the very first one, and
//     only up to a short limit;
//   - at most one refresh runs at a time; a stuck one costs one goroutine, not
//     one per visitor.
type swrCache[T any] struct {
	mu   sync.Mutex
	at   time.Time
	val  T
	have bool
	busy bool
	done chan struct{} // closed when the running refresh ends
}

// get returns the cached value. When it is older than ttl and no refresh is
// running, it starts one (fetch runs in its own goroutine, outside the mutex;
// ok=false keeps the previous value). Without any value yet it waits for the
// first refresh at most firstWait, then returns the zero value.
func (c *swrCache[T]) get(now func() time.Time, ttl, firstWait time.Duration, fetch func() (T, bool)) T {
	c.mu.Lock()
	if c.have && now().Sub(c.at) < ttl {
		v := c.val
		c.mu.Unlock()
		return v
	}
	if !c.busy {
		c.busy = true
		c.done = make(chan struct{})
		done := c.done
		started := now() // the value is as old as the moment the read began
		go func() {
			v, ok := fetch()
			c.mu.Lock()
			if ok {
				c.val, c.have = v, true
			}
			if ok || c.have {
				c.at = started // also after a failure: do not hammer a sick database
			}
			c.busy = false
			c.mu.Unlock()
			close(done)
		}()
	}
	if c.have {
		v := c.val
		c.mu.Unlock()
		return v
	}
	done := c.done
	c.mu.Unlock()
	t := time.NewTimer(firstWait)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
	}
	c.mu.Lock()
	v := c.val
	c.mu.Unlock()
	return v
}

// isBusy reports whether a refresh is running (tests wait on it).
func (c *swrCache[T]) isBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busy
}
