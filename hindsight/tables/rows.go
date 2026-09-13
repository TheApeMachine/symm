package tables

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

// SpotLevel3Row is one order add/modify/delete event on spot level 3.
type SpotLevel3Row struct {
	Epoch      int64     `json:"epoch"`
	Tick       int64     `json:"tick"`
	Symbol     string    `json:"symbol"`
	VenueAt    time.Time `json:"venueAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Side       string    `json:"side"` // "bid" or "ask"
	Event      string    `json:"event"` // "add", "modify", "delete"
	OrderID    string    `json:"orderId"`
	LimitPrice float64   `json:"limitPrice"`
	OrderQty   float64   `json:"orderQty"`
	Checksum   int64     `json:"checksum"`
}

// SpotTickerRow is one spot ticker update containing full venue facts.
type SpotTickerRow struct {
	Epoch      int64     `json:"epoch"`
	Tick       int64     `json:"tick"`
	Symbol     string    `json:"symbol"`
	VenueAt    time.Time `json:"venueAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Bid        float64   `json:"bid"`
	BidQty     float64   `json:"bidQty"`
	Ask        float64   `json:"ask"`
	AskQty     float64   `json:"askQty"`
	Last       float64   `json:"last"`
	Volume     float64   `json:"volume"`
	VWAP       float64   `json:"vwap"`
	Low        float64   `json:"low"`
	High       float64   `json:"high"`
	Change     float64   `json:"change"`
	ChangePct  float64   `json:"changePct"`
}

// SpotTradeRow is one executed spot trade on venue.
type SpotTradeRow struct {
	Epoch      int64     `json:"epoch"`
	Tick       int64     `json:"tick"`
	Symbol     string    `json:"symbol"`
	VenueAt    time.Time `json:"venueAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Price      float64   `json:"price"`
	Qty        float64   `json:"qty"`
	Side       string    `json:"side"`
	OrdType    string    `json:"ordType"`
	TradeID    int64     `json:"tradeId"`
}

// FuturesTickerRow is one futures ticker update with derivative marks.
type FuturesTickerRow struct {
	Epoch        int64     `json:"epoch"`
	Tick         int64     `json:"tick"`
	Symbol       string    `json:"symbol"`
	VenueAt      time.Time `json:"venueAt"`
	ReceivedAt   time.Time `json:"receivedAt"`
	Bid          float64   `json:"bid"`
	BidQty       float64   `json:"bidQty"`
	Ask          float64   `json:"ask"`
	AskQty       float64   `json:"askQty"`
	Last         float64   `json:"last"`
	Volume       float64   `json:"volume"`
	VWAP         float64   `json:"vwap"`
	Low          float64   `json:"low"`
	High         float64   `json:"high"`
	Change       float64   `json:"change"`
	ChangePct    float64   `json:"changePct"`
	MarkPrice    float64   `json:"markPrice"`
	IndexPrice   float64   `json:"indexPrice"`
	OpenInterest float64   `json:"openInterest"`
}

// FuturesTradeRow is one executed futures contract trade.
type FuturesTradeRow struct {
	Epoch      int64     `json:"epoch"`
	Tick       int64     `json:"tick"`
	Symbol     string    `json:"symbol"`
	VenueAt    time.Time `json:"venueAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Price      float64   `json:"price"`
	Qty        float64   `json:"qty"`
	Side       string    `json:"side"`
	OrdType    string    `json:"ordType"`
	TradeID    int64     `json:"tradeId"`
}

