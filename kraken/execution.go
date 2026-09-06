package kraken

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type Execution struct {
	Channel  string          `json:"channel"`
	Type     string          `json:"type"`
	Data     []ExecutionData `json:"data"`
	Sequence int             `json:"sequence"`
}

/*
ExecutionSubscription requests authenticated order and fill updates through
the injected websocket transport.
*/
type ExecutionSubscription struct {
	Token string
}

/*
NewExecutionSubscription binds the current authenticated websocket token.
*/
func NewExecutionSubscription(token string) ExecutionSubscription {
	return ExecutionSubscription{Token: token}
}

/*
MarshalJSON encodes the execution snapshots needed to reconstruct order state.
*/
func (subscription ExecutionSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":     "executions",
			"snap_orders": true,
			"snap_trades": true,
			"token":       subscription.Token,
		},
	})
}

type ExecutionData struct {
	OrderID       string           `json:"order_id"`
	ClientOrderID string           `json:"cl_ord_id"`
	OrderUserref  int              `json:"order_userref"`
	ExecID        string           `json:"exec_id"`
	ExecType      string           `json:"exec_type"`
	TradeID       int              `json:"trade_id"`
	Symbol        string           `json:"symbol"`
	Side          string           `json:"side"`
	LastQty       *decimal.Decimal `json:"last_qty"`
	LastPrice     *decimal.Decimal `json:"last_price"`
	LiquidityInd  string           `json:"liquidity_ind"`
	Cost          *decimal.Decimal `json:"cost"`
	OrderType     string           `json:"order_type"`
	Timestamp     time.Time        `json:"timestamp"`
	OrderStatus   string           `json:"order_status"`
	CumQty        *decimal.Decimal `json:"cum_qty"`
	CumCost       *decimal.Decimal `json:"cum_cost"`
	AvgPrice      *decimal.Decimal `json:"avg_price"`
	FeeUsdEquiv   *decimal.Decimal `json:"fee_usd_equiv"`
	Fees          []ExecutionFee   `json:"fees"`
}

type ExecutionFee struct {
	Asset string  `json:"asset"`
	Qty   float64 `json:"qty"`
}

func NewExecution(buf []byte) *Execution {
	frame := &Execution{}
	errnie.Error(sonic.Unmarshal(buf, frame))
	return frame
}

func (execution *Execution) MarshalJSON() ([]byte, error) {
	type alias Execution
	return sonic.Marshal((*alias)(execution))
}

func (execution *Execution) Action() string {
	return execution.Channel
}

func (execution *Execution) IsSuccess() bool {
	return len(execution.Data) > 0
}

func NewExecutionFromMap(model datura.Map[any]) *Execution {
	execType := "trade"
	orderStatus := "filled"

	if action, ok := model["action"].(string); ok {
		if action == "limit_order_placed" {
			execType = "new"
			orderStatus = "open"
		}
	}

	if status, ok := model["status"].(string); ok {
		orderStatus = status
	}

	orderID, _ := model["order_id"].(string)
	clientOrderID, _ := model["cl_ord_id"].(string)
	execID, _ := model["trade_id"].(string)

	if execID == "" {
		execID, _ = model["id"].(string)
	}

	pair, _ := model["pair"].(string)
	side, _ := model["side"].(string)
	volume, _ := model["volume"].(float64)
	price, _ := model["price"].(float64)
	cost, _ := model["cost"].(float64)
	fee, _ := model["fee"].(float64)

	timestamp := time.Now()

	if timeRaw, ok := model["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, timeRaw); err == nil {
			timestamp = parsed
		}
	}

	return &Execution{
		Channel: "executions",
		Type:    "update",
		Data: []ExecutionData{{
			OrderID:       orderID,
			ClientOrderID: clientOrderID,
			ExecID:        execID,
			ExecType:      execType,
			Symbol:        pair,
			Side:          strings.ToLower(side),
			LastQty:       decimal.NewFromFloat64(volume),
			LastPrice:     decimal.NewFromFloat64(price),
			Cost:          decimal.NewFromFloat64(cost),
			OrderStatus:   orderStatus,
			CumQty:        decimal.NewFromFloat64(volume),
			CumCost:       decimal.NewFromFloat64(cost),
			AvgPrice:      decimal.NewFromFloat64(price),
			FeeUsdEquiv:   decimal.NewFromFloat64(fee),
			Timestamp:     timestamp,
		}},
	}
}
