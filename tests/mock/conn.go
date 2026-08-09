package mock

import (
	"encoding/json"
	"sync"

	"github.com/theapemachine/api-go/v2/pkg/book"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Conn struct {
	status types.Status
}

func NewConn() *Conn {
	return &Conn{
		status: types.INITIALIZING,
	}
}

func (conn *Conn) Close() {}

func (conn *Conn) Client() *spot.WebSocket { return nil }

func (conn *Conn) Status() types.Status { return conn.status }

func (conn *Conn) Subscribe(string, *types.Subscription[any]) *types.Subscription[any] {
	return nil
}

func (conn *Conn) Books() *sync.Map { return nil }

func (conn *Conn) Book(_ string, read func(*book.Book)) { read(nil) }

func (conn *Conn) SubInstrument(callback types.Subscription[any]) {}

func (conn *Conn) SubTicker(symbols []string) {}

func (conn *Conn) SubBook(symbols []string) {}

func (conn *Conn) SubTrades(symbols []string) {}

func (conn *Conn) SubL3(symbols []string) {}

func (conn *Conn) SubCandles(symbols []string) {}

func (conn *Conn) Balance() (map[string]*decimal.Decimal, error) { return nil, nil }

func (conn *Conn) TradesHistory() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}

func (conn *Conn) TradeBalance() (kraken.TradeBalanceResult, error) {
	return kraken.TradeBalanceResult{}, nil
}

func (conn *Conn) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	return &kraken.TradeVolumeResult{}, nil
}

func (conn *Conn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return spot.AddOrderResult{}, nil
}

func (conn *Conn) OpenOrders() (spot.OpenOrdersResult, error) {
	return spot.OpenOrdersResult{}, nil
}

func (conn *Conn) CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error) {
	return spot.CancelResult{}, nil
}

func (conn *Conn) Write(json.Marshaler, ...websocket.Callback[any]) error { return nil }

func (conn *Conn) Post(string, json.Marshaler) ([]byte, error) { return nil, nil }
