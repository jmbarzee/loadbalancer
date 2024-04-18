package core

import (
	"context"
	"errors"
	"log"
	"net"
	"time"

	"github.com/cloudflare/backoff"
	"github.com/google/uuid"

	"github.com/jmbarzee/loadbalancer/internal/proxy"
	"github.com/jmbarzee/loadbalancer/internal/tracker"
)

type Trafficker struct {
	// downstreams holds
	// read only after start, and safe for concurrent access.
	downstreams map[string]Downstream

	// downstreamTracker keeps track of a downstreams existing connections.
	downstreamTracker *tracker.DownstreamConns

	// upstreams provides a lookup for static upstream data.
	// Read only after start, and safe for concurrent access.
	upstreams map[uuid.UUID]Upstream

	// trackers maps groups to the tracker which counts their connections.
	// It also represents availability and provides the least connections load balancing.
	// Read only after start, and safe for concurrent access.
	upstreamTrackers map[string]*tracker.UpstreamConns

	// upstreamHealth is used to track the health of of an upstream.
	// An upstream's health only determines control flow for health checking mechanisms
	// but does not necessarily represent availability. Availability is handled
	// by upstreamTrackers and is eventually consistent with upstreamHealth.
	upstreamHealth upstreamHealth

	config Config

	logger *log.Logger
}

type Config interface {
	HealthCheckInterval() time.Duration

	RetryAttempts() int
	RetryBackoffInterval() time.Duration
	RetryBackoffMax() time.Duration
}

func NewTrafficker(config Config, downs []Downstream, ups []Upstream, logger *log.Logger) *Trafficker {
	upstreamTrackers := map[string]*tracker.UpstreamConns{}
	upstreamHealth := &upstreamHealth{
		upstreams: make(map[uuid.UUID]bool, len(ups)),
	}
	upstreams := make(map[uuid.UUID]Upstream, len(ups))

	for _, up := range ups {
		id := up.ID()
		upstreams[id] = up
		upstreamHealth.setHealth(id, false)

		group := up.Group()
		upTracker, ok := upstreamTrackers[group]
		if !ok {
			upTracker = tracker.NewUpstreamConns()
			upstreamTrackers[group] = upTracker
		}
		upTracker.AddUpstream(id)
	}

	downstreams := make(map[string]Downstream, len(downs))
	for _, down := range downs {
		downstreams[down.ID()] = down
	}

	return &Trafficker{
		downstreamTracker: tracker.NewDownstreamConns(),
		upstreamTrackers:  upstreamTrackers,
		config:            config,
		logger:            logger,
	}
}

// Start begins the supporting routines of the Trafficker
func (t *Trafficker) Start(ctx context.Context) error {
	go t.routineHealthChecks(ctx)
	return nil
}

