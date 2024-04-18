package proxy

import (
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
)

// bidirectionalPipeEnd is one end of a bidirectionalPipe
type bidirectionalPipeEnd struct {
	io.Reader
	io.WriteCloser
}

var _ io.ReadWriteCloser = (*bidirectionalPipeEnd)(nil)

// newBidirectionalPipe returns pipes configured to emulate a network connection
func newBidirectionalPipe() (bidirectionalPipeEnd, bidirectionalPipeEnd) {
	aReader, aWriter := io.Pipe()
	bReader, bWriter := io.Pipe()
	return bidirectionalPipeEnd{
			Reader:      aReader,
			WriteCloser: bWriter,
		}, bidirectionalPipeEnd{
			Reader:      bReader,
			WriteCloser: aWriter,
		}
}

func TestBidirectional(t *testing.T) {
	tests := []struct {
		name                   string
		op                     func(t *testing.T, down, up io.ReadWriteCloser)
		expectedToUpErr        error
		expectedToUpCloseErr   error
		expectedToDownErr      error
		expectedToDownCloseErr error
	}{
		{
			name: "test close both",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {

				// Close down
				err := down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err := up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
		{
			name: "test write down, read up",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {
				testData := []byte("this should pass through the proxy")

				// Write to down
				n, err := down.Write(testData)
				if n != len(testData) {
					t.Errorf("failed to write all bytes to down")
				}
				if err != nil {
					t.Errorf("got error while writing to down: %v", err)
				}

				// Read from up
				recvBuff := make([]byte, len(testData))
				n, err = up.Read(recvBuff)
				if n != len(testData) {
					t.Errorf("failed to read all bytes from up")
				}
				if err != nil {
					t.Errorf("got error while reading from up: %v", err)
				}
				if !reflect.DeepEqual(testData, recvBuff) {
					t.Errorf("bytes passed through did not match")
				}

				// Close down
				err = down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err = up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
		{
			name: "test write up, read down",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {
				testData := []byte("this should pass through the proxy")

				// Write to up
				n, err := up.Write(testData)
				if n != len(testData) {
					t.Errorf("failed to write all bytes to up")
				}
				if err != nil {
					t.Errorf("got error while writing to up: %v", err)
				}

				// Read from down
				recvBuff := make([]byte, len(testData))
				n, err = down.Read(recvBuff)
				if n != len(testData) {
					t.Errorf("failed to read all bytes from down")
				}
				if err != nil {
					t.Errorf("got error while reading from down: %v", err)
				}
				if !reflect.DeepEqual(testData, recvBuff) {
					t.Errorf("bytes passed through did not match")
				}

				// Close down
				err = down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err = up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
		{
			name: "test write both, read both",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {
				testData1 := []byte("this should pass through the proxy - 1")
				testData2 := []byte("this should pass through the proxy - 2")

				// Write to up
				n, err := up.Write(testData1)
				if n != len(testData1) {
					t.Errorf("failed to write all bytes to up")
				}
				if err != nil {
					t.Errorf("got error while writing to up: %v", err)
				}

				// Write to down
				n, err = down.Write(testData2)
				if n != len(testData2) {
					t.Errorf("failed to write all bytes to down")
				}
				if err != nil {
					t.Errorf("got error while writing to down: %v", err)
				}

				// Read from up
				recvBuff := make([]byte, len(testData2))
				n, err = up.Read(recvBuff)
				if n != len(testData2) {
					t.Errorf("failed to read all bytes from up")
				}
				if err != nil {
					t.Errorf("got error while reading from up: %v", err)
				}
				if !reflect.DeepEqual(testData2, recvBuff) {
					t.Errorf("bytes passed through did not match")
				}

				// Read from down
				recvBuff = make([]byte, len(testData1))
				n, err = down.Read(recvBuff)
				if n != len(testData1) {
					t.Errorf("failed to read all bytes from down")
				}
				if err != nil {
					t.Errorf("got error while reading from down: %v", err)
				}
				if !reflect.DeepEqual(testData1, recvBuff) {
					t.Errorf("bytes passed through did not match")
				}

				// Close down
				err = down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err = up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
		{
			name: "test write both, read both, write both, read both",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {
				for i := 0; i < 2; i++ {
					testData1 := []byte("this should pass through the proxy - 1")
					testData2 := []byte("this should pass through the proxy - 2")

					// Write to up
					n, err := up.Write(testData1)
					if n != len(testData1) {
						t.Errorf("failed to write all bytes to up")
					}
					if err != nil {
						t.Errorf("got error while writing to up: %v", err)
					}

					// Write to down
					n, err = down.Write(testData2)
					if n != len(testData2) {
						t.Errorf("failed to write all bytes to down")
					}
					if err != nil {
						t.Errorf("got error while writing to down: %v", err)
					}

					// Read from up
					recvBuff := make([]byte, len(testData2))
					n, err = up.Read(recvBuff)
					if n != len(testData2) {
						t.Errorf("failed to read all bytes from up")
					}
					if err != nil {
						t.Errorf("got error while reading from up: %v", err)
					}
					if !reflect.DeepEqual(testData2, recvBuff) {
						t.Errorf("bytes passed through did not match")
					}

					// Read from down
					recvBuff = make([]byte, len(testData1))
					n, err = down.Read(recvBuff)
					if n != len(testData1) {
						t.Errorf("failed to read all bytes from down")
					}
					if err != nil {
						t.Errorf("got error while reading from down: %v", err)
					}
					if !reflect.DeepEqual(testData1, recvBuff) {
						t.Errorf("bytes passed through did not match")
					}
				}

				// Close down
				err := down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err := up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
		{
			name: "test close both",
			op: func(t *testing.T, down, up io.ReadWriteCloser) {

				// Close down
				err := down.Close()
				if err != nil {
					t.Errorf("failed to close down side of proxy.Bidirectional")
				}
				// Check up is closed
				n, err := up.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}

				// Close up
				err = up.Close()
				if err != nil {
					t.Errorf("failed to close up side of proxy.Bidirectional")
				}
				// Check down is closed
				n, err = down.Read(make([]byte, 1))
				if n != 0 {
					t.Errorf("read bytes from up, should have been closed")
				}
				if !errors.Is(err, io.EOF) {
					t.Errorf("didn't get EOF from up, got %v", err)
				}
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wg := &sync.WaitGroup{}
			wg.Add(1)

			var toUpErr, toUpCloseErr, toDownErr, toDownCloseErr error

			downRemote, downLocal := newBidirectionalPipe()
			upLocal, upRemote := newBidirectionalPipe()

			// Pass the local ends to the proxy
			go func() {
				toUpErr, toUpCloseErr, toDownErr, toDownCloseErr = Bidirectional(downLocal, upLocal)
				wg.Done()
			}()

			// Test the remote ends
			test.op(t, downRemote, upRemote)

			// Test that proxy.Bidirectional returns.
			// Also ensures that underlying go routines have concluded too.
			wg.Wait()

			// Check the errors
			if !errors.Is(toUpErr, test.expectedToUpErr) {
				t.Errorf("test(%v) actual toUpErr did not match expected ToUpErr: \n %v != %v\n", i, toUpErr, test.expectedToUpErr)
			}
			if !errors.Is(toUpCloseErr, test.expectedToUpCloseErr) {
				t.Errorf("test(%v) actual toUpCloseErr did not match expected ToUpCloseErr: \n %v != %v\n", i, toUpCloseErr, test.expectedToUpCloseErr)
			}
			if !errors.Is(toDownErr, test.expectedToDownErr) {
				t.Errorf("test(%v) actual toDownErr did not match expected ToDownErr: \n %v != %v\n", i, toDownErr, test.expectedToDownErr)
			}
			if !errors.Is(toDownCloseErr, test.expectedToDownCloseErr) {
				t.Errorf("test(%v) actual toDownCloseErr did not match expected ToDownCloseErr: \n %v != %v\n", i, toDownCloseErr, test.expectedToDownCloseErr)
			}
		})
	}
}
