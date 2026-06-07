package response

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

const (
	noticeFill = "paper:fill"
	noticeArm  = "paper:arm"
)

/*
FillNotice is an internal observer payload from Orders to Executions.
*/
type FillNotice struct {
	Params       trading.AddParams
	OrderID      string
	Price        float64
	Fee          float64
	Reason       string
	LiquidityInd string
	Maker        bool
	Partial      bool
}

/*
ArmNotice is an internal observer payload when a protective order rests.
*/
type ArmNotice struct {
	Params  trading.AddParams
	OrderID string
}

/*
Executions simulates the Kraken executions channel and publishes the same raw
frames and derived envelopes as the live private websocket.
*/
type Executions struct {
	balances *Balances
	ids      *Identifier
	raw      *qpool.BroadcastGroup
	sequence int64
	tradeID  int64
}

func NewExecutions(
	raw *qpool.BroadcastGroup,
	balances *Balances,
	ids *Identifier,
) *Executions {
	return &Executions{
		balances: balances,
		ids:      ids,
		raw:      raw,
	}
}

func (executions *Executions) Send(message *qpool.QValue[any]) map[string]any {
	switch message.Type {
	case noticeFill:
		notice, ok := message.Value.(FillNotice)

		if ok {
			executions.publishFill(notice)
		}

		return nil
	case noticeArm:
		notice, ok := message.Value.(ArmNotice)

		if ok {
			executions.publishArmed(notice)
		}

		return nil
	}

	switch frame := message.Value.(type) {
	case user.ExecutionSubscribeFrame:
		return executions.subscribeAck(frame)
	case map[string]any:
		if frame["method"] == "subscribe" {
			return executions.subscribeAckMap(frame)
		}
	}

	return nil
}

func (executions *Executions) Observe(_ types.Socket) {}

func (executions *Executions) subscribeAck(frame user.ExecutionSubscribeFrame) map[string]any {
	user.PublishExecutionsRaw(executions.raw, user.ExecutionSnapshot, nil)

	return map[string]any{
		"method":   frame.Method,
		"success":  true,
		"result":   map[string]any{"channel": "executions", "snap_orders": frame.Params.SnapOrders, "snap_trades": frame.Params.SnapTrades},
		"time_out": time.Now(),
	}
}

func (executions *Executions) subscribeAckMap(frame map[string]any) map[string]any {
	user.PublishExecutionsRaw(executions.raw, user.ExecutionSnapshot, nil)

	return map[string]any{
		"method":   frame["method"],
		"req_id":   frame["req_id"],
		"success":  true,
		"time_in":  frame["time_in"],
		"time_out": time.Now(),
		"result":   map[string]any{"channel": "executions", "snap_orders": true, "snap_trades": true},
	}
}

func (executions *Executions) publishArmed(notice ArmNotice) {
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	params := notice.Params

	pending := user.Execution{
		OrderID:     notice.OrderID,
		ClOrdID:     params.ClOrdID,
		Symbol:      params.Symbol,
		Side:        string(params.Side),
		OrderType:   string(params.OrderType),
		OrderQty:    params.OrderQty,
		ExecType:    "pending_new",
		OrderStatus: "pending_new",
		Timestamp:   stamp,
	}

	live := pending
	live.ExecType = "new"
	live.OrderStatus = "new"

	user.PublishExecutionsRaw(executions.raw, "update", []user.Execution{pending})
	user.PublishExecutionsRaw(executions.raw, "update", []user.Execution{live})
}

func (executions *Executions) publishFill(notice FillNotice) {
	if notice.Reason != "" {
		user.PublishExecutionRejectDerived(
			executions.raw, notice.Params.Symbol, string(notice.Params.Side), notice.Reason,
		)

		return
	}

	execID := executions.ids.ExecID()

	if executions.balances != nil {
		if err := executions.balances.ApplyFill(
			notice.Params.Symbol,
			string(notice.Params.Side),
			notice.Params.OrderQty,
			notice.Price,
			notice.Fee,
			execID,
		); err != nil {
			user.PublishExecutionRejectDerived(
				executions.raw, notice.Params.Symbol, string(notice.Params.Side), err.Error(),
			)

			return
		}
	}

	executions.emitTrade(notice, execID)
}

func (executions *Executions) emitTrade(notice FillNotice, execID string) {
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	params := notice.Params
	cost := params.OrderQty * notice.Price
	tradeID := atomic.AddInt64(&executions.tradeID, 1)
	feeAsset, err := quoteAsset(params.Symbol)

	if err != nil {
		user.PublishExecutionRejectDerived(
			executions.raw, params.Symbol, string(params.Side), err.Error(),
		)

		return
	}

	orderStatus := "filled"

	if notice.Partial {
		orderStatus = "partially_filled"
	}

	trade := user.Execution{
		OrderID:      notice.OrderID,
		ClOrdID:      params.ClOrdID,
		Symbol:       params.Symbol,
		Side:         string(params.Side),
		OrderType:    string(params.OrderType),
		OrderQty:     params.OrderQty,
		ExecType:     "trade",
		ExecID:       execID,
		TradeID:      tradeID,
		LastQty:      params.OrderQty,
		LastPrice:    notice.Price,
		AvgPrice:     notice.Price,
		CumQty:       params.OrderQty,
		CumCost:      cost,
		Cost:         cost,
		LiquidityInd: notice.LiquidityInd,
		OrderStatus:  orderStatus,
		Fees:         []user.ExecutionFee{{Asset: feeAsset, Qty: notice.Fee}},
		Timestamp:    stamp,
	}

	filled := trade
	filled.ExecType = "filled"

	if notice.Partial {
		filled.OrderStatus = "partially_filled"
	}

	user.PublishExecutionsRaw(executions.raw, "update", []user.Execution{trade})
	user.PublishExecutionsRaw(executions.raw, "update", []user.Execution{filled})
}

func quoteAsset(symbol string) (string, error) {
	slash := strings.IndexByte(symbol, '/')

	if slash < 0 || slash >= len(symbol)-1 {
		return "", fmt.Errorf("paper fill: malformed symbol %q", symbol)
	}

	return symbol[slash+1:], nil
}
