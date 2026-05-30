package public

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
)

type WebSocket struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	conns  map[string]*websocket.Conn
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		conns:  make(map[string]*websocket.Conn),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"conns":  ws.conns,
	}))
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
	errnie.Debug("kraken.public.websocket.Connect", endpoint, channel)

	if endpoint == "" {
		endpoint = WebSocketURL
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		return errnie.Error(err)
	}

	ws.conns[channel] = conn
	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	conn, ok := ws.conns[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	return errnie.Error(conn.WriteJSON(message))
}

/*
envelopeTyped is implemented by row types that need the channel envelope's "type"
tag — for the book channel, "snapshot" versus "update". Stream hands the tag to any
row that opts in; rows that do not implement it are decoded exactly as before. This
is how the snapshot/delta distinction survives the generic decode instead of being
discarded with the envelope.
*/
type envelopeTyped interface {
	SetEnvelopeType(string)
}

/*
Stream owns the single reader goroutine for a connected channel: it reads each
frame, decodes the envelope, and unmarshals the data array into []T, emitting one
*T per row on the returned channel. The connection's read loop and the typed
decode live in the same goroutine, so a consumer ranges the result directly with
no intermediate pump. The channel closes when ctx is canceled or the socket ends.
*/
func Stream[T any](ws *WebSocket, channel string) (<-chan *T, error) {
	conn, ok := ws.conns[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	out := make(chan *T, 256)

	go func() {
		defer close(out)

		for {
			select {
			case <-ws.ctx.Done():
				return
			default:
				message, ok := ws.read(conn, channel)

				if !ok {
					return
				}

				if message == nil {
					continue
				}

				var rows []T

				if err := sonic.Unmarshal(message.Data, &rows); err != nil {
					continue
				}

				for index := range rows {
					if tagged, ok := any(&rows[index]).(envelopeTyped); ok {
						tagged.SetEnvelopeType(message.Type)
					}

					out <- &rows[index]
				}
			}
		}
	}()

	return out, nil
}

/*
StreamSnapshot is Stream for channels whose data is a single object rather than an
array (e.g. instruments). It emits one *T per frame.
*/
func StreamSnapshot[T any](ws *WebSocket, channel string) (<-chan *T, error) {
	conn, ok := ws.conns[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	out := make(chan *T, 256)

	go func() {
		defer close(out)

		for {
			select {
			case <-ws.ctx.Done():
				return
			default:
				message, ok := ws.read(conn, channel)

				if !ok {
					return
				}

				if message == nil {
					continue
				}

				var row T

				if err := sonic.Unmarshal(message.Data, &row); err != nil {
					continue
				}

				out <- &row
			}
		}
	}()

	return out, nil
}

/*
read pulls one frame off the socket and decodes its envelope. It returns ok=false
when the socket is dead (caller should stop), and a nil message for frames that
are control acks or belong to another channel (caller should skip).
*/
func (ws *WebSocket) read(
	conn *websocket.Conn, channel string,
) (*SocketMessage, bool) {
	_, payload, err := conn.ReadMessage()

	if err != nil {
		return nil, false
	}

	var message SocketMessage

	if err := sonic.Unmarshal(payload, &message); err != nil {
		return nil, true
	}

	if message.Channel != channel || len(message.Data) == 0 {
		return nil, true
	}

	return &message, true
}

func (ws *WebSocket) Close(channel string) error {
	conn, ok := ws.conns[channel]

	if !ok {
		return fmt.Errorf("channel %s not found", channel)
	}

	return errnie.Error(conn.Close())
}
