package kraken

import (
	"context"
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

/*
Live is the authenticated private websocket for real trading. Write injects a
fresh websockets token into private method envelopes. Post hits private REST.
*/
type Live struct {
	ctx  context.Context
	conn interface {
		Read(...string) ([]byte, error)
		Write([]byte) error
	}
	auth *Auth
}

func NewLive(
	ctx context.Context,
	conn interface {
		Read(...string) ([]byte, error)
		Write([]byte) error
	},
	auth *Auth,
) *Live {
	if auth == nil {
		auth = NewAuth()
	}

	return &Live{ctx: ctx, conn: conn, auth: auth}
}

func (live *Live) Read(keys ...string) ([]byte, error) {
	return live.conn.Read(keys...)
}

func (live *Live) Write(buf []byte) error {
	authorized, err := live.authorize(buf)

	if err != nil {
		return err
	}

	return live.conn.Write(authorized)
}

func (live *Live) OnReconnect(hook func() error) {
	type reconnector interface {
		OnReconnect(func() error)
	}

	if ws, ok := live.conn.(reconnector); ok {
		ws.OnReconnect(hook)
	}
}

func (live *Live) authorize(buf []byte) ([]byte, error) {
	envelope := map[string]any{}

	if err := sonic.Unmarshal(buf, &envelope); err != nil {
		return nil, err
	}

	method, _ := envelope["method"].(string)

	switch method {
	case "add_order", "subscribe", "unsubscribe", "cancel_order", "amend_order":
	default:
		return buf, nil
	}

	params, _ := envelope["params"].(map[string]any)

	if params == nil {
		params = map[string]any{}
		envelope["params"] = params
	}

	if token, _ := params["token"].(string); token != "" {
		return buf, nil
	}

	token, err := live.auth.Token(live.ctx)

	if err != nil {
		return nil, err
	}

	params["token"] = token
	return sonic.Marshal(envelope)
}

func (live *Live) SubscribeExecutions() error {
	token, err := live.auth.Token(live.ctx)

	if err != nil {
		return err
	}

	buf, err := NewExecutionSubscription(token).MarshalJSON()

	if err != nil {
		return errnie.Err(errnie.IO, "[live] executions subscribe marshal failed", err)
	}

	return live.Write(buf)
}

func (live *Live) SubscribeBalances() error {
	token, err := live.auth.Token(live.ctx)

	if err != nil {
		return err
	}

	buf, err := NewBalanceSubscription(token).MarshalJSON()

	if err != nil {
		return errnie.Err(errnie.IO, "[live] balances subscribe marshal failed", err)
	}

	return live.Write(buf)
}

func (live *Live) Watch(
	url string,
	body any,
	callback func(json.Unmarshaler),
) {
	go func() {
		for {
			select {
			case <-live.ctx.Done():
				return
			default:
			}

			response, err := live.Post(url, body)

			if err != nil {
				continue
			}

			var data json.Unmarshaler

			if err := json.Unmarshal(response, &data); err != nil {
				continue
			}

			callback(data)
		}
	}()
}

/*
Post hits authenticated private REST with the same paths paper serves.
*/
func (live *Live) Post(url string, body any) ([]byte, error) {
	client, err := NewAuthenticatedREST()

	if err != nil {
		return nil, errnie.Error(err)
	}

	resp, err := spot.Call[any](client, spot.RequestOptions{
		Auth:   true,
		Method: "POST",
		Path:   url,
		Body:   body,
	})

	if err != nil {
		return nil, errnie.Error(err)
	}

	if resp == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[live] empty post response", nil,
		))
	}

	return sonic.Marshal(resp)
}