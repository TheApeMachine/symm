package network

import (
	"context"
	"io"
	"sync"
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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	transports   map[UniConnType]ManagedTransport
	activeType   UniConnType
	active       ManagedTransport
	sources      io.Writer
	destinations io.Reader
	readyErr     error
	ready        sync.Once
}

/*
uniConnOption configures a UniConn at construction time.
*/
type uniConnOption func(*UniConn)

/*
NewUniConn constructs a UniConn. Without options it has no transport;
pass UniConnWithIPC, UniConnWithUDP, or UniConnWithQUIC to wire one up.
*/
func NewUniConn(ctx context.Context, opts ...uniConnOption) *UniConn {
	ctx, cancel := context.WithCancel(ctx)

	conn := &UniConn{
		ctx:        ctx,
		cancel:     cancel,
		transports: make(map[UniConnType]ManagedTransport),
	}

	for _, opt := range opts {
		opt(conn)
	}

	return conn
}

/*
Read delegates to the underlying transport.
*/
func (conn *UniConn) Read(p []byte) (int, error) {
	if err := conn.ensureReady(); err != nil {
		return 0, err
	}

	return conn.destinations.Read(p)
}

/*
Write delegates to the underlying transport.
*/
func (conn *UniConn) Write(p []byte) (int, error) {
	if err := conn.ensureReady(); err != nil {
		return 0, err
	}

	return conn.sources.Write(p)
}

/*
Close tears down the connection context and the underlying transport.
*/
func (conn *UniConn) Close() error {
	conn.cancel()

	if len(conn.transports) == 0 {
		return nil
	}

	var firstErr error
	for _, transport := range conn.transports {
		if transport == nil {
			continue
		}

		if err := transport.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

/*
Ready blocks until the configured transport reaches a protocol-specific
ready state. For IPC/UDP this is immediate; QUIC listener mode accepts and
primes the first stream under the hood.
*/
func (conn *UniConn) Ready() error {
	return conn.ensureReady()
}

func (conn *UniConn) ensureReady() error {
	if conn.active == nil || conn.sources == nil || conn.destinations == nil {
		return NewNetworkError("transport", ErrNoTransport, "ensureReady")
	}

	conn.ready.Do(func() {
		if ready, ok := conn.active.(ReadyTransport); ok {
			conn.readyErr = ready.Ready(conn.ctx)
		}
	})

	return conn.readyErr
}

/*
Traits reports the semantic properties of the active transport.
*/
func (conn *UniConn) Traits() TransportTraits {
	if conn.active == nil {
		return TransportTraits{}
	}

	return conn.active.Traits()
}

/*
Status reports the current health snapshot of the active transport.
*/
func (conn *UniConn) Status() TransportStatus {
	if conn.active == nil {
		return TransportStatus{
			LastFailureMode: TransportFailureNotReady,
			LastFailure:     ErrNoTransport,
			SystemicFailure: true,
			Degraded:        true,
			Breaker:         CircuitOpen,
		}
	}

	return conn.active.Status()
}

/*
UniConnWithContext replaces the default background context.
*/
func UniConnWithContext(ctx context.Context) uniConnOption {
	return func(conn *UniConn) {
		conn.cancel()
		conn.ctx, conn.cancel = context.WithCancel(ctx)
	}
}

/*
UniConnWithTransport registers a transport in the UniConn map. The first
non-nil registration also becomes the active transport (Read/Write/Ready use
it). Later registrations stay available but do not replace active until
SetActiveTransport or UniConnWithActiveTransport selects them—call those after
all transports are registered when you need a specific layer (e.g. QUIC) to win.
*/
func UniConnWithTransport(
	transportType UniConnType, transport ManagedTransport,
) uniConnOption {
	return func(conn *UniConn) {
		if transport == nil {
			return
		}

		conn.transports[transportType] = transport

		if conn.active != nil {
			return
		}

		conn.activeType = transportType
		conn.active = transport
		conn.sources = transport
		conn.destinations = transport
	}
}

/*
SetActiveTransport selects which registered transport handles I/O and Ready.
Resets lazy readiness so the next Ready re-runs against the new active peer.
Call after registration (e.g. after all UniConnWithTransport options).
*/
func (conn *UniConn) SetActiveTransport(transportType UniConnType) error {
	transport := conn.transports[transportType]
	if transport == nil {
		return ErrNoTransport
	}

	conn.activeType = transportType
	conn.active = transport
	conn.sources = transport
	conn.destinations = transport
	conn.ready = sync.Once{}
	conn.readyErr = nil

	return nil
}

/*
UniConnWithActiveTransport applies SetActiveTransport from an option. Use as
the last NewUniConn option after every UniConnWithTransport so the map is
complete; if transportType was never registered, active is left unchanged
(options cannot surface errors—call SetActiveTransport yourself when you
need to handle that failure).
*/
func UniConnWithActiveTransport(transportType UniConnType) uniConnOption {
	return func(conn *UniConn) {
		if err := conn.SetActiveTransport(transportType); err != nil {
			return
		}
	}
}

/*
UniConnError is a typed error for UniConn failures.
*/
type UniConnError string

const (
	ErrNoTransport UniConnError = "uniconn: no transport configured"
)

/*
Error implements the error interface for UniConnError.
*/
func (connErr UniConnError) Error() string {
	return string(connErr)
}
