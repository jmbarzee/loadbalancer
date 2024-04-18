package tracker

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestUpstreamConnsCounts(t *testing.T) {
	upstream1 := uuid.New()
	upstream2 := uuid.New()

	tests := []struct {
		name              string
		op                func(*UpstreamConns)
		expectedUpstreams map[uuid.UUID]*upstream
		// expectedPQ is only checked against if it is non-nil
		expectedPQ *upstreamPQ
	}{
		{
			name: "multiple routines requesting connections",
			op: func(tracker *UpstreamConns) {
				tracker.UpstreamAvailable(upstream1)
				tracker.UpstreamAvailable(upstream2)

				wg := sync.WaitGroup{}
				wg.Add(2)

				// this test does not guarantee that concurrent access works,
				// because the goroutine scheduler will typically run routines till
				// till they cannot be run further.
				// but it was quick to write, and does ensure that separate goroutines
				// can begin connections independently while balancing connections to upstreams

				// spin up 2 goroutines which begin 5 connections each
				for i := 0; i < 2; i++ {
					go func() {
						for j := 0; j < 5; j++ {
							_, err := tracker.NextAvailableUpstream()
							if err != nil {
								t.Errorf("unexpected error: %v\n", err)
							}
						}
						wg.Done()
					}()
				}
				wg.Wait()
			},
			expectedUpstreams: map[uuid.UUID]*upstream{
				upstream1: {
					id:        upstream1,
					connCount: 5,
				},
				upstream2: {
					id:        upstream2,
					connCount: 5,
				},
			},
		},
		{
			name: "return errors when there are no available upstreams",
			op: func(tracker *UpstreamConns) {
				_, err := tracker.NextAvailableUpstream()
				if !errors.Is(err, errorNoAvailableUpstream) {
					t.Errorf("expected error %v, but got nil\n", errorNoAvailableUpstream)
				}
				tracker.UpstreamAvailable(upstream1)

				_, err = tracker.NextAvailableUpstream()
				if err != nil {
					t.Errorf("unexpected error: %v\n", err)
				}
			},
			expectedUpstreams: map[uuid.UUID]*upstream{
				upstream1: {
					id:        upstream1,
					connCount: 1,
				},
				upstream2: {
					id: upstream2,
				},
			},
			expectedPQ: &upstreamPQ{
				{
					id:        upstream1,
					connCount: 1,
					index:     0,
				},
			},
		},
		{
			name: "only allow healthy upstreams to get new connections",
			op: func(tracker *UpstreamConns) {
				tracker.UpstreamAvailable(upstream1)
				tracker.UpstreamAvailable(upstream2)
				_, err := tracker.NextAvailableUpstream()
				failIfNotNil(t, err)
				_, err = tracker.NextAvailableUpstream()
				failIfNotNil(t, err)

				tracker.UpstreamUnavailable(upstream1)
				_, err = tracker.NextAvailableUpstream()
				failIfNotNil(t, err)
				_, err = tracker.NextAvailableUpstream()
				failIfNotNil(t, err)

				tracker.UpstreamAvailable(upstream1)
				_, err = tracker.NextAvailableUpstream()
				failIfNotNil(t, err)
			},
			expectedUpstreams: map[uuid.UUID]*upstream{
				upstream1: {
					id:        upstream1,
					connCount: 2,
				},
				upstream2: {
					id:        upstream2,
					connCount: 3,
				},
			},
			expectedPQ: &upstreamPQ{
				{
					id:        upstream1,
					connCount: 2,
					index:     0,
				},
				{
					id:        upstream2,
					connCount: 3,
					index:     1,
				},
			},
		},
	}

	for i, test := range tests {
		tracker := NewUpstreamConns()
		tracker.AddUpstream(upstream1)
		tracker.AddUpstream(upstream2)
		test.op(tracker)
		actualUpstreams := tracker.upstreams
		for id, actualUpstream := range actualUpstreams {
			expectedUpstream := test.expectedUpstreams[id]
			if expectedUpstream.connCount != actualUpstream.connCount {
				t.Errorf("test(%v) expectedCounts did not match actualCounts: \n %v != %v\n", i, expectedUpstream.connCount, actualUpstream.connCount)
			}
		}

		actualPQ := tracker.pq
		if test.expectedPQ != nil && !reflect.DeepEqual(test.expectedPQ, actualPQ) {
			t.Errorf("test(%v) expectedPQ did not match actualPQ: \n %v != %v\n", i, test.expectedPQ, actualPQ)
		}
	}
}

func failIfNotNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v\n", err)
	}
}
