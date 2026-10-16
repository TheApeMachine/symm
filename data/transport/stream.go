package transport

import (
	"bytes"
	"context"
	"io"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Stream buffers writes to an io.ReadWriteCloser until Flush.
*/
type Stream struct {
	*runtime.System
	writer *bytes.Buffer
	reader *bytes.Reader
	fn     func(*data.Value) *data.Value
}

/*
NewStream wraps rwc for use in Copy, FlipFlop, Pipeline, and Number.
*/
func NewStream(ctx context.Context, fn func(*data.Value) *data.Value) *Stream {
	return &Stream{
		System: runtime.NewSystem(ctx, "stream"),
		writer: bytes.NewBuffer([]byte{}),
		fn:     fn,
	}
}

func (stream *Stream) Read(p []byte) (n int, err error) {
	if stream.reader == nil {
		var value *data.Value
		if stream.writer.Len() > 0 {
			value = data.From(stream.writer.Bytes())
			stream.writer.Reset()
		}

		if stream.fn != nil {
			value = stream.fn(value)
		}

		if value == nil {
			return 0, io.EOF
		}

		stream.reader = bytes.NewReader(value.Bytes())
	}

	n, err = stream.reader.Read(p)
	if stream.reader.Len() == 0 {
		stream.reader = nil
		if err == nil {
			err = io.EOF
		}
	}

	return n, err
}

func (stream *Stream) Write(p []byte) (n int, err error) {
	return stream.writer.Write(p)
}

/*
Flush delivers buffered writes to the underlying ReadWriteCloser.
Copy defers this so capnp frames arrive in one Write to destinations that need it.
*/
func (stream *Stream) Flush() error {
	if stream.writer.Len() > 0 {
		if _, err := stream.Write(stream.writer.Bytes()); err != nil {
			return err
		}

		stream.writer.Reset()
	}

	return nil
}

func (stream *Stream) Close() error {
	if err := stream.Flush(); err != nil {
		return err
	}

	return nil
}
