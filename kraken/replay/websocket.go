package replay

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/public"
	symmreplay "github.com/theapemachine/symm/replay"
)

var _ public.WebSocketClient = (*WebSocket)(nil)

/*
WebSocket replays recorded Kraken WebSocket v2 frames from config.System.ReplayFile.
*/
type WebSocket struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	streams map[string]chan struct{}
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:     ctx,
		cancel:  cancel,
		streams: make(map[string]chan struct{}),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":     ws.ctx,
		"cancel":  ws.cancel,
		"streams": ws.streams,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	return nil
}

func (ws *WebSocket) Stream(channel string) (<-chan *public.SocketMessage, error) {
	path := strings.TrimSpace(config.System.ReplayFile)

	if path == "" {
		return nil, fmt.Errorf("kraken replay websocket: replay file is not configured")
	}

	hub, err := symmreplay.Open(path)

	if err != nil {
		return nil, err
	}

	out := make(chan *public.SocketMessage, 256)
	stop := make(chan struct{})

	ws.mu.Lock()
	ws.streams[channel] = stop
	ws.mu.Unlock()

	inbound := hub.SubscribeWS(channel)

	go func() {
		defer close(out)

		ws.mu.Lock()
		delete(ws.streams, channel)
		ws.mu.Unlock()

		for {
			select {
			case <-ws.ctx.Done():
				return
			case <-stop:
				return
			case payload, ok := <-inbound:
				if !ok {
					return
				}

				messages, err := decodeSocketMessages(payload, channel)

				if err != nil {
					errnie.Error(err)
					continue
				}

				for index := range messages {
					select {
					case <-ws.ctx.Done():
						return
					case <-stop:
						return
					case out <- messages[index]:
					}
				}
			}
		}
	}()

	return out, nil
}

func (ws *WebSocket) Close(channel string) error {
	ws.mu.Lock()
	stop, ok := ws.streams[channel]

	if ok {
		close(stop)
		delete(ws.streams, channel)
	}

	ws.mu.Unlock()

	return nil
}

func decodeSocketMessages(payload []byte, channel string) ([]*public.SocketMessage, error) {
	var envelope public.SocketMessage

	if err := sonic.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("kraken replay websocket decode %s: %w", channel, err)
	}

	if envelope.Channel != channel || len(envelope.Data) == 0 {
		return nil, nil
	}

	var rows []public.SocketMessage

	if err := sonic.Unmarshal(envelope.Data, &rows); err != nil {
		row := envelope

		return []*public.SocketMessage{&row}, nil
	}

	messages := make([]*public.SocketMessage, len(rows))

	for index := range rows {
		rows[index].Channel = channel
		rows[index].Type = envelope.Type
		messages[index] = &rows[index]
	}

	return messages, nil
}
