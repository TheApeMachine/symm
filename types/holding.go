package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

/*
Holding records either wallet-backed inventory or a Thesis-local candidate.
Position orchestrates live holdings; a candidate remains on its originating
Thesis even when strategy declines to submit its proposed order.
*/
type Holding struct {
	Status        Status              `json:"status,omitempty"`
	Request       *spot.OrderRequest  `json:"request,omitempty"`
	Order         *spot.Order         `json:"order,omitempty"`
	Executions    []*kraken.Execution `json:"executions,omitempty"`
	Symbol        string              `json:"symbol"`
	Asset         string              `json:"asset,omitempty"`
	Qty           *decimal.Decimal    `json:"qty"`
	EntryAt       *time.Time          `json:"entry_at,omitempty"`
	ExitAt        *time.Time          `json:"exit_at,omitempty"`
	EntryPrice    *decimal.Decimal    `json:"entry_price"`
	EntryFee      *decimal.Decimal    `json:"entry_fee"`
	ExitPrice     *decimal.Decimal    `json:"exit_price"`
	ExitFee       *decimal.Decimal    `json:"exit_fee"`
	PnL           *decimal.Decimal    `json:"pnl"`
	ReturnPct     *float64            `json:"return_pct"`
	Mark          *decimal.Decimal    `json:"mark"`
	IsOpportunity bool                `json:"is_opportunity"`
}

/*
Update applies one execution print. Buys drive Qty/EntryPrice from exchange
CumQty/AvgPrice and accumulate fees; sells shrink Qty by LastQty and only mark
CLOSED when OrderStatus is filled or remaining base is exhausted.
*/
func (holding *Holding) Update(execution *kraken.ExecutionData) {
	holding.Status = MarketStatuses[execution.ExecType]

	if execution.ExecType != "trade" {
		return
	}

	if execution.Side == "buy" {
		holding.applyBuy(execution)
		return
	}

	if execution.Side == "sell" {
		holding.applySell(execution)
	}
}

/*
Closed reports whether inventory has been fully exited after fills.
*/
func (holding *Holding) Closed() bool {
	if holding == nil {
		return false
	}

	if holding.Status == CLOSED {
		return true
	}

	return holding.Qty != nil && holding.Qty.Sign() <= 0 &&
		holding.ExitAt != nil
}

func (holding *Holding) applyBuy(execution *kraken.ExecutionData) {
	holding.EntryAt = &execution.Timestamp
	holding.EntryPrice = execution.LastPrice.Copy()

	if execution.CumQty > 0 {
		holding.Qty = decimal.NewFromFloat64(execution.CumQty)

		if avg := averagePrice(execution); avg != nil {
			holding.EntryPrice = avg
		}
	} else if execution.LastQty > 0 {
		// First print without CumQty replaces the pre-submit requested size;
		// later prints accumulate until the exchange reports CumQty.
		if holding.EntryAt == nil {
			holding.Qty = decimal.NewFromFloat64(execution.LastQty)
		} else {
			holding.Qty = holding.addQty(execution.LastQty)
		}
	}

	holding.EntryFee = holding.addFee(holding.EntryFee, &execution.FeeUsdEquiv)
	holding.Status = OPEN
}

func (holding *Holding) applySell(execution *kraken.ExecutionData) {
	holding.ExitAt = &execution.Timestamp
	holding.ExitPrice = execution.LastPrice.Copy()

	if avg := averagePrice(execution); avg != nil {
		holding.ExitPrice = avg
	}

	holding.ExitFee = holding.addFee(holding.ExitFee, &execution.FeeUsdEquiv)

	if execution.LastQty > 0 && holding.Qty != nil {
		holding.Qty = holding.Qty.Sub(decimal.NewFromFloat64(execution.LastQty))
	}

	if execution.OrderStatus == "filled" ||
		(holding.Qty != nil && holding.Qty.Sign() <= 0) {
		holding.Status = CLOSED

		if holding.Qty == nil || holding.Qty.Sign() < 0 {
			holding.Qty = decimal.NewFromInt64(0)
		}

		return
	}

	holding.Status = OPEN
}

func (holding *Holding) addQty(lastQty float64) *decimal.Decimal {
	delta := decimal.NewFromFloat64(lastQty)

	if holding.Qty == nil {
		return delta
	}

	return holding.Qty.Add(delta)
}

func (holding *Holding) addFee(
	prior *decimal.Decimal,
	fee *decimal.Decimal,
) *decimal.Decimal {
	if fee == nil {
		return prior
	}

	copied := fee.Copy()

	if prior == nil {
		return copied
	}

	return decimal.NewFromFloat64(prior.Float64() + copied.Float64())
}

/*
averagePrice returns AvgPrice when the exchange supplied a usable value.
Zero-value decimals panic on Sign, so callers must use this guard.
*/
func averagePrice(execution *kraken.ExecutionData) *decimal.Decimal {
	if execution == nil {
		return nil
	}

	defer func() {
		recover()
	}()

	if execution.AvgPrice.Sign() <= 0 {
		return nil
	}

	return execution.AvgPrice.Copy()
}
