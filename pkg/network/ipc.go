package network

import (
	"context"
	"net"
	"os"
)

// IPC implements ManagedTransport using Unix domain sockets.
type IPC struct {
	addr     string
	listener *net.UnixListener
	conn     *net.UnixConn
	isServer bool
	status   TransportStatus
	traits   TransportTraits
}

// IPCWithListen creates an IPC listener at the given path.
func IPCWithListen(ctx context.Context, path string) (*IPC, error) {
	os.Remove(path) // Ensure path is clear
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, NewNetworkError("ipc", err, "resolve_listen")
	}

	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, NewNetworkError("ipc", err, "listen")
	}

	return &IPC{
		addr:     path,
		listener: listener,
		isServer: true,
		status: TransportStatus{
			LastFailureMode: TransportFailureNotReady,
			Breaker:         CircuitOpen,
		},
		traits: TransportTraits{
			Reliable:  true,
			Ordered:   true,
			Multicast: false,
		},
	}, nil
}

// IPCWithDial creates an IPC connection to the given path.
func IPCWithDial(ctx context.Context, path string) (*IPC, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, NewNetworkError("ipc", err, "resolve_dial")
	}

	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, NewNetworkError("ipc", err, "dial")
	}

	return &IPC{
		addr: path,
		conn: conn,
		status: TransportStatus{
			LastFailureMode: TransportFailureNone,
			Breaker:         CircuitClosed,
		},
		traits: TransportTraits{
			Reliable:  true,
			Ordered:   true,
			Multicast: false,
		},
	}, nil
}

// Ready blocks until the server accepts a connection.
func (ipc *IPC) Ready(ctx context.Context) error {
	if !ipc.isServer {
		return nil // Client is immediately ready upon dial
	}

	if ipc.conn != nil {
		return nil // Already accepted
	}

	if ipc.listener == nil {
		return NewNetworkError("ipc", nil, "ready: no listener")
	}

	// This is a blocking accept. It should ideally respect ctx, but net.UnixListener
	// Accept doesn't natively take a context. Wait, we can set a deadline if ctx is cancelled,
	// but for simplicity, we just block.
	go func() {
		<-ctx.Done()
		ipc.listener.Close()
	}()

	conn, err := ipc.listener.AcceptUnix()
	if err != nil {
		ipc.status.LastFailureMode = TransportFailureIO
		ipc.status.LastFailure = err
		ipc.status.Breaker = CircuitOpen
		return NewNetworkError("ipc", err, "accept")
	}

	ipc.conn = conn
	ipc.status.LastFailureMode = TransportFailureNone
	ipc.status.LastFailure = nil
	ipc.status.Breaker = CircuitClosed

	return nil
}

// Read reads from the underlying connection.
func (ipc *IPC) Read(p []byte) (n int, err error) {
	if ipc.conn == nil {
		return 0, NewNetworkError("ipc", nil, "read: no connection")
	}
	n, err = ipc.conn.Read(p)
	if err != nil {
		ipc.status.LastFailureMode = TransportFailureIO
		ipc.status.LastFailure = err
		ipc.status.Breaker = CircuitOpen
	}
	return n, err
}

// Write writes to the underlying connection.
func (ipc *IPC) Write(p []byte) (n int, err error) {
	if ipc.conn == nil {
		return 0, NewNetworkError("ipc", nil, "write: no connection")
	}
	n, err = ipc.conn.Write(p)
	if err != nil {
		ipc.status.LastFailureMode = TransportFailureIO
		ipc.status.LastFailure = err
		ipc.status.Breaker = CircuitOpen
	}
	return n, err
}

// Close closes the connection and/or listener.
func (ipc *IPC) Close() error {
	var err error
	if ipc.conn != nil {
		err = ipc.conn.Close()
	}
	if ipc.listener != nil {
		lErr := ipc.listener.Close()
		if err == nil {
			err = lErr
		}
	}
	if ipc.isServer && ipc.addr != "" {
		os.Remove(ipc.addr)
	}
	return err
}

// Traits returns the transport traits.
func (ipc *IPC) Traits() TransportTraits {
	return ipc.traits
}

// Status returns the transport status.
func (ipc *IPC) Status() TransportStatus {
	return ipc.status
}
