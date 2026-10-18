package transport

import (
	"bufio"
	"io"
)

const (
	copyBufferSize = 1024 * 1024
	maxFrameSize   = 1024 * 1024
)

func Copy(destination io.Writer, origin io.Reader) (n int, err error) {
	defer func() {
		stream, ok := destination.(*Stream)

		if ok && err == nil {
			err = stream.Flush()
		}
	}()

	if writerTo, ok := origin.(io.WriterTo); ok {
		nn, err := writerTo.WriteTo(destination)

		if err != nil {
			return int(nn), err
		}

		return int(nn), nil
	}

	scanner := bufio.NewScanner(origin)

	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if !atEOF || len(data) == 0 {
			return 0, nil, nil
		}
		return len(data), data, nil
	})

	scanner.Buffer(make([]byte, copyBufferSize), maxFrameSize)

	if !scanner.Scan() {
		if err = scanner.Err(); err != nil {
			return 0, err
		}
		return 0, io.EOF
	}

	return destination.Write(scanner.Bytes())
}
