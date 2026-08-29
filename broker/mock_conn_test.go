package broker

import (
	"encoding/json"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
mockConn is the broker test suite's null websocket.Conn: every method returns
an empty, successful default so a test only needs to embed it and override
the one method its scenario cares about.
*/
type mockConn struct {
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
}

func newMockConn() *mockConn {
	return &mockConn{status: runtime.READY}
}

func (conn *mockConn) Close() {}

func (conn *mockConn) Client() *spot.WebSocket { return nil }

func (conn *mockConn) Status() runtime.Stage { return conn.status }

func (conn *mockConn) SubInstrument(callback chan any) {}

func (conn *mockConn) SubTicker(symbols []string) {}

func (conn *mockConn) SubTrades(symbols []string) {}

func (conn *mockConn) SubL3(symbols []string) {}

func (conn *mockConn) Balance() (map[string]*decimal.Decimal, error) {
	if conn.BalanceResult != nil {
		return conn.BalanceResult, nil
	}

	return nil, nil
}

func (conn *mockConn) TradesHistory() (spot.TradesHistoryResult, error) {
	if conn.TradesHistoryResult.Trades != nil {
		return conn.TradesHistoryResult, nil
	}

	return spot.TradesHistoryResult{}, nil
}

func (conn *mockConn) TradeBalance() (kraken.TradeBalanceResult, error) {
	return kraken.TradeBalanceResult{}, nil
}

func (conn *mockConn) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	return &kraken.TradeVolumeResult{}, nil
}

func (conn *mockConn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	if conn.AddOrderErr != nil {
		return spot.AddOrderResult{}, conn.AddOrderErr
	}

	return spot.AddOrderResult{}, nil
}

func (conn *mockConn) OpenOrders() (spot.OpenOrdersResult, error) {
	return spot.OpenOrdersResult{}, nil
}

func (conn *mockConn) CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error) {
	return spot.CancelResult{}, nil
}

func (conn *mockConn) Write(json.Marshaler, ...websocket.Callback[any]) error { return nil }

func (conn *mockConn) Post(string, json.Marshaler) ([]byte, error) { return nil, nil }
