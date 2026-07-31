package types

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
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
	IsOpportunity bool             `json:"is_opportunity"`
	ReservationID string           `json:"reservation_id,omitempty"`
	Stoploss      *Stoploss        `json:"stoploss"`
}

/*
NewHolding constructs a pending Thesis lot with the Stoploss regulator Position
advances directly from live ticker updates.
*/
func NewHolding(
	ctx context.Context,
	symbol string,
	qty *decimal.Decimal,
	mark *decimal.Decimal,
) *Holding {
	errnie.Info("creating holding for: " + symbol)

	ctx, cancel := context.WithCancel(ctx)
	holding := &Holding{
		ctx:      ctx,
		cancel:   cancel,
		Symbol:   symbol,
		Qty:      qty,
		Status:   PENDING,
		Stoploss: NewStoploss(ctx, symbol, mark),
	}

	return holding
}

func (holding *Holding) Close() (err error) {
	holding.cancel()

	if holding.Stoploss != nil {
		err = holding.Stoploss.Close()
	}

	holding.Status = CLOSED

	return err
}
