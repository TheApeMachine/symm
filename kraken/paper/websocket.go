package paper

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
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
			"kraken/paper:"+channel, 1024,
		)
	}

	identifier := NewIdentifier()
	catalog := NewPairCatalog(ctx)
	quotes := broker.EnsureQuoteCache(ctx, pool)
	balances := NewBalances(ws, identifier, catalog)
	orders := NewOrders(ctx, ws, balances, quotes, catalog, identifier)

	quotes.Subscribe(orders.tryMatchQuote)

	ws.sockets[public.BalancesChannel] = balances
	ws.sockets[public.OrdersChannel] = orders

	go catalog.Load()

	if err := user.NewBalance(pool, nil); err != nil {
		errnie.Error(err)
	}

	go ws.runPrivate()

	activate.Boot("kraken/paper websocket ready")

	return ws
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return nil
}

func (ws *WebSocket) Tick() error {
	<-ws.ctx.Done()

	return ws.ctx.Err()
}

func (ws *WebSocket) Close() error {
	ws.cancel()

	return nil
}
