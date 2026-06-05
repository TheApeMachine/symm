package private

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

const privateSubscriberID = "kraken/private:private"

/*
WebSocket maintains the authenticated Kraken WebSocket for public.WebSocket conns.
*/
type WebSocket struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	provider   *TokenProvider
	pool       *qpool.Q
	raw        *qpool.BroadcastGroup
	ui         *qpool.BroadcastGroup
	private    *qpool.BroadcastGroup
	subscriber *qpool.Subscriber
	conn       *websocket.Conn
	dialer     websocket.Dialer
}

/*
NewWebSocket returns paper simulation or a live authenticated connection holder.
*/
func NewWebSocket(
	ctx context.Context, pool *qpool.Q, apiKey, apiSecret string,
) public.WebSocketClient {
	return NewWebSocketWithQuoteCache(
		ctx,
		pool,
		apiKey,
		apiSecret,
		broker.EnsureQuoteCache(ctx, pool),
	)
}

/*
NewWebSocketWithQuoteCache returns paper or live private websocket with explicit
quote state for the paper exchange emulator.
*/
func NewWebSocketWithQuoteCache(
	ctx context.Context,
	pool *qpool.Q,
	apiKey string,
	apiSecret string,
	quotes *broker.QuoteCache,
) public.WebSocketClient {
	if viper.GetViper().GetString("trading.model") == "paper" {
		errnie.Info("kraken/private paper websocket", "kraken/private paper websocket")
		return paper.NewWebSocketWithQuoteCache(ctx, pool, quotes)
	}

	errnie.Info("kraken/private live websocket", "kraken/private live websocket")
	ctx, cancel := context.WithCancel(ctx)
	provider, err := NewTokenProvider(ctx, apiKey, apiSecret)

	if err != nil {
		return &WebSocket{ctx: ctx, cancel: cancel, pool: pool, err: err}
	}

	websocketClient := &WebSocket{
		ctx:      ctx,
		cancel:   cancel,
		provider: provider,
		pool:     pool,
		raw:      bus.Group(pool, "raw", 10*time.Millisecond),
		ui:       bus.Group(pool, "ui", 10*time.Millisecond),
		private:  bus.Group(pool, "kraken:private", 10*time.Millisecond),
		dialer:   *websocket.DefaultDialer,
	}
	websocketClient.subscriber = websocketClient.private.Subscribe(privateSubscriberID, 1024)

	if balanceErr := user.NewBalance(pool, provider); balanceErr != nil {
		errnie.Error(balanceErr)
	}

	if executionErr := user.NewExecution(pool, provider); executionErr != nil {
		errnie.Error(executionErr)
		websocketClient.err = executionErr
	}

	return websocketClient
}

/*
Connect dials the authenticated endpoint and registers the conn on public.WebSocket.
*/
func (websocketClient *WebSocket) Connect(
	endpoint public.EndpointType, channel string, n uint64,
) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	conn, _, err := websocketClient.dialer.Dial(string(endpoint), nil)

	if err != nil {
		websocketClient.err = err
		return err
	}

	websocketClient.conn = conn
	errnie.Info("kraken/private websocket connected", "kraken/private websocket connected")

	return nil
}

/*
Tick blocks until the authenticated session context is cancelled.
*/
func (websocketClient *WebSocket) Tick() error {
	if websocketClient.err != nil {
		return websocketClient.err
	}

	if err := websocketClient.Connect(public.WebSocketAuthURL, "kraken:private", 0); err != nil {
		return err
	}

	readErrs := make(chan error, 1)

	go func() {
		readErrs <- websocketClient.readLoop()
	}()

	for {
		select {
		case <-websocketClient.ctx.Done():
			return websocketClient.ctx.Err()
		case err := <-readErrs:
			websocketClient.err = err
			return err
		case message, ok := <-websocketClient.subscriber.Incoming:
			if !ok {
				return websocketClient.ctx.Err()
			}

			if message == nil {
				continue
			}

			if err := websocketClient.writePrivate(message.Value); err != nil {
				websocketClient.err = err
				return err
			}
		}
	}
}

