package types

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Holding is inventory qty and economics. Wallet lots live on Balance; Thesis
stores only holdings it created (Admit). Live Stoploss is owned by Position;
the Stoploss pointer here is the same regulator after Desk takes the lot.
*/
type Holding struct {
	ctx             context.Context
	cancel          context.CancelFunc
	Status          Status           `json:"status,omitempty"`
	Symbol          string           `json:"symbol"`
	Asset           string           `json:"asset,omitempty"`
	Qty             *decimal.Decimal `json:"qty" validate:"required"`
	SellableQty     *decimal.Decimal `json:"sellable_qty,omitempty"`
	EntryAt         *time.Time       `json:"entry_at,omitempty"`
	ExitAt          *time.Time       `json:"exit_at,omitempty"`
	EntryPrice      *decimal.Decimal `json:"entry_price"`
	EntryFee        *decimal.Decimal `json:"entry_fee"`
	ExitPrice       *decimal.Decimal `json:"exit_price"`
	ExitFee         *decimal.Decimal `json:"exit_fee"`
	PnL             *decimal.Decimal `json:"pnl"`
	ProfitThreshold *decimal.Decimal `json:"profit_threshold"`
	ReturnPct       float64          `json:"return_pct"`
	Mark            *decimal.Decimal `json:"mark"`
	IsOpportunity   bool             `json:"is_opportunity"`
	ReservationID   string           `json:"reservation_id,omitempty"`
	Stoploss        *Stoploss        `json:"stoploss"`
}

/*
NewHolding constructs a pending Thesis lot with the Stoploss regulator Position
advances from live executable marks.
*/
func NewHolding(
	ctx context.Context,
	symbol string,
	decision Decision,
) *Holding {
	errnie.Info("creating holding for: " + symbol)

	ctx, cancel := context.WithCancel(ctx)
	holding := &Holding{
		ctx:           ctx,
		cancel:        cancel,
		Symbol:        symbol,
		Qty:           decision.ProposedQuantity.Copy(),
		SellableQty:   decision.ProposedQuantity.Copy(),
		Mark:          decision.Mark.Copy(),
		Status:        PENDING,
		ReservationID: decision.ReservationID,
		IsOpportunity: decision.Opportunity,
		EntryAt:       decision.EntryAt,
		EntryPrice:    decision.EntryPrice.Copy(),
		EntryFee:      decision.EntryFee.Copy(),
		Stoploss: NewStoploss(
			ctx,
			symbol,
			decision.EntryPrice,
			decision.ProposedQuantity,
			decision.EntryFee,
			decision.Mark,
			decision.Risk,
		),

		/*
			ExitPrice, ExitFee and PnL are left absent rather than zeroed. The
			lot has not been sold, so there is no price it went out at, and a
			zero would read as one, and Position fills all three in from the
			execution that actually closes the lot.
		*/
	}

	/*
		A realised return only exists once something has been realised. The
		field is a plain float64 with no way to say "not yet", so an entry
		carries the zero it has actually returned so far, and the deref is
		guarded because an entry decision never carries one at all.
	*/
	if decision.ReturnPct != nil {
		holding.ReturnPct = *decision.ReturnPct
	}

	return holding
}

/*
Update refreshes the mark from the live book.

ProfitThreshold is read off the regulator rather than recomputed here. It used
to be assembled from EntryPrice + EntryFee + ExitFee, which could not protect an
open position for two reasons: the exit fee only exists once the lot has been
sold, so the branch never ran while it mattered, and the fees are totals for the
whole position while the entry price is per unit, so the sum was not a price at
all. The regulator solves the same question from liquidation economics and has
an answer from the moment the lot is filled.
*/
func (holding *Holding) Update(
	ticker kraken.TickerData,
) error {
	if ticker.Bid == nil {
		return nil
	}

	holding.Mark = ticker.Bid

	if holding.Stoploss != nil && holding.Stoploss.ProfitLine != nil {
		holding.ProfitThreshold = holding.Stoploss.ProfitLine.Copy()
	}

	return nil
}

/*
Observe forwards one executable observation to the regulator. It is separate
from Update because the price the stop judges is not the price on the ticker:
it is what selling this lot's quantity would actually realise, which only the
broker's price surface can derive.
*/
func (holding *Holding) Observe(evidence StopEvidence) {
	if holding == nil || holding.Stoploss == nil {
		return
	}

	holding.Stoploss.Observe(evidence)

	if holding.Stoploss.ProfitLine != nil {
		holding.ProfitThreshold = holding.Stoploss.ProfitLine.Copy()
	}
}

func (holding *Holding) Close() (err error) {
	holding.cancel()

	if holding.Stoploss != nil {
		err = holding.Stoploss.Close()
	}

	holding.Status = CLOSED

	return err
}
