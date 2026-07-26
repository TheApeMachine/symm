package types

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

// Freeze the configuration once at startup to reuse JIT-compiled encoders.
var fastSonic = sonic.Config{
	EncodeNullForInfOrNan: true, // Converts NaN, +Inf, -Inf to JSON `null` instead of returning an error
}.Froze()

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
	ReservationID string           `json:"reservation_id,omitempty"`
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
	mark *decimal.Decimal,
	exit func() error,
) *Holding {
	ctx, cancel := context.WithCancel(ctx)
	holding := &Holding{
		ctx:      ctx,
		cancel:   cancel,
		Symbol:   symbol,
		Qty:      qty,
		Status:   PENDING,
		Stoploss: NewStoploss(ctx, symbol, mark, exit),
	}

	return holding
}

/*
MarshalJSON encodes a JSON-safe desk/thesis surface. Decimal fields become
finite floats and stop_price is derived from the bound regulator.
*/
func (holding Holding) MarshalJSON() ([]byte, error) {
	type alias Holding
	return fastSonic.Marshal(alias(holding))
}

/*
UnmarshalJSON decodes a JSON-safe desk/thesis surface. Decimal fields become
finite floats and stop_price is derived from the bound regulator.
*/
func (holding *Holding) UnmarshalJSON(data []byte) error {
	type alias Holding
	return fastSonic.Unmarshal(data, holding)
}

func (holding *Holding) Close() {
	if holding.cancel != nil {
		holding.cancel()
	}

	holding.Stoploss.Close()
	holding.Status = CLOSED
}
