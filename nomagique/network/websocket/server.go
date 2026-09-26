package websocket

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	gorillaws "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
WebSocketServerServer owns incoming client connections, message broadcasting,
and lock-free queuing of incoming messages from connected WebSocket peers.
*/
type WebSocketServerServer struct {
	*runtime.System
	upgrader  gorillaws.Upgrader
	clients   sync.Map
	joined    atomic.Value
	incoming  *lf.Queue[[]byte]
	focusChan chan string
	out       []byte
}

func NewWebSocketServer(ctx context.Context) *WebSocketServerServer {
	server := &WebSocketServerServer{
		System: runtime.NewSystem(ctx, "websocket.server"),
		upgrader: gorillaws.Upgrader{
			CheckOrigin: func(request *http.Request) bool { return true },
		},
		incoming:  lf.NewQueue[[]byte](),
		focusChan: make(chan string, 32),
	}

	server.Transition(runtime.READY)
	return server
}

/*
UpgradeHandler returns an http.HandlerFunc that upgrades incoming HTTP
requests to WebSocket connections and pumps received messages into the
lock-free incoming queue.
*/
func (server *WebSocketServerServer) UpgradeHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		conn, err := server.upgrader.Upgrade(writer, request, nil)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[websocket.server.UpgradeHandler] handshake upgrade failed",
				err,
			))
			return
		}

		// One frame may wait behind the active socket write. A peer that
		// exceeds this transport allowance is disconnected and refreshes on
		// reconnect; it cannot retain unbounded telemetry or block the graph.
		outgoing := make(chan []byte, 1)
		disconnected := make(chan struct{})
		server.clients.Store(conn, outgoing)
		go server.transmit(conn, outgoing, disconnected)

		if joined, set := server.joined.Load().(func()); set {
			joined()
		}

		go func() {
			defer func() {
				close(disconnected)
				server.disconnect(conn)
			}()

			for {
				select {
				case <-server.Context().Done():
					return
				default:
				}

				_, messageBytes, err := conn.ReadMessage()

				if err != nil {
					break
				}

				var msg map[string]any

				if err := sonic.Unmarshal(messageBytes, &msg); err == nil {
					if typ, ok := msg["type"].(string); ok && strings.EqualFold(typ, "focus") {
						if symbol, ok := msg["symbol"].(string); ok && symbol != "" {
							select {
							case server.focusChan <- symbol:
							default:
							}
						}
					}
				}

				server.incoming.Enqueue(messageBytes)
			}
		}()
	}
}

/*
Write transmits a payload to all connected WebSocket clients.
*/
func (server *WebSocketServerServer) Write(ctx context.Context, call WebSocketServer_write) error {
	data, err := call.Args().Data()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"websocket.server: failed to read data arg",
			err,
		))
	}

	if len(data) > 0 {
		server.Broadcast(data)
		server.out = bytes.Clone(data)
	}

	return nil
}

/*
OnJoin runs join every time a client connects.
*/
func (server *WebSocketServerServer) OnJoin(join func()) {
	server.joined.Store(join)
}

/*
Broadcast admits immutable frames without waiting for any peer's socket.
An overloaded peer is disconnected visibly; reconnect triggers OnJoin refresh.
*/
func (server *WebSocketServerServer) Broadcast(data []byte) {
	var payload []byte
	server.clients.Range(func(key, value any) bool {
		if payload == nil {
			payload = bytes.Clone(data)
		}

		connection := key.(*gorillaws.Conn)
		outgoing := value.(chan []byte)

		select {
		case outgoing <- payload:
		default:
			errnie.Error(errnie.Err(errnie.IO, "websocket: disconnecting peer whose output cannot keep up", nil))
			server.disconnect(connection)
		}
		return true
	})
}

/* transmit owns the sole socket writer for a connected peer. */
func (server *WebSocketServerServer) transmit(connection *gorillaws.Conn, outgoing <-chan []byte, disconnected <-chan struct{}) {
	for {
		select {
		case <-server.Context().Done():
			server.disconnect(connection)
			return
		case <-disconnected:
			return
		case payload := <-outgoing:
			if err := connection.WriteMessage(gorillaws.BinaryMessage, payload); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "websocket: peer write failed", err))
				server.disconnect(connection)
				return
			}
		}
	}
}

/* disconnect removes and closes a connection exactly once across both pumps. */
func (server *WebSocketServerServer) disconnect(connection *gorillaws.Conn) {
	if _, found := server.clients.LoadAndDelete(connection); !found {
		return
	}

	if err := connection.Close(); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "websocket: close peer", err))
	}
}

/*
Done returns the next received message or the last broadcasted payload,
along with the current lifecycle status.
*/
func (server *WebSocketServerServer) Done(ctx context.Context, call WebSocketServer_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"websocket.server: failed to allocate done results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))

	if server.Status() != runtime.READY {
		return nil
	}

	msg, ok := server.incoming.Dequeue()

	if ok {
		return results.SetOut(msg)
	}

	if len(server.out) > 0 {
		err := results.SetOut(server.out)
		server.out = nil
		return err
	}

	return nil
}

/*
Focus exposes the stream of focused symbol requests from connected clients.
*/
func (server *WebSocketServerServer) Focus() <-chan string {
	return server.focusChan
}

/*
Close terminates all client connections and shuts down the runtime system.
*/
func (server *WebSocketServerServer) Close() error {
	server.clients.Range(func(key, value any) bool {
		server.disconnect(key.(*gorillaws.Conn))
		return true
	})

	return server.System.Close()
}

/* Shutdown closes peer pumps when the last Cap'n Proto reference is released. */
func (server *WebSocketServerServer) Shutdown() {
	if err := server.Close(); err != nil {
		errnie.Error(err)
	}
}
