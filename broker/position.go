package broker

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	mu              sync.RWMutex
	price           *Price
	ID              string                  `json:"id"`
	EntryPath       string                  `json:"entry_path"`
	ExitPath        string                  `json:"exit_path"`
	Symbol          string                  `json:"symbol"`
	OpenedAt        time.Time               `json:"opened_at"`
	ClosedAt        *time.Time              `json:"closed_at"`
	EntryTick       uint64                  `json:"entry_tick,omitempty"`
	ExitTick        uint64                  `json:"exit_tick,omitempty"`
	EntryOrders     []*spot.OrderRequest    `json:"entry_orders,omitempty"`
	ExitOrders      []*spot.OrderRequest    `json:"exit_orders,omitempty"`
	Fills           []*kraken.ExecutionData `json:"fills,omitempty"`
	Qty             *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice      *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice       *decimal.Decimal        `json:"exit_price,omitempty"`
	EntryFee        *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee         *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark            *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP        *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom        *float64                `json:"exit_room,omitempty"`
	Return          *float64                `json:"return,omitempty"`
	HoldNs          int64                   `json:"hold_ns,omitempty"`
	Virtual         bool                    `json:"virtual,omitempty"`
	Cost            *decimal.Decimal        `json:"cost,omitempty"`
	Proceeds        *decimal.Decimal        `json:"proceeds,omitempty"`
	Profit          *decimal.Decimal        `json:"profit,omitempty"`
	NetReturn       *float64                `json:"net_return,omitempty"`
	OutcomeTriggers []string                `json:"outcome_triggers,omitempty"`
}

func NewPosition(
	symbol string,
	path string,
	virtual bool,
	price *Price,
) *Position {
	return &Position{
		price:       price,
		ID:          uuid.NewString(),
		EntryOrders: make([]*spot.OrderRequest, 0),
		ExitOrders:  make([]*spot.OrderRequest, 0),
		Fills:       make([]*kraken.ExecutionData, 0),
		Symbol:      symbol,
		EntryPath:   path,
		Virtual:     virtual,
	}
}

/*
Update the position with a new mark. The mark drives the exit decision only
through ShouldExit; the position never closes itself.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Mark = mark

	/*
		Both fee legs ride the current tier as percent rates: the frontend's
		"if sold now" adds them to the mark-over-entry move for the round trip.
		Without a tier there is nothing to ride, and a leg that is erased is
		a leg the price layer can no longer read.
	*/
	if rate := position.feeRate(); rate != nil {
		position.EntryFee = rate
		position.ExitFee = rate
	}
}

/*
feeRate is the current fee tier as a percent — the fee row's units
(1.587 reads as 1.587%).
*/
func (position *Position) feeRate() *decimal.Decimal {
	if position.price == nil {
		return nil
	}

	fee := position.price.Fee(position.Symbol)

	if fee == nil || fee.Fee == nil {
		return nil
	}

	return fee.Fee
}

/*
bandPercent is how far the mark may travel from the entry price before the
walk is over: two percent either way.
*/
const bandPercent = 2

/*
ShouldExit reports whether a virtual position's band walk is over: the mark
has moved more than two percent either way from the entry price. The caller
(desk/training settle loop) closes it; the position only reports.

The band is the price against the entry price, not the fee-net return: the
fee legs carry tier rates (1.587 reads as 1.587%), so they cannot carry this
comparison, and the round trip already costs more than the band.
*/
func (position *Position) ShouldExit() bool {
	position.mu.Lock()
	defer position.mu.Unlock()

	if !position.Virtual || position.ClosedAt != nil {
		return false
	}

	if position.Mark == nil || position.EntryPrice == nil {
		return false
	}

	if position.EntryPrice.Sign() <= 0 {
		return false
	}

	move := (position.Mark.Float64()/position.EntryPrice.Float64() - 1) * 100

	return move < -bandPercent || move > bandPercent
}

