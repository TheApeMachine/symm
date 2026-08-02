package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Conn implements the interface needed by the websocket.API so that it
can replace the real connections to the Kraken API, while being entirely
transparent to the rest of the system. This allows us to test the market 
mechanics without needing to connect to the real API.
The way this works is via the fixtures, which use real json payloads from
the Kraken API as a template for the events that are emitted. A signal
generator is used to dynamically inject the values into the JSON payloads
to simulate various market conditions, such as price changes, order book depth, 
and trade volume.
Additionally, there is a transition mechanism, which allows the tests to simulate 
the transition from one market state to another, such as a sudden price drop or a 
liquidity shock. This is done by defining a series of market states and the conditions 
under which the system should transition from one state to another.
*/
type Conn struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	subscribers *sync.Map
	manager     *spot.BookManager
}

func NewConn(ctx context.Context) *Conn {
	ctx, cancel := context.WithCancel(ctx)

	return &Conn{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.READY,
		subscribers: &sync.Map{},
		manager:     spot.NewBookManager(),
	}
}

func (conn *Conn) Status() types.Status {
	return conn.status
}

func (conn *Conn) Subscribe(
	key string, subscription ...*types.Subscription[any],
) *types.Subscription[any] {
	errnie.Info(fmt.Sprintf("websocket: new subscriber %s", key))

	sub := types.NewSubscription[any]()

	if len(subscription) > 0 && subscription[0] != nil {
		sub = subscription[0]
	}

	return utils.Subscribe(
		conn.subscribers, key, sub,
	)
}

func (conn *Conn) Publish(channel string, payload []byte) {
	if len(payload) == 0 {
		return
	}

	found, ok := conn.subscribers.Load(channel)

	if !ok || found == nil {
		return
	}

	var message any = payload

	switch channel {
	case "ticker":
		message = kraken.NewTicker(payload)
	case "trade":
		message = kraken.NewTrade(payload)
	case "book":
		message = kraken.NewBook(payload)
	case "level3", "l3":
		message = kraken.NewLevel3(payload)
	}

	for _, subscriber := range found.([]*types.Subscription[any]) {
		subscriber.Send(message)
	}
}

func (conn *Conn) Books() *spot.BookManager {
	return conn.manager
}

func (conn *Conn) Book(symbol string) *book.Book {
	return conn.manager.GetBook(symbol)
}

func (conn *Conn) SubInstrument(subscription types.Subscription[any]) {}

func (conn *Conn) SubTicker(symbols []string) {}

func (conn *Conn) SubBook(symbols []string) {}

func (conn *Conn) SubTrades(symbols []string) {}

func (conn *Conn) SubL3(symbols []string) {}

func (conn *Conn) SubCandles(symbols []string) {}

func (conn *Conn) Balance() (map[string]*decimal.Decimal, error) {
	return make(map[string]*decimal.Decimal), nil
}

func (conn *Conn) TradeBalance() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}

func (conn *Conn) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	return &kraken.TradeVolumeResult{}, nil
}

func (conn *Conn) AddOrder(request *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return spot.AddOrderResult{}, nil
}

func (conn *Conn) Write(marshaler json.Marshaler, callbacks ...websocket.Callback[any]) error {
	return nil
}

func (conn *Conn) Post(endpoint string, marshaler json.Marshaler) ([]byte, error) {
	return []byte("{}"), nil
}

func (conn *Conn) Client() *spot.WebSocket {
	return nil
}

func (conn *Conn) Close() {
	conn.cancel()
	conn.status = types.CLOSED
}
