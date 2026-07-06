package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

type Execution struct {
	Channel   string          `json:"channel"`
	Type      string          `json:"type"`
	Data      []ExecutionData `json:"data"`
	Sequence  int64           `json:"sequence"`
	Timestamp time.Time       `json:"timestamp"`
}

type ExecutionData struct {
	Amended          bool      `json:"amended"`
	AvgPrice         float64   `json:"avg_price"`
	CashOrderQty     float64   `json:"cash_order_qty"`
	ClientOrderID    string    `json:"cl_ord_id"`
	Cost             float64   `json:"cost"`
	CumCost          float64   `json:"cum_cost"`
	CumQty           float64   `json:"cum_qty"`
	DisplayQty       float64   `json:"display_qty"`
	DisplayQtyRemain float64   `json:"display_qty_remain"`
	EffectiveTime    time.Time `json:"effective_time"`
	ExecID           string    `json:"exec_id"`
	ExecType         string    `json:"exec_type"`
	ExpireTime       time.Time `json:"expire_time"`
	ExtOrdID         string    `json:"ext_ord_id"`
	ExtExecID        string    `json:"ext_exec_id"`
	Fee              float64   `json:"fee"`
	FeeCcy           string    `json:"fee_ccy"`
	FeeCcyPref       string    `json:"fee_ccy_pref"`
	FeeUSDEquiv      float64   `json:"fee_usd_equiv"`
	Fees             []struct {
		Asset string  `json:"asset"`
		Qty   float64 `json:"qty"`
	} `json:"fees"`
	LimitPrice     float64   `json:"limit_price"`
	LimitPriceType string    `json:"limit_price_type"`
	Liquidated     bool      `json:"liquidated"`
	LiquidityInd   string    `json:"liquidity_ind"`
	LastPrice      float64   `json:"last_price"`
	LastQty        float64   `json:"last_qty"`
	Margin         bool      `json:"margin"`
	MarginBorrow   bool      `json:"margin_borrow"`
	NoMPP          bool      `json:"no_mpp"`
	OrdRefID       string    `json:"ord_ref_id"`
	OrderID        string    `json:"order_id"`
	OrderQty       float64   `json:"order_qty"`
	OrderStatus    string    `json:"order_status"`
	OrderType      string    `json:"order_type"`
	OrderUserRef   int64     `json:"order_userref"`
	PostOnly       bool      `json:"post_only"`
	PositionStatus string    `json:"position_status"`
	Reason         string    `json:"reason"`
	ReduceOnly     bool      `json:"reduce_only"`
	SenderSubID    string    `json:"sender_sub_id"`
	Side           string    `json:"side"`
	StopPrice      float64   `json:"stop_price"`
	Symbol         string    `json:"symbol"`
	TimeInForce    string    `json:"time_in_force"`
	Timestamp      time.Time `json:"timestamp"`
	TradeID        int64     `json:"trade_id"`
	User           string    `json:"user"`
	CancelReason   string    `json:"cancel_reason"`
	Trigger        string    `json:"trigger"`
	TriggeredPrice float64   `json:"triggered_price"`
	Contingent     struct {
		OrderType        string  `json:"order_type"`
		TriggerPrice     float64 `json:"trigger_price"`
		TriggerPriceType string  `json:"trigger_price_type"`
		LimitPrice       float64 `json:"limit_price"`
		LimitPriceType   string  `json:"limit_price_type"`
	} `json:"contingent"`
	Triggers struct {
		Reference   string    `json:"reference"`
		Price       float64   `json:"price"`
		PriceType   string    `json:"price_type"`
		ActualPrice float64   `json:"actual_price"`
		PeakPrice   float64   `json:"peak_price"`
		LastPrice   float64   `json:"last_price"`
		Status      string    `json:"status"`
		Timestamp   time.Time `json:"timestamp"`
	} `json:"triggers"`
}

type ExecutionDataSlice []ExecutionData

func NewExecutionDataSlice(buf []byte) *ExecutionDataSlice {
	data := &ExecutionDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}
