package transport

import "io"

/*
GateWriter wraps an io.Writer so tee traffic can selectively forward bytes
(e.g. only READY wire frames to the pool stream ring) without breaking
io.MultiWriter: when a frame is dropped, Write still returns (len(p), nil).

Transform receives the caller's slice; return nil or empty to drop without
writing to W. Return the same p to forward without copying.
*/
type GateWriter struct {
	W         io.Writer
	Transform func(p []byte) []byte
}

func NewGateWriter(w io.Writer, transform func(p []byte) []byte) *GateWriter {
	return &GateWriter{W: w, Transform: transform}
}

func (gate *GateWriter) Write(p []byte) (n int, err error) {
	if gate == nil || gate.W == nil {
		return 0, io.ErrClosedPipe
	}

	if len(p) == 0 {
		return 0, nil
	}

	if gate.Transform == nil {
		return gate.W.Write(p)
	}

	out := gate.Transform(p)
	if len(out) == 0 {
		return len(p), nil
	}

	nw, err := gate.W.Write(out)
	if err != nil {
		return nw, err
	}

	if nw != len(out) {
		return nw, io.ErrShortWrite
	}

	return len(p), nil
}

func (gate *GateWriter) Read(p []byte) (n int, err error) {
	if gate == nil || gate.W == nil {
		return 0, io.ErrClosedPipe
	}

	if r, ok := gate.W.(io.Reader); ok {
		return r.Read(p)
	}

	return 0, io.EOF
}