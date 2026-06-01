package replay

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

var _ public.WebSocketClient = (*WebSocket)(nil)

/*
WebSocket is a no-live-dial client used while replay fixtures drive market data
through kraken/market. Order traffic is handled by the optimizer trader, not here.
*/
type WebSocket struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{ctx: ctx, cancel: cancel}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Send(channel string, message any) error {
	return nil
}

func (ws *WebSocket) Stream(channel string) (<-chan *public.SocketMessage, error) {
	out := make(chan *public.SocketMessage)

	close(out)

	return out, nil
}

func (ws *WebSocket) Close(channel string) error {
	return nil
}

func (ws *WebSocket) Error() error {
	return nil
}
