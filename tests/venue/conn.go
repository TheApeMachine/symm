package venue

import (
	
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Conn struct {
	status runtime.Stage
	AddOrderErr error
	BalanceResult       *kraken.Balance
	TradeBalanceResult  *kraken.TradeBalanceResult
	TradesHistoryResult spot.TradesHistoryResult
	TradeVolumeResult   *kraken.TradeVolumeResult
	book                *spotbook.Book
	books               *sync.Map
	onExecution         func(*kraken.Execution)
	orderSeq            atomic.Int64
}

func (conn *Conn) Transition(stage runtime.Stage) { conn.status = stage }

func (conn *Conn) OnExecution(handler func(*kraken.Execution)) {
	conn.onExecution = handler
}

func (conn *Conn) EmitExecution(exec *kraken.Execution) {
	if conn.onExecution != nil {
		conn.onExecution(exec)
	}
}

func NewConn() *Conn {
	return &Conn{
		status: runtime.READY,
	}
}

func (conn *Conn) Close() error { return nil }
func (conn *Conn) Client() *spot.WebSocket { return nil }
func (conn *Conn) Status() runtime.Stage { return conn.status }
func (conn *Conn) SubInstrument(callback chan any) {
	if callback != nil {
		select {
		case callback <- true:
		default:
		}
	}
}

func (conn *Conn) SubTicker(symbols []string) {}
func (conn *Conn) SubTrades(symbols []string) {}
func (conn *Conn) SubL3(symbols []string) {}
func (conn *Conn) UnsubTicker(symbols []string) {}
func (conn *Conn) UnsubTrades(symbols []string) {}
func (conn *Conn) UnsubL3(symbols []string) {}

func (conn *Conn) Balance() (*kraken.Balance, error) {
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

func (conn *Conn) TradeBalance() (*kraken.TradeBalanceResult, error) {
	if conn.TradeBalanceResult != nil {
		return conn.TradeBalanceResult, nil
	}
	return &kraken.TradeBalanceResult{}, nil
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
	orderID := fmt.Sprintf("order-%d", conn.orderSeq.Add(1))
	return spot.AddOrderResult{
		OrderPlacementSingle: spot.OrderPlacementSingle{ID: []string{orderID}},
	}, nil
}

func (conn *Conn) OpenOrders() (spot.OpenOrdersResult, error) {
	return spot.OpenOrdersResult{}, nil
}

func (conn *Conn) CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error) {
	return spot.CancelResult{}, nil
}

func (conn *Conn) Write(json.Marshaler) error { return nil }
func (conn *Conn) Post(string, json.Marshaler) ([]byte, error) { return nil, nil }
func (conn *Conn) ApplyLevel3(data kraken.Level3Data) {}


func (conn *Conn) Book(symbol string, callback func(*spotbook.Book)) {}
