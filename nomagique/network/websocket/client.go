package websocket

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	gorillaws "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
	WebSocketClientServer owns a physical connection and its frame queues.

The graph supplies all protocol messages, including the connection handshake.
*/
type WebSocketClientServer struct {
	*runtime.System
	mu         sync.Mutex
	conn       *gorillaws.Conn
	endpoint   string
	onConnect  [][]byte
	pending    [][]byte
	incoming   *lf.Queue[receivedFrame]
	dialing    atomic.Bool
	generation uint64
	session    string
	sequence   atomic.Int64
}

/* receivedFrame retains metadata at socket receipt, before graph scheduling. */
type receivedFrame struct {
	payload    []byte
	at         time.Time
	endpoint   string
	generation uint64
	sequence   int64
}

func NewWebSocketClient(ctx context.Context) *WebSocketClientServer {
	return &WebSocketClientServer{session: rand.Text(), System: runtime.NewSystem(ctx, "websocket.client"), incoming: lf.NewQueue[receivedFrame]()}
}

/* Write configures the connection and admits every supplied outbound frame. */
func (server *WebSocketClientServer) Write(ctx context.Context, call WebSocketClient_write) error {
	endpoint, err := call.Args().Endpoint()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket: endpoint", err))
	}
	initial, err := call.Args().OnConnect()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket: connection message", err))
	}
	frames, err := call.Args().Write()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket: outgoing frames", err))
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.Context().Err() != nil {
		return errnie.Error(errnie.Err(errnie.IO, "websocket: client is closed", server.Context().Err()))
	}

	if endpoint != "" && server.endpoint != "" && endpoint != server.endpoint {
		return errnie.Error(errnie.Err(errnie.Validation, "websocket: changing an active endpoint requires a new capability", nil))
	}

	if endpoint != "" {
		server.endpoint = endpoint
	}

	if call.Args().HasOnConnect() {
		handshake := make([][]byte, initial.Len())

		for index := range initial.Len() {
			payload, err := initial.At(index)

			if err != nil {
				return errnie.Error(errnie.Err(errnie.Validation, "websocket: connection frame", err))
			}
			handshake[index] = bytes.Clone(payload)
		}
		server.onConnect = handshake
	}
	for index := range frames.Len() {
		payload, err := frames.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "websocket: outgoing frame", err))
		}

		if len(payload) > 0 {
			server.pending = append(server.pending, bytes.Clone(payload))
		}
	}

	if server.endpoint == "" {
		return nil
	}

	if server.conn == nil {
		server.reconnect()
		return nil
	}

	server.send()
	return nil
}

/* Done drains received frames even while the transport reconnects. */
func (server *WebSocketClientServer) Done(ctx context.Context, call WebSocketClient_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: results", err))
	}
	results.SetStatus(runtime.Status(server.Status()))
	server.mu.Lock()
	results.SetConnection(server.generation)
	server.mu.Unlock()
	message, found := server.incoming.Dequeue()

	if !found {
		results.SetIdle()
		return nil
	}
	results.SetFrame()
	frame := results.Frame()
	frame.SetGeneration(message.generation)
	provenance, err := json.Marshal(struct {
		Session    string `json:"session"`
		Sequence   int64  `json:"sequence"`
		Endpoint   string `json:"endpoint"`
		ReceivedAt string `json:"receivedAt"`
	}{server.session, message.sequence, message.endpoint, message.at.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return errnie.Error(err)
	}
	if err := frame.SetProvenance(provenance); err != nil {
		return errnie.Error(err)
	}

	if err := frame.SetReceivedAt(message.at.UTC().Format(time.RFC3339Nano)); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: receive timestamp", err))
	}

	if err := frame.SetEndpoint(message.endpoint); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: receive endpoint", err))
	}

	if err := frame.SetRead(message.payload); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: received frame", err))
	}
	return nil
}

