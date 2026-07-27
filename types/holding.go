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
NewHolding constructs a pending Thesis lot with a Stoploss Position will own
after Desk entry. market is the upstream market actor for the stoploss ticker subscription.
*/
func NewHolding(
	ctx context.Context,
	symbol string,
	qty *decimal.Decimal,
	mark *decimal.Decimal,
	exit func() error,
	onChange func(),
	market *Actor,
) *Holding {
	errnie.Info("creating holding for: " + symbol)

	ctx, cancel := context.WithCancel(ctx)
	holding := &Holding{
		ctx:      ctx,
		cancel:   cancel,
		Symbol:   symbol,
		Qty:      qty,
		Status:   PENDING,
		Stoploss: NewStoploss(ctx, symbol, mark, exit, onChange, market),
	}

	return holding
}

/*
Initialize the Holding if we are "recovering" after a restart.
market is the upstream market actor for the stoploss ticker subscription.
*/
func (holding *Holding) Initialize(
	ctx context.Context,
	qty *decimal.Decimal,
	mark *decimal.Decimal,
	exit func() error,
	onChange func(),
	market *Actor,
) {
	holding.ctx = ctx
	holding.Qty = qty
	holding.Status = READY
	holding.Stoploss.Initialize(ctx, mark, exit, onChange, market)
}

func (holding *Holding) Close() (err error) {
	holding.cancel()

	if holding.Stoploss != nil {
		err = holding.Stoploss.Close()
	}

	holding.Status = CLOSED

	return err
}
