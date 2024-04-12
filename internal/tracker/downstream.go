package tracker

import (
	"sync"
)

// DownstreamConns tracks connections per downstream based on a
// unique string identifier.
// DownstreamConns safe for concurrent use.
type DownstreamConns struct {
	// mu protects the resources of DownstreamConns
	mu sync.Mutex

	// connCounts is a map of downstreamID to a count of connections
	connCounts map[string]uint32
}

// NewDownstreamConns initializes and returns a DownstreamConns with
func NewDownstreamConns() *DownstreamConns {
	return &DownstreamConns{
		connCounts: map[string]uint32{},
	}
}

// TryRecordConnection checks if a downstreamID's connections are below the provided max
// and if so records an additional connection for the downstream.
// If the downstream has no history, a new count will be started.
// The return indicates if the new connection should be allowed.
func (t *DownstreamConns) TryRecordConnection(downstreamID string, max uint32) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	value, ok := t.connCounts[downstreamID]
	if !ok {
		t.connCounts[downstreamID] = 1
		return true
	}
	if value < max {
		t.connCounts[downstreamID]++
		return true
	}
	return false
}

// ConnectionEnded decrements the count of connections for a given downstreamID.
// ConnectionEnded requires that a connection was begun previously,
// otherwise it may access a key which does not exist.
func (t *DownstreamConns) ConnectionEnded(downstreamID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connCounts[downstreamID]--
}