/*
Enter records a buy of qty and returns the order to submit. A virtual
position never submits: its fill is emulated from the desk's price at entry,
so the path discovery ledger carries real entry costs and fees through the
same Position.Apply path venue fills use.
*/
func (position *Position) Enter() (*spot.OrderRequest, error) {
	position.mu.Lock()

	position.EntryOrders = append(position.EntryOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    position.Qty.String(),
	})

	position.ExitOrders = append(position.ExitOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    position.Qty.String(),
	})

	position.OpenedAt = time.Now().UTC()
	position.EntryTick = data.Tick.Load()

	virtual := position.Virtual
	order := position.EntryOrders[len(position.EntryOrders)-1]

	position.mu.Unlock()

	if virtual {
		cost, err := position.price.EntryCost(position.Symbol, position.Qty)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[position.Enter] no fill price for "+position.Symbol,
				err,
			))
		}

		position.Apply(kraken.ExecutionData{
			Symbol:      position.Symbol,
			Side:        "buy",
			ExecType:    "Fill",
			LastQty:     position.Qty,
			CumQty:      position.Qty,
			LastPrice:   cost.EntryPrice,
			AvgPrice:    cost.EntryPrice,
			FeeUsdEquiv: cost.EntryFee,
			Timestamp:   time.Now().UTC(),
		})
	}

	return order, nil
}

/*
Exit records a sell of everything open and returns the order to submit. A
position that is closed or already has its sell pending returns nil: selling
it again would sell quantity it no longer holds.
*/
func (position *Position) Exit(path string) *spot.OrderRequest {
	position.mu.Lock()
	defer position.mu.Unlock()

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitPath = path

	if !position.OpenedAt.IsZero() {
		position.HoldNs = int64(now.Sub(position.OpenedAt))
	}

	position.ExitTick = data.Tick.Load()

	return position.ExitOrders[len(position.ExitOrders)-1]
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

	position.Fills = append(position.Fills, &execution)

	fields := map[string]any{
		"side":      execution.Side,
		"exec_type": execution.ExecType,
	}

	if execution.LastQty != nil {
		fields["volume"] = execution.LastQty.String()
	}

	if execution.LastPrice != nil {
		fields["price"] = execution.LastPrice.String()
	}

	if execution.LastQty != nil && execution.LastPrice != nil {
		fields["cost"] = execution.LastQty.Mul(execution.LastPrice).String()
	}

	if execution.FeeUsdEquiv != nil {
		fields["fee"] = execution.FeeUsdEquiv.String()
	}

	if execution.AvgPrice != nil {
		fields["avg_price"] = execution.AvgPrice.String()
	}

	if !execution.Timestamp.IsZero() {
		fields["venue_at"] = execution.Timestamp.UTC().Format(time.RFC3339)
	}

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.Qty = execution.CumQty
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
			if position.Qty == nil {
				position.Qty = execution.LastQty
			}

			if position.Qty != nil && position.Qty != execution.LastQty {
				position.Qty = position.Qty.Add(execution.LastQty)
			}
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.EntryPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.EntryPrice = execution.LastPrice
		}

		/*
			Seed the mark and both fee legs so a fresh entry arrives on the
			wire with complete data. Update refines them from the fee tier
			as the book moves. The legs carry tier rates: the dollar fee of
			the fill stays in the fill itself, where the paper ledger and
			the frontend's execution totals read it.
		*/
		if position.Mark == nil {
			position.Mark = position.EntryPrice
		}

		if position.EntryFee == nil || position.EntryFee.Sign() == 0 {
			position.EntryFee = position.feeRate()
		}

		if position.ExitFee == nil || position.ExitFee.Sign() == 0 {
			position.ExitFee = position.feeRate()
		}

		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.ExitPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.ExitPrice = execution.LastPrice
		}

		/*
			The exit leg carries the tier rate like the entry leg: the dollar
			fee of the fill stays in the fill itself.
		*/
		if position.ExitFee == nil || position.ExitFee.Sign() == 0 {
			position.ExitFee = position.feeRate()
		}

		if position.Qty != nil {
			position.Qty = position.Qty.Sub(execution.LastQty)

			if position.Qty.Sign() <= 0 {
				position.Qty = decimal.NewFromFloat64(0)
			}
		}
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}
}
