package tracker

import (
	"container/heap"
	"errors"
	"sync"

	"github.com/google/uuid"
)

var errorNoAvailableUpstream = errors.New("No Available Upstream")

// UpstreamConns tracks connections for an upstreamGroup
// on a per upstream basis. Upstreams can be marked as
// unhealthy to prevent them from being chosen for new connections.
// UpstreamConns handles load balancing through BeginConnection()
type UpstreamConns struct {
	// mu protects the resources of UpstreamConns
	mu sync.Mutex

	// upstreams holds all upstreams, healthy or unhealthy
	upstreams map[uuid.UUID]*upstream

	// pq holds healthy upstreams and provides the means to
	// pick the upstream with the least connections.
	pq *upstreamPQ
}

// An upstream stores a count of connections
// as well as some overhead for use in upstreamPQ.
type upstream struct {
	// id is the id of the upstream
	id uuid.UUID

	// The count of connections to the upstream.
	// Also the priority of an upstream, lowest first.
	connCount uint32

	// The index is needed by update and is maintained by the heap.Interface methods.
	// if an upstream is pulled from the upstreamPQ (because of health)
	// its index will be set to -1
	index int
}

// NewUpstreamConns creates a new UpstreamConns
// with upstreams based on provided upstreamIDs.
// upstreams must be marked as healthy before they will be
// added to the internal priorityQueue and available for BeginConnection()
func NewUpstreamConns(upstreamIDs []uuid.UUID) *UpstreamConns {
	upstreams := make(map[uuid.UUID]*upstream, len(upstreamIDs))
	for _, id := range upstreamIDs {
		upstreams[id] = &upstream{
			id:    id,
			index: -1,
		}
	}
	return &UpstreamConns{
		upstreams: upstreams,
		pq:        &upstreamPQ{},
	}
}

// NextAvailableUpstream returns the UUID of the upstream with the least connections
// and records the additional connection.
// An error is returned if there are no available upstreams
func (t *UpstreamConns) NextAvailableUpstream() (uuid.UUID, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	upstream := t.pq.peek()
	if upstream == nil {
		return uuid.UUID{}, errorNoAvailableUpstream
	}

	// do we need a check for an upstream which is not in the upstreamPQ?
	// The assumption is that we are only incrementing upstreams which are
	// healthy and in the upstreamPQ. unhealthy upstreams are removed from the upstreamPQ.
	upstream.connCount++
	heap.Fix(t.pq, upstream.index)
	return upstream.id, nil
}

// ConnectionEnded takes the UUID of the upstream which has just had a connection terminate
// and records the ended connection.
// ConnectionEnded requires that a connection was begun previously,
// otherwise it may access a key which does not exist and panic from nil pointer dereference
func (t *UpstreamConns) ConnectionEnded(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	upstream := t.upstreams[id]
	upstream.connCount--

	if upstream.index < 0 {
		// upstream is not in the upstreamPQ
		return
	}

	heap.Fix(t.pq, upstream.index)
}

// UpstreamUnavailable is used to remove an upstream from the available upstreams
// UpstreamUnavailable requires that the given id was provided to NewUpstreamConns(),
// otherwise it may access a key which does not exist and panic from nil pointer dereference
func (t *UpstreamConns) UpstreamUnavailable(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	upstream := t.upstreams[id]

	if upstream.index < 0 {
		// upstream is not in the upstreamPQ
		// generally should not be likely, but possible
		return
	}

	t.pq.remove(upstream)
}

// UpstreamAvailable is used to restore an upstream to the available upstreams
// UpstreamAvailable requires that the given id was provided to NewUpstreamConns(),
// otherwise it may access a key which does not exist and panic from nil pointer dereference
func (t *UpstreamConns) UpstreamAvailable(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	upstream := t.upstreams[id]

	if upstream.index > -1 {
		// upstream is in the upstreamPQ
		// generally should not be likely, but possible
		return
	}

	heap.Push(t.pq, upstream)
}

// A upstreamPQ implements heap.Interface and holds upstreams.
type upstreamPQ []*upstream

var _ heap.Interface = (*upstreamPQ)(nil)

func (pq upstreamPQ) Len() int { return len(pq) }

func (pq upstreamPQ) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].connCount < pq[j].connCount
}

func (pq upstreamPQ) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *upstreamPQ) Push(x any) {
	n := len(*pq)
	item := x.(*upstream)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *upstreamPQ) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// peek returns the upstream at the front of the upstreamPQ without altering or moving it
func (pq *upstreamPQ) peek() *upstream {
	if len(*pq) == 0 {
		return nil
	}
	return (*pq)[0]
}

// remove pulls an upstream from the upstreamPQ.
// remove assumes that up is in the upstreamPQ
func (pq *upstreamPQ) remove(up *upstream) {
	if len(*pq) == 1 {
		// up is the only item in the upstreamPQ
		pq.Pop()
		return
	}

	i := up.index
	j := len(*pq) - 1
	if i == j {
		// up is the last item in the upstreamPQ
		pq.Pop()
		return
	}

	// up can be swapped with the last item in the upstreamPQ (then fixed)
	pq.Swap(i, j)
	pq.Pop()
	heap.Fix(pq, i)
}
