package types

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Holding is inventory qty and economics. Wallet lots live on Balance; Thesis
stores only holdings it created (Admit). Live Stoploss is owned by Position;
the Stoploss pointer here is the same regulator after Desk takes the lot.
*/
type Holding struct {
	ctx    context.Context
	cancel context.CancelFunc
	Status Status           `json:"status,omitempty"`
	Symbol string           `json:"symbol"`
	Asset  string           `json:"asset,omitempty"`
	Qty    *decimal.Decimal `json:"qty" validate:"required"`
	// SellableQty is exchange Available (unreserved). Qty is total Balance.
	SellableQty *decimal.Decimal `json:"sellable_qty,omitempty"`
	EntryAt     *time.Time       `json:"entry_at,omitempty"`
	ExitAt      *time.Time       `json:"exit_at,omitempty"`
	EntryPrice  *decimal.Decimal `json:"entry_price"`
	EntryFee    *decimal.Decimal `json:"entry_fee"`
	ExitPrice   *decimal.Decimal `json:"exit_price"`
	ExitFee     *decimal.Decimal `json:"exit_fee"`
	PnL         *decimal.Decimal `json:"pnl"`
	ReturnPct   *float64         `json:"return_pct"`
	Mark        *decimal.Decimal `json:"mark"`
	// StopMark is mid/last for stop geometry; Mark stays executable bid for PnL.
	StopMark      *decimal.Decimal `json:"stop_mark,omitempty"`
	IsOpportunity bool             `json:"is_opportunity"`
	Stoploss      *Stoploss        `json:"stoploss"`
}

/*
NewHolding constructs a pending Thesis lot with a Stoploss Position will own
after Desk entry.
*/
func NewHolding(
	ctx context.Context,
	symbol string,
	qty *decimal.Decimal,
) *Holding {
	ctx, cancel := context.WithCancel(ctx)

	return &Holding{
		ctx:      ctx,
		cancel:   cancel,
		Symbol:   symbol,
		Qty:      qty,
		Status:   PENDING,
		Stoploss: NewStoploss(ctx),
	}
}

/*
MarshalJSON encodes a JSON-safe desk/thesis surface. Decimal fields become
finite floats, ReturnPct is sanitized, and stop_price is derived from the
bound regulator so the websocket never sees Inf/NaN from LockedFloor.
*/
func (holding Holding) MarshalJSON() ([]byte, error) {
	type wire struct {
		Status        Status     `json:"status,omitempty"`
		Symbol        string     `json:"symbol"`
		Asset         string     `json:"asset,omitempty"`
		Qty           float64    `json:"qty"`
		SellableQty   float64    `json:"sellable_qty,omitempty"`
		EntryAt       *time.Time `json:"entry_at,omitempty"`
		ExitAt        *time.Time `json:"exit_at,omitempty"`
		EntryPrice    float64    `json:"entry_price"`
		EntryFee      float64    `json:"entry_fee"`
		ExitPrice     float64    `json:"exit_price"`
		ExitFee       float64    `json:"exit_fee"`
		PnL           float64    `json:"pnl"`
		ReturnPct     float64    `json:"return_pct"`
		Mark          float64    `json:"mark"`
		IsOpportunity bool       `json:"is_opportunity,omitempty"`
		StopPrice     float64    `json:"stop_price,omitempty"`
		Stoploss      *Stoploss  `json:"stoploss,omitempty"`
	}

	frame := wire{
		Status:        holding.Status,
		Symbol:        holding.Symbol,
		Asset:         holding.Asset,
		Qty:           decimalFloat(holding.Qty),
		SellableQty:   decimalFloat(holding.SellableQty),
		EntryAt:       holding.EntryAt,
		ExitAt:        holding.ExitAt,
		EntryPrice:    decimalFloat(holding.EntryPrice),
		EntryFee:      decimalFloat(holding.EntryFee),
		ExitPrice:     decimalFloat(holding.ExitPrice),
		ExitFee:       decimalFloat(holding.ExitFee),
		PnL:           decimalFloat(holding.PnL),
		Mark:          decimalFloat(holding.Mark),
		IsOpportunity: holding.IsOpportunity,
		Stoploss:      holding.Stoploss,
	}

	if holding.ReturnPct != nil {
		frame.ReturnPct = finiteFloat(*holding.ReturnPct)
	}

	if holding.Stoploss != nil {
		if stopPrice := holding.Stoploss.StopPrice(); stopPrice > 0 {
			frame.StopPrice = stopPrice
		}
	}

	return sonic.Marshal(frame)
}

/*
StopFrame projects the live stop surface for the terminal gauge.
*/
func (holding *Holding) StopFrame() map[string]any {
	if holding == nil || holding.Stoploss == nil || holding.Symbol == "" {
		return nil
	}

	stop := holding.Stoploss

	return map[string]any{
		"symbol":      holding.Symbol,
		"peak_return": finiteFloat(stop.PeakReturn),
		"stop_return": finiteFloat(stop.StopReturn),
		"armed":       stop.Armed(),
		"stop_price":  stop.StopPrice(),
	}
}

func decimalFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	return finiteFloat(value.Float64())
}