/*
Close shuts down the authenticated connection holder.
*/
func (websocketClient *WebSocket) Close() error {
	websocketClient.cancel()

	if websocketClient.private != nil && websocketClient.subscriber != nil {
		websocketClient.private.Unsubscribe(websocketClient.subscriber.ID)
	}

	if websocketClient.conn == nil {
		return websocketClient.err
	}

	if err := websocketClient.conn.Close(); err != nil && websocketClient.err == nil {
		websocketClient.err = err
	}

	return websocketClient.err
}

type authFrame struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Method  string          `json:"method"`
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func (websocketClient *WebSocket) readLoop() error {
	for {
		select {
		case <-websocketClient.ctx.Done():
			return websocketClient.ctx.Err()
		default:
		}

		frame := authFrame{}

		if err := websocketClient.conn.ReadJSON(&frame); err != nil {
			return err
		}

		if frame.Channel == "" {
			continue
		}

		websocketClient.publishRaw(frame)
		websocketClient.publishDerived(frame)
	}
}

func (websocketClient *WebSocket) publishRaw(frame authFrame) {
	user.PublishRaw(
		websocketClient.raw,
		frame.Channel,
		frame.Type,
		frame.Data,
	)
}

func (websocketClient *WebSocket) publishDerived(frame authFrame) {
	switch frame.Channel {
	case public.BalancesChannel:
		var rows []user.Balance

		if err := sonic.Unmarshal(frame.Data, &rows); err != nil {
			errnie.Error(err)
			return
		}

		user.PublishWalletFromBalances(websocketClient.ui, rows)
	case public.ExecutionsChannel:
		var rows []user.Execution

		if err := sonic.Unmarshal(frame.Data, &rows); err != nil {
			errnie.Error(err)
			return
		}

		for _, execution := range rows {
			if execution.Symbol == "" || execution.ExecType != "trade" {
				continue
			}

			user.PublishExecutionDerived(websocketClient.raw, execution)
		}
	}
}

func (websocketClient *WebSocket) writePrivate(value any) error {
	frame, err := websocketClient.authorize(value)

	if err != nil {
		return err
	}

	if websocketClient.conn == nil {
		return fmt.Errorf("kraken/private websocket not connected")
	}

	return websocketClient.conn.WriteJSON(frame)
}

func (websocketClient *WebSocket) authorize(value any) (any, error) {
	token, err := websocketClient.provider.Token(websocketClient.ctx)

	if err != nil {
		return nil, err
	}

	switch frame := value.(type) {
	case user.SubscribeFrame:
		frame.Params.Token = token
		return frame, nil
	case user.ExecutionSubscribeFrame:
		frame.Params.Token = token
		return frame, nil
	case map[string]any:
		return authorizeMap(frame, token), nil
	default:
		return value, nil
	}
}

func authorizeMap(frame map[string]any, token string) map[string]any {
	params, ok := frame["params"]

	if !ok {
		return frame
	}

	frame["params"] = authorizeParams(params, token)

	return frame
}

func authorizeParams(params any, token string) any {
	switch typed := params.(type) {
	case trading.AddParams:
		typed.Token = token
		return typed
	case trading.AmendParams:
		typed.Token = token
		return typed
	case trading.CancelParams:
		typed.Token = token
		return typed
	case trading.CancelAllParams:
		typed.Token = token
		return typed
	case trading.CancelAllOrdersAfterParams:
		typed.Token = token
		return typed
	case trading.BatchAddParams:
		typed.Token = token
		return typed
	case trading.BatchCancelParams:
		typed.Token = token
		return typed
	case trading.EditParams:
		typed.Token = token
		return typed
	case map[string]any:
		typed["token"] = token
		return typed
	default:
		return params
	}
}
