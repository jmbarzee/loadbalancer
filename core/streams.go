package core

import (
	"net"

	"github.com/google/uuid"
	"github.com/jmbarzee/loadbalancer/internal/tracker"
)

// Upstream defines the necessary values for Trafficker to
// forward traffic and loadBalance to an upstream.
type Upstream interface {
	// ID is used primarily to look up the Upstream's connections
	// in the rate limit cache. Maybe better thought of as the "connection tracker".
	ID() uuid.UUID

	// Group returns the group name that the upstream
	Group() string

	// Provides necessary information to call net.DialTCP()
	TCPAddr() *net.TCPAddr
}

// upstreamAddr is a convenience function to lookup the addr for a given upstream id
func (t *Trafficker) upstreamAddr(id uuid.UUID) *net.TCPAddr {
	return t.upstreams[id].TCPAddr()
}

// upstreamGroup is a convenience function to lookup the group for a given upstream id
func (t *Trafficker) upstreamGroup(id uuid.UUID) string {
	return t.upstreams[id].Group()
}

// upstreamGroup is a convenience function to lookup the upstreamTracker for a given upstream id
func (t *Trafficker) upstreamTracker(id uuid.UUID) *tracker.UpstreamConns {
	group := t.upstreamGroup(id)
	return t.upstreamTrackers[group]
}

// Downstream defines the necessary values for Trafficker to
// forward traffic and loadBalance from a downstream.
type Downstream interface {
	// DownstreamID is the CN from the subject of the clients provided certificate.
	ID() string

	// MaxConnections is the number of connections which will be allowed by rate limiting
	MaxConnections() uint32

	// AllowedServerGroups provides a slice of server groups which the downstream is allowed to connect to.
	// Not used by core library, only used by github.com/jmbarzee/loadbalancer/
	AllowedServerGroups() []string
}

// downstreamConnLimit is a convenience function to lookup the connection limit for a given downstream downstreamID
func (t *Trafficker) downstreamConnLimit(downstreamID string) uint32 {
	return t.downstreams[downstreamID].MaxConnections()
}
