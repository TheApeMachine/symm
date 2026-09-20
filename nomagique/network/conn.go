package network

import (
	"context"
	"io"

	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
UniConnType selects the transport layer backing a UniConn.
*/
type UniConnType uint

const (
	IPCType UniConnType = iota
	UDPType
	QUICType
)

/*
UniConn is a unified network connection that delegates to a concrete
transport (IPC shared memory, UDP multicast, or QUIC) selected at
construction time. It implements io.ReadWriteCloser so that Value
frames flow through the same interface regardless of the underlying wire.
*/
type UniConn struct {
	*runtime.System
	transports   map[UniConnType]io.ReadWriteCloser
	activeType   UniConnType
	active       io.ReadWriteCloser
	sources      io.Writer
	destinations io.Reader
}

/*
NewUniConn constructs a UniConn. Without options it has no transport;
pass UniConnWithIPC, UniConnWithUDP, or UniConnWithQUIC to wire one up.
*/
func NewUniConn(ctx context.Context) *UniConn {
	conn := &UniConn{
		transports: make(map[UniConnType]io.ReadWriteCloser),
	}

	conn.System = runtime.NewSystem(ctx, "conn", conn)
	return conn
}

/*
Read delegates to the underlying transport.
*/
func (conn *UniConn) Read(p []byte) (int, error) {
	if conn.Status() != runtime.READY {
		return 0, nil
	}

	return conn.destinations.Read(p)
}

/*
Write delegates to the underlying transport.
*/
func (conn *UniConn) Write(p []byte) (int, error) {
	if conn.Status() != runtime.READY {
		return 0, nil
	}

	return conn.sources.Write(p)
}
