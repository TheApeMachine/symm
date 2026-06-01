package public

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

var socket *WebSocket
var socketOnce sync.Once

type WebSocketClient interface {
	Connect(endpoint EndpointType, channel string) error
	Send(channel string, message any) error
	Stream(channel string) (<-chan *SocketMessage, error)
	Close(channel string) error
}

type endpointHub struct {
	conn    *websocket.Conn
	streams map[string][]chan *SocketMessage
	mu      sync.RWMutex
	writeMu sync.Mutex
}

type WebSocket struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	endpoints map[EndpointType]*endpointHub
	channels  map[string]EndpointType
	recorder  io.Writer
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	socketOnce.Do(func() {
		socket = &WebSocket{
			ctx:       ctx,
			cancel:    cancel,
			endpoints: make(map[EndpointType]*endpointHub),
			channels:  make(map[string]EndpointType),
		}
		socket.bindRecorder()
	})

	return socket, errnie.Error(errnie.Require(map[string]any{
		"ctx":       socket.ctx,
		"cancel":    socket.cancel,
		"endpoints": socket.endpoints,
		"channels":  socket.channels,
	}))
}

func (ws *WebSocket) bindRecorder() {
	if viper.GetViper().Get("trading.model") != "record" {
		return
	}

	recorder, err := os.Create(viper.GetViper().GetString("trading.record.file"))

	if err != nil {
		errnie.Error(err)

		return
	}

	ws.recorder = recorder
}

func (ws *WebSocket) Connect(endpoint EndpointType, channel string) error {
	errnie.Debug("kraken.public.websocket.Connect", endpoint, channel)

	if endpoint == "" {
		endpoint = WebSocketURL
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.channels[channel] = endpoint

	if _, ok := ws.endpoints[endpoint]; ok {
		return nil
	}

	conn, _, err := websocket.DefaultDialer.Dial(string(endpoint), nil)

	if err != nil {
		return errnie.Error(err)
	}

	hub := &endpointHub{
		conn:    conn,
		streams: make(map[string][]chan *SocketMessage),
	}
	ws.endpoints[endpoint] = hub

	go ws.pump(hub)

	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	hub, err := ws.hubFor(channel)

	if err != nil {
		return err
	}

	if err := func() error {
		hub.writeMu.Lock()
		defer hub.writeMu.Unlock()

		return hub.conn.WriteJSON(message)
	}(); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (ws *WebSocket) Stream(channel string) (<-chan *SocketMessage, error) {
	hub, err := ws.hubFor(channel)

	if err != nil {
		return nil, err
	}

	out := make(chan *SocketMessage, 256)

	hub.mu.Lock()
	hub.streams[channel] = append(hub.streams[channel], out)
	hub.mu.Unlock()

	return out, nil
}

func (ws *WebSocket) StreamSnapshot(channel string) (<-chan *SocketMessage, error) {
	source, err := ws.Stream(channel)

	if err != nil {
		return nil, err
	}

	out := make(chan *SocketMessage, 1)

	go func() {
		defer close(out)

		for message := range source {
			if message == nil {
				continue
			}

			select {
			case <-ws.ctx.Done():
				return
			case out <- message:
				return
			}
		}
	}()

	return out, nil
}

func (ws *WebSocket) pump(hub *endpointHub) {
	defer ws.teardown(hub)

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		_, payload, err := hub.conn.ReadMessage()

		if err != nil {
			if ws.ctx.Err() == nil {
				errnie.Error(fmt.Errorf("kraken ws read: %w", err))
			}

			return
		}

		var message SocketMessage

		if err := sonic.Unmarshal(payload, &message); err != nil {
			errnie.Error(fmt.Errorf("kraken ws envelope decode: %w", err))

			continue
		}

		if len(message.Data) == 0 {
			continue
		}

		ws.record(&message)
		ws.fanout(hub, &message)
	}
}

func (ws *WebSocket) fanout(hub *endpointHub, message *SocketMessage) {
	hub.mu.RLock()
	subscribers := append([]chan *SocketMessage(nil), hub.streams[message.Channel]...)
	hub.mu.RUnlock()

	for _, subscriber := range subscribers {
		if err := message.EmitRows(ws.ctx, subscriber); err != nil {
			errnie.Error(err)
		}
	}
}

func (ws *WebSocket) hubFor(channel string) (*endpointHub, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	endpoint, ok := ws.channels[channel]

	if !ok {
		return nil, fmt.Errorf("channel %s not found", channel)
	}

	hub := ws.endpoints[endpoint]

	if hub == nil {
		return nil, fmt.Errorf("channel %s not connected", channel)
	}

	return hub, nil
}

func (ws *WebSocket) record(message *SocketMessage) {
	if ws.recorder == nil {
		return
	}

	rows, err := message.SplitDataRows()

	if err != nil {
		return
	}

	for _, row := range rows {
		payload, err := sonic.Marshal(row)

		if err != nil {
			continue
		}

		ws.recorder.Write(payload)
		ws.recorder.Write([]byte{'\n'})
	}
}

func (ws *WebSocket) Close(channel string) error {
	ws.mu.Lock()

	endpoint, ok := ws.channels[channel]

	if !ok {
		ws.mu.Unlock()

		return fmt.Errorf("channel %s not found", channel)
	}

	delete(ws.channels, channel)
	hub := ws.endpoints[endpoint]

	endpointActive := false

	for _, mapped := range ws.channels {
		if mapped == endpoint {
			endpointActive = true

			break
		}
	}

	if !endpointActive {
		delete(ws.endpoints, endpoint)
	}

	ws.mu.Unlock()

	if hub == nil {
		return fmt.Errorf("channel %s not connected", channel)
	}

	hub.mu.Lock()
	subscribers := hub.streams[channel]
	delete(hub.streams, channel)
	hub.mu.Unlock()

	for _, subscriber := range subscribers {
		close(subscriber)
	}

	if !endpointActive {
		return errnie.Error(hub.conn.Close())
	}

	return nil
}

func (ws *WebSocket) teardown(hub *endpointHub) {
	hub.mu.Lock()

	for _, subscribers := range hub.streams {
		for _, subscriber := range subscribers {
			close(subscriber)
		}
	}

	hub.streams = make(map[string][]chan *SocketMessage)
	hub.mu.Unlock()

	_ = hub.conn.Close()
}
