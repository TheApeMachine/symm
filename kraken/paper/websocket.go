package paper

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Socket handles one kraken:private message type and returns the raw bus payload.
*/
type Socket interface {
	Send(message *qpool.QValue[any]) public.SocketMessage
}

/*
WebSocket simulates Kraken private websocket traffic on raw and kraken:private.
*/
type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	sockets     map[string]Socket
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		sockets:     make(map[string]Socket),
	}

	for _, channel := range []string{"raw", "kraken:private"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(
			"kraken/paper:"+channel, 128,
		)
	}

	identifier := NewIdentifier()
	catalog := NewPairCatalog(ctx)
	prices := NewPrices(ctx, pool)
	balances := NewBalances(ws, identifier, catalog)
	orders := NewOrders(ctx, ws, balances, prices, catalog, identifier)

	ws.sockets[public.BalancesChannel] = balances
	ws.sockets[public.OrdersChannel] = orders

	go prices.Run()
	go catalog.Load()

	activate.Boot("kraken/paper websocket ready")

	return ws
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Tick() error {
	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:private"].Incoming:
			if !ok {
				errnie.Debug("kraken.paper.websocket.Tick", "no ok")
				return nil
			}

			if message == nil {
				errnie.Debug("kraken.paper.websocket.Tick", "nil message")
				continue
			}

			var out public.SocketMessage

			if socket, ok := ws.sockets[message.Type]; ok {
				out = socket.Send(message)
			}

			if out.Channel == "" {
				continue
			}

			activate.Once("kraken/paper:channel:" + out.Channel)

			if ch := ws.broadcasts["raw"]; ch != nil {
				ch.Send(&qpool.QValue[any]{
					Type:  out.Channel,
					Value: out,
				})
			}
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	return nil
}
