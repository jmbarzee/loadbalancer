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

	// connCounts is a map of downstream to a count of connections
	connCounts map[string]uint32
}

// NewDownstreamConns initializes and returns a DownstreamConns with
func NewDownstreamConns() *DownstreamConns {
	return &DownstreamConns{
		connCounts: map[string]uint32{},
	}
}

// TryBeginConnection checks if a downstream is below a maximum
// and if so records an additional connection for the client.
// If the client has no history, a new count will be started.
// The return indicates if the new connection should be allowed.
func (t *DownstreamConns) TryBeginConnection(downstream string, max uint32) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	value, ok := t.connCounts[downstream]
	if !ok {
		t.connCounts[downstream] = 1
		return true
	}
	if value < max {
		t.connCounts[downstream] = value + 1
		return true
	}
	return false
}

// EndConnection decrements the count of connections for a given downstream.
// EndConnection requires that a connection was begun previously,
// otherwise it may access a key which does not exist.
func (t *DownstreamConns) EndConnection(downstream string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connCounts[downstream] -= t.connCounts[downstream]
}
