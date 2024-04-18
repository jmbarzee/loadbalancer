package proxy

import (
	"errors"
	"io"
	"net"
	"sync"
)

// Bidirectional is used to operate a two-way proxy.
// There is a go routine per direction, calling blocking reads,
// and writing to the other side when bytes are returned.
// When a call to read returns an error, it will attempt to close the writer,
// ensuring that a single connection closing results in both closing.
// Nil is returned instead of EOF errors, as they are used to indicate a closed connection.
func Bidirectional(down, up io.ReadWriteCloser) (toUp, toUpClose, toDown, toDownClose error) {

	/*
		This sync code can appear somewhat confusing at first,
		but can be easily understood with a diagram:


		 Down          proxy.Bidirectional         Up
		=======|      |===================|     |=======
		       |      |                   |     |
		      --->   --->               --->   --->
		      down.Read()                up.Write()
		       |      |  readWriteLoop()  |     |
		       |      |                   |     |
		       |      |                   |     |
		       |      |                   |     |
		    <---    <---                <---   <---
		      down.Write()                up.Read()
		       |      |  readWriteLoop()  |     |
		       |      |                   |     |
		=======|      |===================|     |=======


		The initial goroutine must wait on the results of both
		calls to readWriteLoop. In most cases, one side of the connection
		will be closed, prompting readWriteLoop to call close on the other side,
		which will cause the call to Read() to return, because the net.Conn will close.
		This conveniently causes one readWriteLoop ending to indirectly end the other.
	*/

	wg := &sync.WaitGroup{}
	wg.Add(2)

	var toUpErr, toUpCloseErr, toDownErr, toDownCloseErr error

	go func() {
		toUpErr, toUpCloseErr = readWriteLoop(down, up)
		wg.Done()
	}()
	go func() {
		toDownErr, toDownCloseErr = readWriteLoop(up, down)
		wg.Done()
	}()

	wg.Wait()

	return toUpErr, toUpCloseErr, toDownErr, toDownCloseErr
}

// readWriteLoop is one half of a bidirectional proxy,
// using blocking reads to pull data and blocking writes to push data.
// errors on either writing or reading result in the function returning
func readWriteLoop(r io.Reader, w io.WriteCloser) (writeErr, closeError error) {
	// It may be wise to make a pool of buffers at some point.
	buff := make([]byte, 0xffff)

	for {
		var n int
		n, err := r.Read(buff)
		// breaking convention here, we check the err after writing bytes.
		// From io.Reader godoc:
		// > Callers should always process the n > 0 bytes returned before
		// > considering the error err.
		if n != 0 {
			b := buff[:n]
			// Write returns an error if it doesn't write n bytes.
			// for now we are assuming an error from write indicates
			// that we can no longer write and should exit.
			_, err = w.Write(b)
			if err != nil {
				return err, w.Close()
			}
		}

		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return nil, w.Close()
		}
		if err != nil {
			return err, w.Close()
		}
	}
}
