package user

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/bytedance/sonic"
)

const executionSnapshot = "snapshot"

/*
ExecutionTokenSource supplies short-lived authenticated WebSocket tokens.
*/
type ExecutionTokenSource interface {
	Token(context.Context) (string, error)
}

/*
ExecutionParams is the Kraken WebSocket v2 subscribe payload for the executions channel.
*/
type ExecutionParams struct {
	Channel     string `json:"channel"`
	Token       string `json:"token"`
	SnapOrders  bool   `json:"snap_orders"`
	SnapTrades  bool   `json:"snap_trades"`
	OrderStatus bool   `json:"order_status"`
	RateCounter bool   `json:"ratecounter,omitempty"`
	Rebased     bool   `json:"rebased,omitempty"`
	Users       string `json:"users,omitempty"`
}

/*
ExecutionFee is one fee line on a trade execution event.
*/
type ExecutionFee struct {
	Asset string  `json:"asset"`
	Qty   float64 `json:"qty"`
}

/*
Execution is one order status or fill report from the executions channel.
*/
type Execution struct {
	OrderID      string         `json:"order_id,omitempty"`
	OrderUserref int            `json:"order_userref,omitempty"`
	ClOrdID      string         `json:"cl_ord_id,omitempty"`
	Symbol       string         `json:"symbol,omitempty"`
	Side         string         `json:"side,omitempty"`
	OrderType    string         `json:"order_type,omitempty"`
	OrderQty     float64        `json:"order_qty,omitempty"`
	LimitPrice   float64        `json:"limit_price,omitempty"`
	OrderStatus  string         `json:"order_status,omitempty"`
	ExecType     string         `json:"exec_type,omitempty"`
	ExecID       string         `json:"exec_id,omitempty"`
	TradeID      int64          `json:"trade_id,omitempty"`
	LastQty      float64        `json:"last_qty,omitempty"`
	LastPrice    float64        `json:"last_price,omitempty"`
	AvgPrice     float64        `json:"avg_price,omitempty"`
	CumQty       float64        `json:"cum_qty,omitempty"`
	CumCost      float64        `json:"cum_cost,omitempty"`
	Cost         float64        `json:"cost,omitempty"`
	LiquidityInd string         `json:"liquidity_ind,omitempty"`
	TimeInForce  string         `json:"time_in_force,omitempty"`
	FeeUsdEquiv  float64        `json:"fee_usd_equiv,omitempty"`
	FeeCcyPref   string         `json:"fee_ccy_pref,omitempty"`
	Fees         []ExecutionFee `json:"fees,omitempty"`
	Timestamp    string         `json:"timestamp,omitempty"`
	EnvelopeType string         `json:"-"`
}

func (execution *Execution) SetEnvelopeType(kind string) {
	execution.EnvelopeType = kind
}

func (execution *Execution) IsSnapshot() bool {
	return execution.EnvelopeType == executionSnapshot
}

/*
DecodeExecutions decodes every row in an executions channel message.
*/
func DecodeExecutions(message map[string]any) ([]Execution, error) {
	var executions []Execution

	if err := sonic.Unmarshal(message["data"].(json.RawMessage), &executions); err != nil {
		return nil, err
	}

	for index := range executions {
		executions[index].SetEnvelopeType(message["type"].(string))
	}

	return executions, nil
}

var (
	executionTokenSourceMu sync.RWMutex
	executionTokenSource   ExecutionTokenSource
)

func SetExecutionTokenSource(source ExecutionTokenSource) {
	executionTokenSourceMu.Lock()
	defer executionTokenSourceMu.Unlock()

	executionTokenSource = source
}

func ExecutionAvailable() bool {
	executionTokenSourceMu.RLock()
	defer executionTokenSourceMu.RUnlock()

	return executionTokenSource != nil
}
