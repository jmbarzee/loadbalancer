package core

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// upstreamHealth records the health of upstreams .
// Safe for concurrent access.
type upstreamHealth struct {
	// mu protects the resources of upstreamHealth
	mu sync.Mutex

	// upstreams holds the health status of upstreams
	upstreams map[uuid.UUID]bool
}

// rangeOverConcurrently ranges over the health of upstreams.
// f is the function to be run on each k,v pair
// f is started in a new goroutine to prevent limit locking of upstreamHealth
func (h *upstreamHealth) rangeOverConcurrently(f func(uuid.UUID, bool)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, health := range h.upstreams {
		go f(id, health)
	}
}

func (h *upstreamHealth) setHealth(id uuid.UUID, health bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.upstreams[id] = health
}

func (t *Trafficker) routineHealthChecks(ctx context.Context) {
	ticker := time.NewTicker(t.config.HealthCheckInterval())

	for {
		select {
		case <-ctx.Done():
			t.logger.Printf("Context closed, ending health checks\n")
			return
		case <-ticker.C:
			// Its especially important to do this concurrently because
			// of the call to dialRetryBackoff in healthCheckUpstream
			t.upstreamHealth.rangeOverConcurrently(func(id uuid.UUID, healthOld bool) {
				healthNew := t.healthCheckUpstream(id)
				if healthOld {
					if !healthNew {
						// no longer healthy
						t.upstreamHealth.setHealth(id, healthNew)
						t.upstreamTracker(id).UpstreamUnavailable(id)
						t.logger.Printf("Health check: upstream(%v) failed health check and removed from availability\n", id)
					}
					// still healthy, do nothing
				} else {
					if healthNew {
						// returned to healthy
						t.upstreamHealth.setHealth(id, healthNew)
						t.upstreamTracker(id).UpstreamAvailable(id)
						t.logger.Printf("Health check: upstream(%v) passed health check and returned to availability\n", id)
					} else {
						// still unhealthy
						t.logger.Printf("Health check: upstream(%v) is still failing health check\n", id)
					}
				}
			})
		}
	}
}

func (t *Trafficker) healthCheckUpstream(id uuid.UUID) bool {
	conn, err := t.dialRetryBackoff(t.upstreamAddr(id))
	if err != nil {
		// No need to log errors here, they are handled by dialRetryBackoff
		return false
	}
	err = conn.Close()
	if err != nil {
		t.logger.Printf("Health check: upstream(%v) failed to close: %v\n", id, err)
		return false
	}
	return true
}
