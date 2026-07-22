package types

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
)

/*
Holding is inventory qty and economics. Wallet lots live on Balance; Thesis
stores only holdings it created (Admit). Live Stoploss is owned by Position;
the Stoploss pointer here is the same regulator after Desk takes the lot.
*/
type Holding struct {
	ctx           context.Context
	cancel        context.CancelFunc
	Status        Status           `json:"status,omitempty"`
	Symbol        string           `json:"symbol"`
	Asset         string           `json:"asset,omitempty"`
	Qty           *decimal.Decimal `json:"qty" validate:"required"`
	SellableQty   *decimal.Decimal `json:"sellable_qty,omitempty"`
	EntryAt       *time.Time       `json:"entry_at,omitempty"`
	ExitAt        *time.Time       `json:"exit_at,omitempty"`
	EntryPrice    *decimal.Decimal `json:"entry_price"`
	EntryFee      *decimal.Decimal `json:"entry_fee"`
	ExitPrice     *decimal.Decimal `json:"exit_price"`
	ExitFee       *decimal.Decimal `json:"exit_fee"`
	PnL           *decimal.Decimal `json:"pnl"`
	ReturnPct     *float64         `json:"return_pct"`
	Mark          *decimal.Decimal `json:"mark"`
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
finite floats and stop_price is derived from the bound regulator.
*/
func (holding Holding) MarshalJSON() ([]byte, error) {
	if holding.Stoploss != nil {
		holding.Stoploss.RLock()
		defer holding.Stoploss.RUnlock()
	}

	frame := datura.Map[any]{
		"status":         holding.Status,
		"symbol":         holding.Symbol,
		"asset":          holding.Asset,
		"qty":            decimalFloat(holding.Qty),
		"sellable_qty":   decimalFloat(holding.SellableQty),
		"entry_at":       holding.EntryAt,
		"exit_at":        holding.ExitAt,
		"entry_price":    decimalFloat(holding.EntryPrice),
		"entry_fee":      decimalFloat(holding.EntryFee),
		"exit_price":     decimalFloat(holding.ExitPrice),
		"exit_fee":       decimalFloat(holding.ExitFee),
		"pnl":            decimalFloat(holding.PnL),
		"mark":           decimalFloat(holding.Mark),
		"is_opportunity": holding.IsOpportunity,
		"stoploss":       holding.Stoploss,
	}

	if holding.ReturnPct != nil {
		frame["return_pct"] = finiteFloat(*holding.ReturnPct)
	}

	if holding.Stoploss != nil {
		stopPrice := 0.0

		if holding.Stoploss.armed && holding.Stoploss.entry > 0 {
			stopPrice = finiteFloat(
				holding.Stoploss.entry * (1 + holding.Stoploss.StopReturn),
			)
		}

		if stopPrice > 0 {
			frame["stop_price"] = stopPrice
		}
	}

	return frame.Marshal(), nil
}

/*
StopFrame projects the live stop surface for the terminal gauge.
*/
func (holding *Holding) StopFrame() map[string]any {
	if holding == nil || holding.Stoploss == nil || holding.Symbol == "" {
		return nil
	}

	stop := holding.Stoploss
	stop.RLock()
	defer stop.RUnlock()
	stopPrice := 0.0

	if stop.armed && stop.entry > 0 {
		stopPrice = stop.entry * (1 + stop.StopReturn)
	}

	return map[string]any{
		"symbol":      holding.Symbol,
		"peak_return": finiteFloat(stop.PeakReturn),
		"stop_return": finiteFloat(stop.StopReturn),
		"armed":       stop.armed,
		"stop_price":  finiteFloat(stopPrice),
	}
}

func decimalFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	return finiteFloat(value.Float64())
}