// Handle is the entry point for a connection which should be rate limited and load balanced.
// Handle addresses closing downstreamConn, as well an upstreamConn if allowed.
func (t *Trafficker) Handle(downstreamID string, upstreamGroup string, downstreamConn net.Conn) {
	connLimit := t.downstreamConnLimit(downstreamID)
	allowed := t.downstreamTracker.TryRecordConnection(downstreamID, connLimit)
	if !allowed {
		t.logger.Printf("Rate limiting downstream(%v)\n", downstreamID)
		if err := downstreamConn.Close(); err != nil {
			t.logger.Printf("Error while closing downstream(%v): %v\n", downstreamID, err)
		}
		return
	}

	// Now that we recorded a downstream connection, we must make sure to record it ending
	defer t.downstreamTracker.ConnectionEnded(downstreamID)

	upstreamID, err := t.upstreamTrackers[upstreamGroup].NextAvailableUpstream()
	if err != nil {
		t.logger.Printf("No available upstream in group(%v) for downstream(%v): %v\n", upstreamGroup, downstreamID, err)
		if err := downstreamConn.Close(); err != nil {
			t.logger.Printf("Error while closing downstream(%v): %v\n", downstreamID, err)
		}

		// We could backoff/retry here, but its simpler to leave that to the downstream.
		// This may not be ideal since it the demonstrated behavior will be analogous
		// to the behavior of being rate limited. A longer period before the connection is
		// closed would help differentiate them.
		return
	}

	// Now that we recorded an upstream connection, we must make sure to record it ending
	defer t.upstreamTracker(upstreamID).ConnectionEnded(upstreamID)

	upstreamAddr := t.upstreamAddr(upstreamID)
	upstreamConn, err := t.dialRetryBackoff(upstreamAddr)
	if err != nil {
		t.logger.Printf("Failed to connect  downstream(%v) with chosen upstream(%v): %v\n", downstreamID, upstreamID, err)
		if err := downstreamConn.Close(); err != nil {
			t.logger.Printf("Error while closing downstream(%v): %v\n", downstreamID, err)
		}

		t.upstreamTracker(upstreamID).UpstreamUnavailable(upstreamID)
		t.logger.Printf("Health check: upstream(%v) failed to connect and will be removed from availability\n", upstreamID)

		// Instead of returning here, we could try to select another upstream.
		// Considering that we failed connecting to an upstream which we thought was healthy
		// we are already in a bad state; A little bit of back pressure on the downstream
		// is acceptable for now.
		return
	}

	// There are now two connections which will be managed by Bidirectional, and then closed
	toUpErr, toUpCloseErr, toDownErr, toDownCloseErr := proxy.Bidirectional(downstreamConn, upstreamConn)

	if toUpErr != nil && toDownErr != nil {
		// both errored
		t.logger.Printf("Error while proxying both directions, upstream(%v) <-> downstream(%v)\n\t\ttoUpErr: %v\n\t\ttoDownErr: %v\n", upstreamID, downstreamID, toUpErr, toDownErr)
		// We could mark the upstream unhealthy here.
		// Decision should be made based on what types
		// of errors are logged and if they indicate upstream misbehavior.
	} else if toUpErr != nil && toDownErr == nil {
		// toUp errored
		t.logger.Printf("Error while proxying to upstream(%v) from downstream(%v): %v\n", upstreamID, downstreamID, toUpErr)
		// We could mark the upstream unhealthy here.
		// Decision should be made based on what types
		// of errors are logged and if they indicate upstream misbehavior.
	} else if toUpErr == nil && toDownErr != nil {
		// toDown errored
		t.logger.Printf("Error while proxying from upstream(%v) to downstream(%v): %v\n", upstreamID, downstreamID, toDownErr)
	} else {
		// No RW errors, check CloseErrs
		if toUpCloseErr != nil && toDownCloseErr != nil {
			// both errored
			t.logger.Printf("Error while closing both directions, upstream(%v) <-> downstream(%v)\n\t\ttoUpCloseErr: %v\n\t\ttoDownCloseErr: %v\n", upstreamID, downstreamID, toUpCloseErr, toDownCloseErr)

		} else if toUpCloseErr != nil && toDownCloseErr == nil {
			// toUp errored
			if !errors.Is(toUpCloseErr, net.ErrClosed) {
				// error was not just a closed connection (we expect at least one in nearly all cases)
				t.logger.Printf("Error while closing upstream(%v) from downstream(%v): %v\n", upstreamID, downstreamID, toUpCloseErr)
			}
		} else if toUpCloseErr == nil && toDownCloseErr != nil {
			// toDown errored
			if !errors.Is(toDownCloseErr, net.ErrClosed) {
				// error was not just a closed connection (we expect at least one in nearly all cases)
				t.logger.Printf("Error while closing downstream(%v) from  upstream(%v): %v\n", downstreamID, upstreamID, toDownCloseErr)
			}
		}
	}

	// record connections ended (handled by defers)
}

var errorAllAttemptsFailed = errors.New("all DialTCP attempts failed")

// dialRetryBackoff will dial with retry and exponential backoff.
// It will either return the connected *net.TCPConn or errorAllAttemptsFailed.
// At some point, it would be great to let upstreams configure how they are dialed.
// This would allow for more configuration through the library API and unit testing.
func (t *Trafficker) dialRetryBackoff(addr *net.TCPAddr) (upstreamConn *net.TCPConn, err error) {
	expBackoff := backoff.New(t.config.RetryBackoffInterval(), t.config.RetryBackoffMax())
	attempts := t.config.RetryAttempts()
	for i := 0; i < attempts; i++ {

		upstreamConn, err = net.DialTCP(addr.Network(), nil, addr)
		if err == nil {
			return upstreamConn, nil
		}
		t.logger.Printf("Error while dialing addr(%v)\n", addr.String())

		// wait for prescribed backoff
		t := time.NewTimer(expBackoff.Duration())
		<-t.C
	}

	return nil, errorAllAttemptsFailed
}
