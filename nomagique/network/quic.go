package network

import (
	"context"
	"crypto/tls"
	"io"

	"github.com/theapemachine/errnie"
	"golang.org/x/net/quic"
)

const quicReadyHandshakeByte = 0xA7

/*
QUIC provides reliable WAN transport using golang.org/x/net/quic.
A single bidirectional stream carries Value frames, giving
io.ReadWriteCloser semantics with the congestion control and
encryption that raw UDP lacks.
*/
type QUIC struct {
	endpoint *quic.Endpoint
	conn     *quic.Conn
	stream   *quic.Stream
	owner    bool
	addr     string
	tlsConf  *tls.Config
}

/*
quicOption configures a QUIC transport at construction time.
*/
type quicOption func(*QUIC)

/*
NewQUIC constructs a QUIC transport. Use QUICWithListen to start
an endpoint (then call Accept), or QUICWithDial to connect outbound,
or QUICWithStream to wrap an already-accepted stream.
*/
func NewQUIC(ctx context.Context, opts ...quicOption) (*QUIC, error) {
	q := &QUIC{}
	for _, opt := range opts {
		opt(q)
	}

	if q.stream != nil {
		return q, nil
	}

	if q.owner {
		endpoint, err := quic.Listen("udp", q.addr, &quic.Config{
			TLSConfig: q.tlsConf,
		})
		if err != nil {
			return nil, errnie.Err(errnie.IO, "[quic] failed to listen", err)
		}
		q.endpoint = endpoint
		return q, nil
	}

	endpoint, err := quic.Listen("udp", ":0", nil)
	if err != nil {
		return nil, errnie.Err(errnie.IO, "[quic] failed to open local endpoint", err)
	}

	conn, err := endpoint.Dial(ctx, "udp", q.addr, &quic.Config{
		TLSConfig: q.tlsConf,
	})
	if err != nil {
		_ = endpoint.Close(context.Background())
		return nil, errnie.Err(errnie.IO, "[quic] failed to dial", err)
	}

	stream, err := conn.NewStream(ctx)
	if err != nil {
		_ = conn.Close()
		_ = endpoint.Close(context.Background())
		return nil, errnie.Err(errnie.IO, "[quic] failed to open stream", err)
	}

	if err := q.sendHandshake(stream); err != nil {
		_ = stream.Close()
		_ = conn.Close()
		_ = endpoint.Close(context.Background())
		return nil, errnie.Err(errnie.IO, "[quic] failed handshake", err)
	}

	q.endpoint = endpoint
	q.conn = conn
	q.stream = stream
	return q, nil
}

/*
Read receives bytes from the QUIC stream.
*/
func (q *QUIC) Read(p []byte) (int, error) {
	if q.stream == nil {
		return 0, errnie.Err(errnie.IO, "[quic] stream not established", nil)
	}
	return q.stream.Read(p)
}

/*
Write sends bytes over the QUIC stream, flushing immediately so each
Value hits the wire as a distinct datagram when possible.
*/
func (q *QUIC) Write(p []byte) (n int, err error) {
	if q.stream == nil {
		return 0, errnie.Err(errnie.IO, "[quic] stream not established", nil)
	}

	if n, err = q.stream.Write(p); err != nil {
		return n, err
	}

	return n, q.stream.Flush()
}

/*
Close gracefully closes the QUIC stream and connection.
*/
func (q *QUIC) Close() error {
	if q.stream != nil {
		_ = q.stream.Close()
	}
	if q.conn != nil {
		_ = q.conn.Close()
	}
	if q.endpoint != nil {
		_ = q.endpoint.Close(context.Background())
	}
	return nil
}

/*
Accept blocks until an inbound connection arrives on the endpoint
created by QUICWithListen, then opens the first bidirectional stream.
*/
func (q *QUIC) Accept(ctx context.Context) error {
	if q.endpoint == nil {
		return errnie.Err(errnie.IO, "[quic] not listening", nil)
	}
	if q.stream != nil {
		return nil
	}

	conn, err := q.endpoint.Accept(ctx)
	if err != nil {
		return err
	}

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		_ = conn.Close()
		return err
	}

	if err := q.consumeHandshake(stream); err != nil {
		_ = stream.Close()
		_ = conn.Close()
		return err
	}

	q.conn = conn
	q.stream = stream
	return nil
}

/*
QUICWithListen creates a QUIC endpoint listening on addr. Call Accept
separately to wait for an inbound connection and stream.
*/
func QUICWithListen(addr string, tlsConf *tls.Config) quicOption {
	return func(q *QUIC) {
		q.addr = addr
		q.tlsConf = tlsConf
		q.owner = true
	}
}

/*
QUICWithDial connects to a remote QUIC endpoint and opens a
bidirectional stream. The transport is ready for Read/Write
immediately after construction.
*/
func QUICWithDial(addr string, tlsConf *tls.Config) quicOption {
	return func(q *QUIC) {
		q.addr = addr
		q.tlsConf = tlsConf
		q.owner = false
	}
}

/*
QUICWithStream wraps an already-accepted *quic.Stream. Use this on
the server side when an external accept loop manages the endpoint
and connection lifecycle.
*/
func QUICWithStream(stream *quic.Stream) quicOption {
	return func(q *QUIC) {
		q.stream = stream
	}
}

func (q *QUIC) sendHandshake(stream *quic.Stream) error {
	if _, err := stream.Write([]byte{quicReadyHandshakeByte}); err != nil {
		return err
	}
	return stream.Flush()
}

func (q *QUIC) consumeHandshake(stream *quic.Stream) error {
	var buf [1]byte
	if _, err := io.ReadFull(stream, buf[:]); err != nil {
		return err
	}
	if buf[0] != quicReadyHandshakeByte {
		return errnie.Err(errnie.IO, "invalid quic handshake byte", nil)
	}
	return nil
}