// ExecutionRow captures one venue execution update (channel: executions).
type ExecutionRow struct {
	Epoch        int64            `json:"epoch"`
	Tick         int64            `json:"tick"`
	Symbol       string           `json:"symbol"`
	VenueAt      time.Time        `json:"venueAt"`
	ReceivedAt   time.Time        `json:"receivedAt"`
	OrderID      string           `json:"orderId"`
	OrderUserRef int64            `json:"orderUserref"`
	ExecID       string           `json:"execId"`
	ExecType     string           `json:"execType"`
	TradeID      int64            `json:"tradeId"`
	Side         string           `json:"side"`
	LastQty      *decimal.Decimal `json:"lastQty"`
	LastPrice    *decimal.Decimal `json:"lastPrice"`
	LiquidityInd string           `json:"liquidityInd"`
	Cost         *decimal.Decimal `json:"cost"`
	OrderType    string           `json:"orderType"`
	OrderStatus  string           `json:"orderStatus"`
	CumQty       *decimal.Decimal `json:"cumQty"`
	CumCost      *decimal.Decimal `json:"cumCost"`
	AvgPrice     *decimal.Decimal `json:"avgPrice"`
	FeeUsdEquiv  *decimal.Decimal `json:"feeUsdEquiv"`
	Fees         string           `json:"fees"`
}

// MeasurementRow captures one canonical *data.Measurement[float64].
type MeasurementRow struct {
	Epoch      int64              `json:"epoch"`
	Tick       int64              `json:"tick"`
	Source     string             `json:"source"`
	Symbol     string             `json:"symbol"`
	VenueAt    time.Time          `json:"venueAt"`
	ObservedAt time.Time          `json:"observedAt"`
	Maturity   float64            `json:"maturity"`
	SNR        float64            `json:"snr"`
	SNRDefined bool               `json:"snrDefined"`
	Metrics    map[string]float64 `json:"metrics"`
	Metadata   map[string]float64 `json:"metadata"`
	Payload    []byte             `json:"payload,omitempty"`
}

// ModelRow is an atomic point-in-time snapshot of the trained cognitive radix trie.
type ModelRow struct {
	Epoch      int64  `json:"epoch"`
	Tick       int64  `json:"tick"`
	AgentID    int32  `json:"agentId"`
	StepCount  int64  `json:"stepCount"`
	NodesCount int64  `json:"nodesCount"`
	Payload    []byte `json:"payload"`
}

// GridRow is an atomic point-in-time snapshot of the associative perception grid.
type GridRow struct {
	Epoch        int64  `json:"epoch"`
	Tick         int64  `json:"tick"`
	AgentID      int32  `json:"agentId"`
	ContextLabel string `json:"contextLabel"`
	Payload      []byte `json:"payload"`
}

// PositionRow records one position transition with authoritative inventory and basis.
type PositionRow struct {
	Epoch       int64            `json:"epoch"`
	Tick        int64            `json:"tick"`
	Symbol      string           `json:"symbol"`
	Status      string           `json:"status"`
	Qty         *decimal.Decimal `json:"qty"`
	Basis       *decimal.Decimal `json:"basis,omitempty"`
	EntryPrice  *decimal.Decimal `json:"entryPrice,omitempty"`
	EntryFee    *decimal.Decimal `json:"entryFee,omitempty"`
	ExitPrice   *decimal.Decimal `json:"exitPrice,omitempty"`
	ExitFee     *decimal.Decimal `json:"exitFee,omitempty"`
	Mark        *decimal.Decimal `json:"mark,omitempty"`
	PnL         *decimal.Decimal `json:"pnl,omitempty"`
	RealizedPnL *decimal.Decimal `json:"realizedPnl,omitempty"`
	EntryAt     *time.Time       `json:"entryAt,omitempty"`
	ExitAt      *time.Time       `json:"exitAt,omitempty"`
}

// OutcomeRow records one agent action decision and its post-hoc outcome.
type OutcomeRow struct {
	Epoch        int64     `json:"epoch"`
	Tick         int64     `json:"tick"`
	DecisionID   int64     `json:"decisionId"`
	Symbol       string    `json:"symbol"`
	At           time.Time `json:"at"`
	ActionKind   string    `json:"actionKind"`
	ActionPower  int32     `json:"actionPower"`
	ActionReduce bool      `json:"actionReduce"`
	Authority    float64   `json:"authority"`
	Outcome      *float64  `json:"outcome,omitempty"`
}
