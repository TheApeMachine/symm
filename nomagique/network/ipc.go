package network

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/theapemachine/errnie"
)

/*
IPC provides same-machine transport over Unix domain sockets.
The listener side binds a Unix socket path and accepts exactly one peer.
The dialer side connects to that path eagerly during construction.
*/
type IPC struct {
	listener *net.UnixListener
	conn     net.Conn
	timeout  time.Duration
	path     string
	owner    bool
}

type ipcOption func(*IPC)

/*
NewIPC constructs an IPC transport over Unix domain sockets.
Pass IPCWithListen on the server side and IPCWithDial on the client side.
*/
func NewIPC(ctx context.Context, opts ...ipcOption) (*IPC, error) {
	ipc := &IPC{
		timeout: 10 * time.Second,
	}

	for _, opt := range opts {
		opt(ipc)
	}

	if ipc.owner {
		if err := ipc.listen(); err != nil {
			return nil, errnie.Err(errnie.IO, "[ipc] failed to listen: "+ipc.path, err)
		}
		return ipc, nil
	}

	if err := ipc.dial(); err != nil {
		return nil, errnie.Err(errnie.IO, "[ipc] failed to dial: "+ipc.path, err)
	}

	return ipc, nil
}

/*
Read receives bytes from the active Unix socket connection.
*/
func (ipc *IPC) Read(p []byte) (int, error) {
	conn, err := ipc.ensureConn(false)
	if err != nil {
		return 0, err
	}
	return conn.Read(p)
}

/*
Write sends bytes over the active Unix socket connection.
*/
func (ipc *IPC) Write(p []byte) (int, error) {
	conn, err := ipc.ensureConn(false)
	if err != nil {
		return 0, err
	}
	return conn.Write(p)
}

/*
Close terminates the listener and connection.
*/
func (ipc *IPC) Close() error {
	if ipc.conn != nil {
		_ = ipc.conn.Close()
	}
	if ipc.listener != nil {
		_ = ipc.listener.Close()
	}
	return nil
}

/*
Accept blocks until a client connects to the listening socket.
*/
func (ipc *IPC) Accept() error {
	if !ipc.owner {
		return errnie.Err(errnie.IO, "[ipc] not listening", nil)
	}
	if ipc.listener == nil {
		return errnie.Err(errnie.IO, "[ipc] listener is nil", nil)
	}
	if ipc.conn != nil {
		return nil
	}

	deadline := time.Now().Add(ipc.timeout)
	if err := ipc.listener.SetDeadline(deadline); err != nil {
		return err
	}

	connection, err := ipc.listener.Accept()
	if err != nil {
		return err
	}

	ipc.conn = connection
	return nil
}

func (ipc *IPC) listen() error {
	if ipc.path == "" {
		return nil
	}

	_ = os.Remove(ipc.path)

	address, err := net.ResolveUnixAddr("unix", ipc.path)
	if err != nil {
		return err
	}

	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		return err
	}

	ipc.listener = listener
	return nil
}

func (ipc *IPC) dial() error {
	if ipc.path == "" {
		return nil
	}

	address, err := net.ResolveUnixAddr("unix", ipc.path)
	if err != nil {
		return err
	}

	connection, err := net.DialTimeout("unix", address.String(), ipc.timeout)
	if err != nil {
		return err
	}

	ipc.conn = connection
	return nil
}

func (ipc *IPC) ensureConn(allowAccept bool) (net.Conn, error) {
	if ipc.conn != nil {
		return ipc.conn, nil
	}

	if ipc.path == "" {
		return nil, errnie.Err(errnie.IO, "[ipc] not connected", nil)
	}

	if allowAccept && ipc.owner {
		if err := ipc.Accept(); err != nil {
			return nil, err
		}
		return ipc.conn, nil
	}

	if ipc.owner {
		return nil, errnie.Err(errnie.IO, "[ipc] not connected", nil)
	}

	if err := ipc.dial(); err != nil {
		return nil, err
	}

	return ipc.conn, nil
}

/*
IPCWithListen configures the listener side on the given Unix socket path.
*/
func IPCWithListen(path string) ipcOption {
	return func(ipc *IPC) {
		ipc.path = path
		ipc.owner = true
	}
}

/*
IPCWithDial connects to a listener created with IPCWithListen(path).
*/
func IPCWithDial(path string) ipcOption {
	return func(ipc *IPC) {
		ipc.path = path
		ipc.owner = false
	}
}

/*
IPCWithAeronDir is retained for API stability and is a no-op for Unix sockets.
*/
func IPCWithAeronDir(dir string) ipcOption {
	return func(ipc *IPC) {
		_ = dir
	}
}

/*
IPCWithTimeout sets how long listen-side Accept and dial-side connect may block.
*/
func IPCWithTimeout(duration time.Duration) ipcOption {
	return func(ipc *IPC) {
		if duration > 0 {
			ipc.timeout = duration
		}
	}
}
