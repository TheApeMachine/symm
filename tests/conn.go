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
}

func NewConn(ctx context.Context) *Conn {
	ctx, cancel := context.WithCancel(ctx)

	return &Conn{
		ctx:         ctx,
		cancel:      cancel,
		status:      types.INITIALIZING,
		subscribers: &sync.Map{},
	}
}

func (conn *Conn) Status() types.Status {
	return conn.status
}

func (conn *Conn) Subscribe(
	key string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	errnie.Info(fmt.Sprintf("websocket: new subscriber %s", key))

	return utils.Subscribe(
		conn.subscribers, key, subscription,
	)
}

func (conn *Conn) Books() *sync.Map {}

func (conn *Conn) Book(string) *book.Book {}

func (conn *Conn) SubInstrument(types.Subscription[any]) {}

func (conn *Conn) SubTicker([]string) {}

func (conn *Conn) SubBook([]string) {}

func (conn *Conn) SubTrades([]string) {}

func (conn *Conn) SubL3([]string) {}

func (conn *Conn) SubCandles([]string) {}

func (conn *Conn) Balance() (map[string]*decimal.Decimal, error) {}

func (conn *Conn) TradeBalance() (spot.TradesHistoryResult, error) {}

func (conn *Conn) TradeVolume([]string) (*kraken.TradeVolumeResult, error) {}

func (conn *Conn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {}

func (conn *Conn) Write(json.Marshaler, ...websocket.Callback[any]) error {}

func (conn *Conn) Post(string, json.Marshaler) ([]byte, error) {}

func (conn *Conn) Client() *spot.WebSocket {}

func (conn *Conn) Close() {
	conn.cancel()
	conn.status = types.CLOSED
}
