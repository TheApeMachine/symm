package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
WSMessage represents an incoming or outgoing WebSocket frame.
*/
type WSMessage struct {
	Type      int
	Payload   []byte
	Channel   string
	Timestamp int64
}

/*
WSConnection manages a live WebSocket session with thread-safe read/write.
*/
type WSConnection struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

/*
NewWSConnect establishes a WebSocket connection to the given URL endpoint.
*/
func NewWSConnect(endpoint string) types.Value[context.Context, *WSConnection] {
	return func(ctx context.Context) *WSConnection {
		dialer := websocket.DefaultDialer

		conn, resp, err := dialer.DialContext(ctx, endpoint, http.Header{})
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: websocket dial failed", err))
			return nil
		}

		if resp != nil && resp.Body != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				errnie.Error(errnie.Err(errnie.IO, "transport: close response body", closeErr))
			}
		}

		return &WSConnection{conn: conn}
	}
}

/*
NewWSRead creates a closure that reads the next message from the WebSocket.
*/
func NewWSRead() types.Value[*WSConnection, *WSMessage] {
	return func(ws *WSConnection) *WSMessage {
		if ws == nil || ws.conn == nil {
			return nil
		}

		msgType, payload, err := ws.conn.ReadMessage()
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: websocket read failed", err))
			return nil
		}

		return &WSMessage{
			Type:      msgType,
			Payload:   payload,
			Timestamp: time.Now().UnixMilli(),
		}
	}
}

/*
NewWSWrite creates a closure that writes a message to the WebSocket connection.
*/
func NewWSWrite(ws *WSConnection) types.Value[*WSMessage, error] {
	return func(msg *WSMessage) error {
		if msg == nil {
			return nil
		}

		if ws == nil || ws.conn == nil {
			return errnie.Error(errnie.Err(errnie.IO, "transport: websocket not connected", nil))
		}

		ws.mu.Lock()
		defer ws.mu.Unlock()

		err := ws.conn.WriteMessage(msg.Type, msg.Payload)
		if err != nil {
			return errnie.Error(errnie.Err(errnie.IO, "transport: websocket write failed", err))
		}

		return nil
	}
}

/*
NewWSClose creates a closure that cleanly shuts down the WebSocket connection.
*/
func NewWSClose() types.Value[*WSConnection, error] {
	return func(ws *WSConnection) error {
		if ws == nil || ws.conn == nil {
			return nil
		}

		ws.mu.Lock()
		defer ws.mu.Unlock()

		err := ws.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: write close message", err))
		}

		return ws.conn.Close()
	}
}

/*
NewSubscription constructs a standard subscription message structure.
*/
func NewSubscription(method, channel string, symbols ...string) types.Value[any, map[string]any] {
	return func(any) map[string]any {
		return map[string]any{
			"method": method,
			"params": map[string]any{
				"channel": channel,
				"symbol":  symbols,
			},
		}
	}
}

/*
NewPingPong creates a closure that checks for ping frames and responds with pong.
*/
func NewPingPong() types.Value[*WSMessage, *WSMessage] {
	return func(msg *WSMessage) *WSMessage {
		if msg == nil {
			return nil
		}

		if msg.Type == websocket.PingMessage {
			return &WSMessage{
				Type:      websocket.PongMessage,
				Payload:   msg.Payload,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		return msg
	}
}

/*
StartWSIngress establishes a persistent WebSocket session, subscribes to requested channels
and symbols in paced batches, and pumps incoming market payloads into the onTick callback. It handles automatic
reconnection on network drops until ctx is cancelled.
*/
func StartWSIngress(
	ctx context.Context,
	endpoint string,
	channels []string,
	symbols []string,
	batchSize int,
	pace time.Duration,
	onTick func(any),
) {
	connect := NewWSConnect(endpoint)
	read := NewWSRead()
	closeConn := NewWSClose()
	pingPong := NewPingPong()

	effectiveBatch := batchSize
	if effectiveBatch <= 0 {
		effectiveBatch = 200
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		errnie.Info(fmt.Sprintf("[websocket] connecting to %s...", endpoint))
		conn := connect(ctx)

		if conn == nil {
			errnie.Warn(fmt.Sprintf("[websocket] connect failed for %s, retrying in 3s...", endpoint))

			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}

		write := NewWSWrite(conn)

		for _, channel := range channels {
			for batch := range slices.Chunk(symbols, effectiveBatch) {
				subParams := map[string]any{
					"channel": channel,
					"symbol":  batch,
				}

				if channel == "book" {
					subParams["depth"] = 10
				}

				subMsg := map[string]any{
					"method": "subscribe",
					"params": subParams,
				}

				payload, err := json.Marshal(subMsg)
				if err != nil {
					continue
				}

				if err := write(&WSMessage{
					Type:    websocket.TextMessage,
					Payload: payload,
				}); err != nil {
					errnie.Error(errnie.Err(errnie.IO, "[websocket] subscription write failed", err))
					continue
				}

				errnie.Info(fmt.Sprintf("[websocket] subscribed to %s for %d symbols on %s", channel, len(batch), endpoint))

				if pace > 0 {
					select {
					case <-ctx.Done():
						closeConn(conn)
						return
					case <-time.After(pace):
					}
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				closeConn(conn)
				return
			default:
			}

			msg := read(conn)

			if msg == nil {
				errnie.Warn(fmt.Sprintf("[websocket] disconnected from %s, reconnecting...", endpoint))
				closeConn(conn)
				break
			}

			if msg.Type == websocket.PingMessage {
				if pong := pingPong(msg); pong != nil {
					write(pong)
				}

				continue
			}

			if msg.Type == websocket.TextMessage && len(msg.Payload) > 0 {
				var tick any

				if err := json.Unmarshal(msg.Payload, &tick); err == nil && tick != nil {
					onTick(tick)
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}
