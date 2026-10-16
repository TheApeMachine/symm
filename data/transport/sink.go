package transport

import "io"

/*
Sink is a workflow component that consumes messages without passing through any data.
It implements a null device pattern, where writes are accepted but discarded, and reads
always return EOF.
*/
type Sink struct {
}

/*
NewSink creates a new Sink instance.

Returns:
  - *Sink: A new Sink instance ready to consume data
*/
func NewSink() *Sink {
	return &Sink{}
}

/*
Read implements io.Reader. It always returns EOF as Sink does not provide any data.

Parameters:
  - p: Byte slice to read data into (unused)

Returns:
  - n: Always 0 as no data is read
  - err: Always io.EOF
*/
func (sink *Sink) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

/*
Write implements io.Writer. It accepts and discards all data.

Parameters:
  - p: Byte slice containing data to be discarded

Returns:
  - n: Number of bytes that would have been written
  - err: Always nil as write always succeeds
*/
func (sink *Sink) Write(p []byte) (n int, err error) {
	return len(p), nil
}

/*
Close implements io.Closer. It's a no-op since Sink maintains no resources.

Returns:
  - error: Always nil as there's nothing to close
*/
func (sink *Sink) Close() error {
	return nil
}