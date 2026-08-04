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
advances directly from live ticker updates.
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
		Mark:          decision.Mark.Copy(),
		Status:        PENDING,
		ReservationID: decision.ReservationID,
		IsOpportunity: decision.Opportunity,
		Stoploss:      NewStoploss(ctx, symbol, decision.Mark),
	}

	return holding
}

func (holding *Holding) Update(
	ticker kraken.TickerData,
) error {
	if ticker.Bid == nil {
		return nil
	}

	holding.Mark = ticker.Bid

	if holding.EntryPrice != nil && holding.EntryFee != nil && holding.ExitFee != nil {
		// Calculate the amount that puts the position into profit.
		holding.ProfitThreshold = holding.EntryPrice.Add(
			holding.EntryFee,
		).Add(
			holding.ExitFee,
		)
	}

	if holding.Stoploss != nil {
		if err := holding.Stoploss.Update(ticker); err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"holding: could not update stoploss",
				err,
			))
		}
	}

	return nil
}

func (holding *Holding) Close() (err error) {
	holding.cancel()

	if holding.Stoploss != nil {
		err = holding.Stoploss.Close()
	}

	holding.Status = CLOSED

	return err
}
