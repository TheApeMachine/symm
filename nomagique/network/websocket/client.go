package websocket

import (
	"context"
	"fmt"
	"net/http"
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
	incoming *lf.Queue[[]byte]
}

func NewWebSocketClient(ctx context.Context) *WebSocketClientServer {
	return &WebSocketClientServer{
		System:   runtime.NewSystem(ctx, "websocket.client"),
		incoming: lf.NewQueue[[]byte](),
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

	if server.endpoint != "" && server.Status() != runtime.READY {
		server.connect()
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

	if server.Status() != runtime.READY {
		return nil
	}

	msg, ok := server.incoming.Dequeue()

	if !ok {
		return nil
	}

	return results.SetRead(msg)
}

func (server *WebSocketClientServer) connect() {
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

		go server.reconnect()
		return
	}

	server.conn = conn
	server.Transition(runtime.READY)
	server.read()

	server.Info("connected to %s", server.endpoint)
}

func (server *WebSocketClientServer) read() {
	go func() {
		for {
			select {
			case <-server.Context().Done():
				server.Close()
				return
			default:
				if server.Status() != runtime.READY {
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			_, message, err := server.conn.ReadMessage()

			if err != nil {
				server.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf(
						"[network.websocket.client.read] read message failed from %s",
						server.endpoint,
					),
					err,
				))

				server.Close()
				return
			}

			server.incoming.Enqueue(message)
		}
	}()
}

func (server *WebSocketClientServer) reconnect() {
	backoff := 50 * time.Millisecond
	maxBackoff := 2 * time.Second

	for server.Status() != runtime.READY {
		select {
		case <-server.Context().Done():
			server.Close()
			return
		case <-time.After(backoff):
			server.connect()
			backoff *= 2

			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
