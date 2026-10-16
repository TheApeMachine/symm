package transport

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

const copyBufferSize = 32 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, copyBufferSize)
	},
}

func Copy(destination io.Writer, origin io.Reader) (n int64, err error) {
	defer func() {
		if stream, ok := destination.(*Stream); ok {
			if flushErr := stream.Flush(); flushErr != nil && err == nil {
				err = flushErr
			}
		}
	}()

	frame, err := readFrame(origin)

	if err != nil {
		return 0, err
	}

	if len(frame) == 0 {
		return 0, io.EOF
	}

	written, err := destination.Write(frame)

	return int64(written), err
}

func readFrame(origin io.Reader) ([]byte, error) {
	size := copyBufferSize

	for {
		frame, err := readFrameWithBuffer(origin, size)

		if errors.Is(err, io.ErrShortBuffer) {
			size *= 2

			continue
		}

		return frame, err
	}
}

func readFrameWithBuffer(origin io.Reader, size int) ([]byte, error) {
	var frame bytes.Buffer
	buffer := copyBufferPool.Get().([]byte)
	if cap(buffer) < size {
		buffer = make([]byte, size)
	}
	buffer = buffer[:size]
	defer copyBufferPool.Put(buffer[:copyBufferSize])

	for {
		read, err := origin.Read(buffer)

		if errors.Is(err, io.ErrShortBuffer) {
			return nil, err
		}

		if read > 0 {
			frame.Write(buffer[:read])
		}

		if err == nil {
			continue
		}

		if errors.Is(err, io.EOF) {
			return frame.Bytes(), nil
		}

		return frame.Bytes(), err
	}
}
