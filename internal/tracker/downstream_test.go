package tracker

import (
	"reflect"
	"sync"
	"testing"
)

func TestDownstreamConnsCounts(t *testing.T) {
	downstream1 := "downstream1"
	downstream2 := "downstream2"

	tests := []struct {
		name           string
		op             func(*DownstreamConns)
		expectedCounts map[string]uint32
	}{
		{
			name: "record a new downstream connection if under maximum",
			op: func(tracker *DownstreamConns) {
				tracker.TryRecordConnection(downstream1, 10)
			},
			expectedCounts: map[string]uint32{
				downstream1: 1,
			},
		},
		{
			name: "record a connection ending",
			op: func(tracker *DownstreamConns) {
				tracker.TryRecordConnection(downstream1, 10)
				tracker.ConnectionEnded(downstream1)
			},
			expectedCounts: map[string]uint32{
				downstream1: 0,
			},
		},
		{
			name: "don't record connections which would extend beyond maximum",
			op: func(tracker *DownstreamConns) {
				tracker.TryRecordConnection(downstream1, 10)
				tracker.TryRecordConnection(downstream1, 10)
				tracker.TryRecordConnection(downstream2, 2)
				tracker.TryRecordConnection(downstream2, 2)

				// this connection should not be recorded because of the maximums
				tracker.TryRecordConnection(downstream2, 2)
				tracker.TryRecordConnection(downstream1, 10)

				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream2)
				tracker.ConnectionEnded(downstream2)

				tracker.TryRecordConnection(downstream2, 2)
			},
			expectedCounts: map[string]uint32{
				downstream1: 0,
				downstream2: 1,
			},
		},
		{
			name: "don't record connections which would extend beyond maximum, concurrently",
			op: func(tracker *DownstreamConns) {
				wg := sync.WaitGroup{}
				wg.Add(2)

				// this test does not guarantee that concurrent access works,
				// because the goroutine scheduler will typically run routines till
				// till they cannot be run further.
				// but it was quick to write, and does ensure that separate goroutines
				// can end connections which they didn't begin.
				go func() {
					tracker.TryRecordConnection(downstream2, 2)
					tracker.TryRecordConnection(downstream1, 10)
					wg.Done()
				}()
				go func() {
					tracker.TryRecordConnection(downstream1, 10)
					tracker.TryRecordConnection(downstream1, 10)
					tracker.TryRecordConnection(downstream2, 2)
					tracker.TryRecordConnection(downstream2, 2)
					wg.Done()
				}()

				wg.Wait()
				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream1)
				tracker.ConnectionEnded(downstream2)
				tracker.ConnectionEnded(downstream2)
				tracker.TryRecordConnection(downstream2, 2)
			},
			expectedCounts: map[string]uint32{
				downstream1: 0,
				downstream2: 1,
			},
		},
	}

	for i, test := range tests {
		tracker := NewDownstreamConns()
		test.op(tracker)
		actualCounts := tracker.connCounts
		if !reflect.DeepEqual(test.expectedCounts, actualCounts) {
			t.Errorf("test(%v) expectedCounts did not match actualCounts: \n %v != %v\n", i, test.expectedCounts, actualCounts)
		}
	}
}