/* connect sends the declared handshake before publishing the new connection. */
func (server *WebSocketClientServer) connect() bool {
	server.mu.Lock()
	endpoint := server.endpoint
	closed := server.Context().Err() != nil
	connected := server.conn != nil
	server.mu.Unlock()

	if closed {
		return false
	}

	if connected {
		return true
	}
	server.Transition(runtime.WAITING)
	server.Info("connecting to %s", endpoint)
	dialer := gorillaws.Dialer{HandshakeTimeout: 5 * time.Second, Proxy: http.ProxyFromEnvironment}
	connection, _, err := dialer.DialContext(server.Context(), endpoint, nil)

	if err != nil {
		if server.Context().Err() == nil {
			errnie.Error(errnie.Err(errnie.IO, "websocket: connect "+endpoint, err))
		}
		return false
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.Context().Err() != nil {
		if err := connection.Close(); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "websocket: close cancelled connection", err))
		}
		return false
	}
	server.conn = connection

	for _, payload := range server.onConnect {
		if err := connection.WriteMessage(gorillaws.TextMessage, payload); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "websocket: connection message", err))

			if err := connection.Close(); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "websocket: close failed handshake", err))
			}
			server.conn = nil
			return false
		}
	}
	server.Transition(runtime.READY)
	server.generation++
	server.read(connection, endpoint, server.generation)

	server.send()
	server.Info("connected to %s", endpoint)
	return true
}

/*
send transmits each admitted frame once. A write that fails is ambiguous: the
venue may or may not have taken it, and the connection it was meant for is
gone. What was pending belonged to that connection, so it is dropped with it
rather than replayed onto the next one, where it would carry that connection's
state (a token, a subscription the graph derives again per connection). The
failure is reported, the connection is closed, and the client reconnects; one
venue going away never fails the evaluation of everything else.
*/
func (server *WebSocketClientServer) send() {
	for len(server.pending) > 0 {
		payload := server.pending[0]
		messageType := gorillaws.TextMessage

		if !utf8.Valid(payload) {
			messageType = gorillaws.BinaryMessage
		}

		if err := server.conn.WriteMessage(messageType, payload); err != nil {
			errnie.Error(errnie.Err(errnie.IO, fmt.Sprintf("websocket: write to %s failed; dropping its connection", server.endpoint), err))
			server.drop()
			return
		}
		server.pending[0] = nil
		server.pending = server.pending[1:]
	}
}

/* drop closes the current connection with what was pending for it, and reconnects. */
func (server *WebSocketClientServer) drop() {
	if err := server.conn.Close(); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "websocket: close failed connection", err))
	}

	server.conn = nil
	server.pending = nil
	server.Transition(runtime.WAITING)
	server.reconnect()
}

/* reconnect admits one transport retry loop, bounded by the client context. */
func (server *WebSocketClientServer) reconnect() {
	if !server.dialing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer server.dialing.Store(false)
		backoff := 50 * time.Millisecond
		const maxBackoff = 2 * time.Second
		for server.Context().Err() == nil {
			if server.connect() {
				return
			}
			select {
			case <-server.Context().Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2

			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()
}

/* read preserves arrival order and cannot close a newer connection. */
func (server *WebSocketClientServer) read(connection *gorillaws.Conn, endpoint string, generation uint64) {
	go func() {
		for {
			_, payload, err := connection.ReadMessage()

			if err != nil {
				server.mu.Lock()

				if server.conn == connection {
					if err := connection.Close(); err != nil {
						errnie.Error(errnie.Err(errnie.IO, "websocket: close dropped connection", err))
					}
					server.conn = nil
				}
				server.mu.Unlock()

				if server.Context().Err() != nil {
					return
				}
				errnie.Error(errnie.Err(errnie.IO, "websocket: read "+endpoint, err))
				server.Transition(runtime.WAITING)
				server.reconnect()
				return
			}
			server.incoming.Enqueue(receivedFrame{payload: payload, at: time.Now(), endpoint: endpoint, generation: generation, sequence: server.sequence.Add(1) - 1})
		}
	}()
}

/* Close cancels reconnects and releases the socket so its read pump exits. */
func (server *WebSocketClientServer) Close() error {
	closeErr := server.System.Close()
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.conn == nil {
		return closeErr
	}
	err := server.conn.Close()
	server.conn = nil

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "websocket: close", errors.Join(closeErr, err)))
	}
	return closeErr
}

/* Shutdown releases transport resources when the Cap'n Proto capability dies. */
func (server *WebSocketClientServer) Shutdown() {
	if err := server.Close(); err != nil {
		errnie.Error(err)
	}
}
