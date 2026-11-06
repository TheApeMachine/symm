package broker

import (
	"context"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Live is the real tier's private session: websocket token, balance and
execution subscriptions, and authenticated order writes.
*/
type Live struct {
	ctx    context.Context
	token  func(context.Context) (string, error)
	socket *network.WebsocketClient
}

func NewLive(ctx context.Context, private network.Client) *Live {
	socket, ok := private.(*network.WebsocketClient)

	if !ok || socket == nil {
		return nil
	}

	auth := kraken.NewAuth()

	return &Live{
		ctx:    ctx,
		token:  auth.Token,
		socket: socket,
	}
}

/*
Arm subscribes balances and executions, and resubscribes after a redial.
*/
func (live *Live) Arm() error {
	live.socket.OnReconnect(func() error {
		return live.Subscribe()
	})

	if err := live.Subscribe(); err != nil {
		return live.socket.SoftDown(err)
	}

	return nil
}

/*
Subscribe opens the private balance and execution streams.
*/
func (live *Live) Subscribe() error {
	token, err := live.token(live.ctx)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "[broker.Live.Subscribe] failed to get token", err,
		))
	}

	if err := live.write(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "balances",
			"snapshot": true,
			"token":    token,
		},
	}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "[broker.Live.Subscribe] failed to write balance subscription", err,
		))
	}

	return live.write(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":     "executions",
			"snap_orders": true,
			"snap_trades": true,
			"token":       token,
		},
	})
}

func (live *Live) write(payload map[string]any) error {
	buf, err := sonic.Marshal(payload)

	if err != nil {
		return errnie.Err(
			errnie.Internal, "[broker.Live] failed to marshal subscription", err,
		)
	}

	if err := live.socket.Write(buf); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Seal copies an add_order frame and attaches the session token.
order_qty is sent as a JSON number, which the private socket requires.
*/
func (live *Live) Seal(message []byte) ([]byte, error) {
	var request struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}

	if err := sonic.Unmarshal(message, &request); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Live.Seal] order frame is not JSON", err,
		))
	}

	if request.Method != "add_order" {
		return message, nil
	}

	token, err := live.token(live.ctx)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal, "[broker.Live.Seal] failed to get token", err,
		))
	}

	if request.Params == nil {
		request.Params = map[string]any{}
	}

	request.Params["token"] = token

	if qty, ok := request.Params["order_qty"].(string); ok && qty != "" {
		request.Params["order_qty"] = json.Number(qty)
	}

	buf, err := sonic.Marshal(request)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal, "[broker.Live.Seal] failed to marshal order", err,
		))
	}

	return buf, nil
}

/*
BalanceURL is the signed REST balance path for a live account.
*/
func (live *Live) BalanceURL() string {
	return "https://api.kraken.com" + system.Cfg.WebSocket.Endpoints.Balance
}
