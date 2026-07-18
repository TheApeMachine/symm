package types

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Holding records either wallet-backed inventory or a Thesis-local candidate.
Position orchestrates live holdings; a candidate remains on its originating
Thesis even when strategy declines to submit its proposed order.
*/
type Holding struct {
	ctx           context.Context
	cancel        context.CancelFunc
	Status        Status           `json:"status,omitempty"`
	Symbol        string           `json:"symbol"`
	Asset         string           `json:"asset,omitempty"`
	Qty           *decimal.Decimal `json:"qty" validate:"required"`
	EntryAt       *time.Time       `json:"entry_at,omitempty"`
	ExitAt        *time.Time       `json:"exit_at,omitempty"`
	EntryPrice    *decimal.Decimal `json:"entry_price"`
	EntryFee      *decimal.Decimal `json:"entry_fee"`
	ExitPrice     *decimal.Decimal `json:"exit_price"`
	ExitFee       *decimal.Decimal `json:"exit_fee"`
	PnL           *decimal.Decimal `json:"pnl"`
	ReturnPct     *float64         `json:"return_pct"`
	Mark          *decimal.Decimal `json:"mark"`
	IsOpportunity bool             `json:"is_opportunity"`
	Stoploss      *Stoploss        `json:"stoploss"`
}

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
