package replay

import (
	"context"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
)

var _ public.WebSocketClient = (*WebSocket)(nil)

/*
WebSocket replays lines from trading.replay.file. Orders go through paper.
*/
type WebSocket struct {
	ctx     context.Context
	cancel  context.CancelFunc
	paper   *paper.WebSocket
	capture *Capture
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	paperSocket, err := paper.NewWebSocket(ctx)

	if err != nil {
		cancel()

		return nil, err
	}

	ws := &WebSocket{
		ctx:     ctx,
		cancel:  cancel,
		paper:   paperSocket,
		capture: ActiveCapture(),
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":     ws.ctx,
		"cancel":  ws.cancel,
		"paper":   ws.paper,
		"capture": ws.capture,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	if ws.paperChannel(channel) {
		return ws.paper.Connect(endpoint, channel)
	}

	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	if ws.paperChannel(channel) {
		return ws.paper.Send(channel, message)
	}

	return nil
}

func (ws *WebSocket) Stream(channel string) (<-chan *public.SocketMessage, error) {
	if ws.paperChannel(channel) {
		return ws.paper.Stream(channel)
	}

	path := strings.TrimSpace(viper.GetViper().GetString("trading.replay.file"))

	if path == "" {
		return nil, errnie.Error(errnie.Require(map[string]any{"trading.replay.file": path}))
	}

	ws.capture.start(ws.ctx, path)

	return ws.capture.subscribe(channel), nil
}

func (ws *WebSocket) Close(channel string) error {
	if ws.paperChannel(channel) {
		return ws.paper.Close(channel)
	}

	return nil
}

func (ws *WebSocket) paperChannel(channel string) bool {
	switch channel {
	case public.OrdersChannel, public.ExecutionsChannel:
		return true
	default:
		return false
	}
}
