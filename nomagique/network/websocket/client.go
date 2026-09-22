package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
	"unicode/utf8"

	gorillaws "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
WebSocketClientServer owns the physical WebSocket connection lifecycle.
It connects to a target URL, runs an asynchronous read pump into an
internal ring buffer, handles thread-safe message transmission, and
manages background reconnects on disconnects or network failures.
*/
type WebSocketClientServer struct {
	*runtime.System
	conn     *gorillaws.Conn
	endpoint string
	incoming *lf.Queue[receivedFrame]
	// dialing admits a single reconnect loop. Without it every failed dial
	// would start another one and the retries would double each round.
	dialing atomic.Bool
}

/* receivedFrame retains metadata at socket receipt, before graph scheduling. */
type receivedFrame struct {
	payload  []byte
	at       time.Time
	endpoint string
}

func NewWebSocketClient(ctx context.Context) *WebSocketClientServer {
	return &WebSocketClientServer{
		System:   runtime.NewSystem(ctx, "websocket.client"),
		incoming: lf.NewQueue[receivedFrame](),
	}
}

/*
Write accepts an endpoint URL and/or an outbound message frame to transmit.
*/
func (server *WebSocketClientServer) Write(ctx context.Context, call WebSocketClient_write) error {
	endpointStr, err := call.Args().Endpoint()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[network.websocket.client.Write] endpoint argument is required",
			err,
		))
	}

	if len(endpointStr) > 0 && endpointStr != server.endpoint {
		server.endpoint = endpointStr
		server.Transition(runtime.WAITING)
	}

	// The first attempt is made here so a write that carries both an endpoint
	// and a frame still sends that frame. Once a dial loop is running, retrying
	// belongs to it rather than to every write that arrives meanwhile.
	if server.endpoint != "" && server.Status() != runtime.READY && !server.dialing.Load() {
		if !server.connect() {
			server.reconnect()
		}
	}

	payload, err := call.Args().Write()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("websocket: failed to write frame to %s", server.endpoint),
			err,
		))
	}

	if len(payload) > 0 && server.conn != nil && server.Status() == runtime.READY {
		msgType := gorillaws.TextMessage

		if !utf8.Valid(payload) {
			msgType = gorillaws.BinaryMessage
		}

		if err := server.conn.WriteMessage(msgType, payload); err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("websocket: failed to write frame to %s", server.endpoint),
				err,
			))
		}
	}

	return nil
}

/*
Done returns the next queued message received from the WebSocket, along
with the current connection status.
*/
func (server *WebSocketClientServer) Done(ctx context.Context, call WebSocketClient_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"websocket: failed to allocate done results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	msg, ok := server.incoming.Dequeue()

	if !ok {
		results.SetIdle()
		return nil
	}

	results.SetFrame()
	frame := results.Frame()

	if err := frame.SetReceivedAt(msg.at.UTC().Format(time.RFC3339Nano)); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: set receive time", err))
	}

	if err := frame.SetEndpoint(msg.endpoint); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "websocket: set receive endpoint", err))
	}

	return frame.SetRead(msg.payload)
}

/*
connect dials the endpoint once, reporting whether the connection is live. It
never schedules its own retry: retrying belongs to the single loop that owns
it, so a failed dial cannot multiply into more of them.
*/
func (server *WebSocketClientServer) connect() bool {
	server.Info("connecting to %s", server.endpoint)

	dialer := &gorillaws.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}

	conn, _, err := dialer.DialContext(server.Context(), server.endpoint, nil)

	if err != nil {
		server.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[network.websocket.client.connect] dial %s failed", server.endpoint),
			err,
		))

		return false
	}

	server.conn = conn
	server.Transition(runtime.READY)
	server.read()

	server.Info("connected to %s", server.endpoint)
	return true
}

/*
reconnect keeps one dial loop running until the connection is live or the
client is shut down. A second caller while a loop is already running is a no-op
rather than another loop.
*/
func (server *WebSocketClientServer) reconnect() {
	if !server.dialing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer server.dialing.Store(false)

		backoff := 50 * time.Millisecond
		maxBackoff := 2 * time.Second

		for {
			if server.Context().Err() != nil {
				return
			}

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

func (server *WebSocketClientServer) read() {
	endpoint := server.endpoint
	connection := server.conn
	go func() {
		for {
			select {
			case <-server.Context().Done():
				return
			default:
			}

			if server.Status() != runtime.READY {
				return
			}

			_, message, err := connection.ReadMessage()

			if err != nil {
				server.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf(
						"[network.websocket.client.read] read message failed from %s",
						server.endpoint,
					),
					err,
				))

				// A dropped connection is not the end of the client: close the
				// socket and let the dial loop take it up again. Closing the
				// client itself would cancel its context for good, and every
				// later dial would fail as cancelled without ever waiting.
				server.drop()
				return
			}

			server.incoming.Enqueue(receivedFrame{payload: message, at: time.Now(), endpoint: endpoint})
		}
	}()
}

/*
drop releases the current connection and marks the client as waiting, so the
next write starts a fresh dial loop.
*/
func (server *WebSocketClientServer) drop() {
	if server.conn != nil {
		if err := server.conn.Close(); err != nil {
			server.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("[network.websocket.client.drop] close %s failed", server.endpoint),
				err,
			))
		}

		server.conn = nil
	}

	server.Transition(runtime.WAITING)
	server.reconnect()
}
