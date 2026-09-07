package venue

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Conn is the broker test suite's null websocket.Conn: every method returns
an empty, successful default so a test only needs to embed it and override
the one method its scenario cares about.
*/
type Conn struct {
	status runtime.Stage

	// AddOrderErr, when set, is returned by AddOrder instead of a synthetic
	// success — tests use it to simulate an exchange/network rejection of an
	// order submission.
	AddOrderErr error

	// BalanceResult and TradesHistoryResult, when set, are returned verbatim
	// by Balance/TradesHistory instead of the empty defaults — tests use them
	// to simulate an exchange reporting multiple held assets with fill
	// history, e.g. for account-recovery-on-boot scenarios.
	BalanceResult       map[string]*decimal.Decimal
	TradesHistoryResult spot.TradesHistoryResult
	TradeVolumeResult   *kraken.TradeVolumeResult
	book                *spotbook.Book
	books               *sync.Map
	wsBook              *websocket.Book
}

func (conn *Conn) MarkReady() {}

func NewConn() *Conn {
	return &Conn{
		status: runtime.READY,
		wsBook: websocket.NewBook(context.Background(), spot.NewNormalizer()),
	}
}

func (conn *Conn) Close() {}

func (conn *Conn) Client() *spot.WebSocket { return nil }

func (conn *Conn) Status() runtime.Stage { return conn.status }

func (conn *Conn) SubInstrument(callback chan any) {}

func (conn *Conn) SubTicker(symbols []string) {}

func (conn *Conn) SubTrades(symbols []string) {}

func (conn *Conn) SubL3(symbols []string) {}

func (conn *Conn) UnsubTicker(symbols []string) {}

func (conn *Conn) UnsubTrades(symbols []string) {}

func (conn *Conn) UnsubL3(symbols []string) {}

func (conn *Conn) Balance() (map[string]*decimal.Decimal, error) {
	if conn.BalanceResult != nil {
		return conn.BalanceResult, nil
	}

	return nil, nil
}

func (conn *Conn) TradesHistory() (spot.TradesHistoryResult, error) {
	if conn.TradesHistoryResult.Trades != nil {
		return conn.TradesHistoryResult, nil
	}

	return spot.TradesHistoryResult{}, nil
}

func (conn *Conn) TradeBalance() (kraken.TradeBalanceResult, error) {
	return kraken.TradeBalanceResult{}, nil
}

func (conn *Conn) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	if conn.TradeVolumeResult != nil {
		return conn.TradeVolumeResult, nil
	}

	return &kraken.TradeVolumeResult{}, nil
}

func (conn *Conn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	if conn.AddOrderErr != nil {
		return spot.AddOrderResult{}, conn.AddOrderErr
	}

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

func (conn *Conn) ApplyLevel3(data kraken.Level3Data) {
	if conn.wsBook == nil {
		return
	}

	payload := &kraken.Level3{
		Type: data.Type,
		Data: []kraken.Level3Data{data},
	}
	event := &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage([]byte(`{"channel":"level3"}`)),
	}
	if err := conn.wsBook.Update(event, payload); err != nil {
		panic(err)
	}
}

func (conn *Conn) Book(symbol string, read func(*spotbook.Book)) {
	if conn.wsBook != nil {
		found := false
		conn.wsBook.Book(symbol, func(managed *spotbook.Book) {
			found = true
			read(managed)
		})
		if found {
			return
		}
	}

	read(conn.book)
}

func (conn *Conn) Books() *sync.Map {
	if conn.wsBook != nil {
		return conn.wsBook.All()
	}

	if conn.books != nil {
		return conn.books
	}

	return &sync.Map{}
}

func Order(id, price, qty string) kraken.Level3Order {
	return kraken.Level3Order{
		OrderID:    id,
		LimitPrice: Decimal(price),
		OrderQty:   Decimal(qty),
		Timestamp:  time.Now().UTC(),
	}
}
