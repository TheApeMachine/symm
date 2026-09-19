package transport

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
WSConnection wraps an active WebSocket connection and protects write concurrency.
*/
type WSConnection struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

/*
WSMessage represents a typed WebSocket frame payload with timestamp metadata.
*/
type WSMessage struct {
	Type      int    `json:"type"`
	Payload   []byte `json:"payload"`
	Timestamp int64  `json:"timestamp"`
}

/*
WSConnect establishes a WebSocket connection to the configured endpoint URL.
Pure types.Value closure with local dialer state.
*/
type WSConnect types.Value[context.Context, *WSConnection]

func NewWSConnect(endpoint types.String) WSConnect {
	dialer := websocket.DefaultDialer
	return func(ctx context.Context) *WSConnection {
		if endpoint == nil {
			errnie.Error(errnie.Err(errnie.Validation, "transport: websocket endpoint is nil", nil))
			return nil
		}
		ep := endpoint(ctx)
		if ep == "" {
			errnie.Error(errnie.Err(errnie.Validation, "transport: websocket endpoint is empty", nil))
			return nil
		}
		conn, resp, err := dialer.DialContext(ctx, ep, http.Header{})
		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "transport: websocket dial failed", err))
			return nil
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return &WSConnection{conn: conn}
	}
}

/*
WSRead reads the next frame payload from an active WebSocket connection.
Pure types.Value closure.
*/
type WSRead types.Value[*WSConnection, *WSMessage]

func NewWSRead() WSRead {
	return func(ws *WSConnection) *WSMessage {
		if ws == nil || ws.conn == nil {
			return nil
		}
		msgType, payload, err := ws.conn.ReadMessage()
		if err != nil {
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
WSWrite writes a message frame to an active WebSocket connection.
Pure types.Value closure.
*/
type WSWrite types.Value[*WSMessage, error]

func NewWSWrite(ws *WSConnection) WSWrite {
	return func(msg *WSMessage) error {
		if msg == nil {
			return nil
		}
		if ws == nil || ws.conn == nil {
			return errnie.Error(errnie.Err(errnie.IO, "transport: websocket not connected", nil))
		}
		ws.mu.Lock()
		defer ws.mu.Unlock()
		return ws.conn.WriteMessage(msg.Type, msg.Payload)
	}
}

/*
WSClose cleanly closes an active WebSocket connection.
Pure types.Value closure.
*/
type WSClose types.Value[*WSConnection, error]

func NewWSClose() WSClose {
	return func(ws *WSConnection) error {
		if ws == nil || ws.conn == nil {
			return nil
		}
		ws.mu.Lock()
		defer ws.mu.Unlock()
		_ = ws.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		return ws.conn.Close()
	}
}

/*
WSPingPong responds to WebSocket ping frames with a pong frame.
Pure types.Value closure with local state.
*/
type WSPingPong types.Value[*WSMessage, *WSMessage]

func NewWSPingPong(ws *WSConnection) WSPingPong {
	return func(msg *WSMessage) *WSMessage {
		if msg == nil {
			return nil
		}
		if msg.Type == websocket.PingMessage && ws != nil && ws.conn != nil {
			ws.mu.Lock()
			_ = ws.conn.WriteMessage(websocket.PongMessage, nil)
			ws.mu.Unlock()
		}
		return msg
	}
}

/*
JSONMessage is a pure Value closure that outputs its configured data payload.
*/
type JSONMessage types.Value[any, any]

func NewJSONMessage(payload types.Any) JSONMessage {
	return func(in any) any {
		if in != nil {
			return in
		}
		if payload != nil {
			return payload(nil)
		}
		return nil
	}
}

/*
EncodeJSON serializes arbitrary data into JSON bytes.
Pure types.Value closure.
*/
type EncodeJSON types.Value[any, []byte]

func NewEncodeJSON() EncodeJSON {
	return func(in any) []byte {
		data, err := sonic.Marshal(in)
		if err != nil {
			return nil
		}
		return data
	}
}

/*
DecodeJSON deserializes JSON bytes into structured data.
Pure types.Value closure.
*/
type DecodeJSON types.Value[[]byte, any]

func NewDecodeJSON() DecodeJSON {
	return func(data []byte) any {
		var out any
		if err := sonic.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

/*
Batch divides a slice of items into batches of the configured size.
Pure types.Value closure.
*/
type Batch[T any] types.Value[[]T, [][]T]

func NewBatch[T any](size types.Integer) Batch[T] {
	return func(items []T) [][]T {
		batchSize := 0
		if size != nil {
			batchSize = size(items)
		}
		if batchSize <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"batch: size must be positive",
				nil,
			))
			return nil
		}
		var batches [][]T
		for chunk := range slices.Chunk(items, batchSize) {
			batches = append(batches, chunk)
		}
		return batches
	}
}

/*
WSStream continuously reads messages from an endpoint and forwards to a sink.
Reconnections and pace are handled adaptively.
*/
type WSStream types.Value[types.Value[any, any], error]

func NewWSStream(
	ctx context.Context,
	endpoint types.String,
	messages []types.Any,
	pace types.Integer,
) WSStream {
	connect := NewWSConnect(endpoint)
	read := NewWSRead()
	closeConn := NewWSClose()

	return func(consumer types.Value[any, any]) error {
		paceMs := 0
		if pace != nil {
			paceMs = pace(nil)
		}
		delay := time.Duration(paceMs) * time.Millisecond

		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			ws := connect(ctx)
			if ws == nil {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(3 * time.Second):
					continue
				}
			}

			write := NewWSWrite(ws)
			pingPong := NewWSPingPong(ws)

			for _, msgFn := range messages {
				if msgFn == nil {
					continue
				}
				msg := msgFn(nil)
				var payload []byte
				switch m := msg.(type) {
				case []byte:
					payload = m
				case string:
					payload = []byte(m)
				default:
					data, err := sonic.Marshal(m)
					if err == nil {
						payload = data
					}
				}
				if len(payload) > 0 {
					_ = write(&WSMessage{
						Type:      websocket.TextMessage,
						Payload:   payload,
						Timestamp: time.Now().UnixMilli(),
					})
				}
				if delay > 0 {
					time.Sleep(delay)
				}
			}

			for {
				select {
				case <-ctx.Done():
					_ = closeConn(ws)
					return nil
				default:
				}

				msg := read(ws)
				if msg == nil {
					_ = closeConn(ws)
					break
				}

				msg = pingPong(msg)
				if msg.Type == websocket.TextMessage && len(msg.Payload) > 0 {
					var raw any
					if err := sonic.Unmarshal(msg.Payload, &raw); err == nil && consumer != nil {
						consumer(raw)
					}
				}
			}

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(1 * time.Second):
			}
		}
	}
}
