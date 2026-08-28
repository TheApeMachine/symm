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

	// Authoritative realized economics, separated so the journal can reconcile
	// fee-net exits against entry basis without overloading one field.
	EntryVWAP   *decimal.Decimal `json:"entry_vwap,omitempty"`
	ExitVWAP    *decimal.Decimal `json:"exit_vwap,omitempty"`
	EntryQty    *decimal.Decimal `json:"entry_qty,omitempty"`
	ExitQty     *decimal.Decimal `json:"exit_qty,omitempty"`
	EntryFees   *decimal.Decimal `json:"entry_fees,omitempty"`
	ExitFees    *decimal.Decimal `json:"exit_fees,omitempty"`
	RealizedPnL *decimal.Decimal `json:"realized_pnl,omitempty"`
	// RealizedReturn is the fee-inclusive percentage return derived from the
	// same realized economics that produce RealizedPnL, never from a ticker
	// mark or a current-price estimate.
	RealizedReturn *decimal.Decimal `json:"realized_return,omitempty"`
}

/*
NewHolding constructs a pending Thesis lot with the Stoploss regulator Position
advances from live executable marks.
*/
func NewHolding(
	ctx context.Context,
	symbol string,
	decision Decision,
) (*Holding, error) {
	errnie.Info("creating holding for: " + symbol)

	if decision.Stoploss == nil {
		return nil, errnie.Err(
			errnie.Validation,
			"holding: strategy stoploss required",
			nil,
		)
	}

	ctx, cancel := context.WithCancel(ctx)
	holding := &Holding{
		ctx:           ctx,
		cancel:        cancel,
		Symbol:        symbol,
		Qty:           decision.ProposedQuantity,
		SellableQty:   decision.ProposedQuantity,
		Mark:          decision.Mark,
		Status:        PENDING,
		ReservationID: decision.ReservationID,
		IsOpportunity: decision.Opportunity,
		EntryAt:       decision.EntryAt,
		EntryPrice:    decision.EntryPrice,
		EntryFee:      decision.EntryFee,
		Stoploss:      decision.Stoploss,
	}

	if decision.ReturnPct != nil {
		holding.ReturnPct = *decision.ReturnPct
	}

	return holding, nil
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
	holding.Stoploss.Update(holding.Mark)

	return nil
}

func (holding *Holding) Close() (err error) {
	if holding.cancel != nil {
		holding.cancel()
	}

	if holding.Stoploss != nil {
		err = holding.Stoploss.Close()
	}

	holding.Status = CLOSED

	return err
}
