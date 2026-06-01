package paper

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/public"
)

/*
WebSocket is a fake WebSocket client for paper trading, that acts exactly
like the real Kraken WebSocket client, but instead of making actual API
calls, it just returns an accurately simulated response.
*/
type WebSocket struct {
	ctx    context.Context
	cancel context.CancelFunc
}

/*
NewWebSocket builds a paper WebSocket client.
*/
func NewWebSocket(ctx context.Context) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{ctx: ctx, cancel: cancel}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
	}))
}

/*
Connect connects the WebSocket client to the Kraken API.
*/
func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

/*
Send sends a message to the WebSocket client.
*/
func (ws *WebSocket) Send(channel string, message any) error {
	return nil
}

/*
Close closes the WebSocket client.
*/
func (ws *WebSocket) Close(channel string) error {
	return nil
}

/*
Error returns the error of the WebSocket client.
*/
func (ws *WebSocket) Error() error {
	return nil
}
