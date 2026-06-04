package private

import (
	"context"
	"math"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

/*
WebSocket maintains the authenticated Kraken WebSocket for public.WebSocket conns.
*/
type WebSocket struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	provider *TokenProvider
	conns    []*websocket.Conn
}

/*
NewWebSocket returns paper simulation or a live authenticated connection holder.
*/
func NewWebSocket(
	ctx context.Context, pool *qpool.Q, apiKey, apiSecret string,
) public.WebSocketClient {
	if viper.GetViper().GetString("trading.model") == "paper" {
		errnie.Info("kraken/private paper websocket", "kraken/private paper websocket")
		return paper.NewWebSocket(ctx, pool)
	}

	errnie.Info("kraken/private live websocket", "kraken/private live websocket")
	provider, err := NewTokenProvider(ctx, apiKey, apiSecret)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:      ctx,
		cancel:   cancel,
		provider: provider,
		conns:    make([]*websocket.Conn, 1),
	}

	if balanceErr := user.NewBalance(pool, provider); balanceErr != nil {
		errnie.Error(balanceErr)
	}

	return public.NewWebSocket(ctx, pool, nil, ws.conns...)
}

/*
Connect dials the authenticated endpoint and registers the conn on public.WebSocket.
*/
func (ws *WebSocket) Connect(
	endpoint public.EndpointType, channel string, n uint64,
) error {
	if endpoint == "" {
		endpoint = public.WebSocketAuthURL
	}

	ws.conns[0], _, ws.err = websocket.DefaultDialer.Dial(string(endpoint), nil)

	if ws.err != nil {
		errnie.Error(ws.err)

		n = uint64(
			math.Round((math.Pow(
				math.Phi, float64(n),
			) + math.Pow(
				math.Phi-1, float64(n),
			)) / math.Sqrt(5)),
		)

		time.Sleep(time.Duration(n) * time.Second)

		return nil
	}

	public.AppendConn(ws.conns[0])
	errnie.Info("kraken/private websocket connected", "kraken/private websocket connected")

	return nil
}

/*
Tick blocks until the authenticated session context is cancelled.
*/
func (ws *WebSocket) Tick() error {
	return nil
}

/*
Close shuts down the authenticated connection holder.
*/
func (ws *WebSocket) Close() error {
	ws.conns[0].Close()
	ws.cancel()

	return nil
}
