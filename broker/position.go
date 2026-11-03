package broker

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/venue/kraken"
)

type Position struct {
	*runtime.System
	mu              sync.Mutex
	transport       network.Client
	price           *Price
	tierKey         string
	onSettled       func(*Position)
	Stoploss        *Stoploss               `json:"stoploss,omitempty"`
	ID              string                  `json:"id"`
	Book            string                  `json:"book,omitempty"`
	EntryPath       string                  `json:"entry_path"`
	HoldPaths       []string                `json:"hold_paths,omitempty"`
	ExitPath        string                  `json:"exit_path"`
	ExitTrigger     string                  `json:"exit_trigger,omitempty"`
	OutcomeTriggers []string                `json:"outcome_triggers,omitempty"`
	Symbol          string                  `json:"symbol"`
	OpenedAt        time.Time               `json:"opened_at"`
	ClosedAt        *time.Time              `json:"closed_at"`
	UpdatedAt       *time.Time              `json:"updated_at"`
	PolicyEpoch     uint64                  `json:"policy_epoch,omitempty"`
	EntryTick       uint64                  `json:"entry_tick,omitempty"`
	ExitTick        uint64                  `json:"exit_tick,omitempty"`
	EntryOrder      *spot.OrderRequest      `json:"entry_order,omitempty"`
	ExitOrder       *spot.OrderRequest      `json:"exit_order,omitempty"`
	Fills           []*kraken.ExecutionData `json:"fills,omitempty"`
	Mark            *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP        *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom        *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL             *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct          *decimal.Decimal        `json:"pnl_pct,omitempty"`
}

func NewPosition(
	ctx context.Context,
	symbol string,
	tierKey string,
	path string,
	price *Price,
	transport network.Client,
	onSettled func(*Position),
) *Position {
	return &Position{
		System:    runtime.NewSystem(ctx, "position"),
		price:     price,
		tierKey:   tierKey,
		transport: transport,
		onSettled: onSettled,
		ID:        uuid.NewString(),
		Book:      tierKey,
		Fills:     make([]*kraken.ExecutionData, 0),
		Symbol:    symbol,
		EntryPath: path,
	}
}

/*
Update refreshes the mark and reprices open PnL. Stoploss owns risk state;
Position only feeds marks and exits when Triggered.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	position.mu.Lock()
	defer position.mu.Unlock()

	// Settled lots keep their exit mark — live ticks must not rewrite them.
	if position.ClosedAt != nil {
		return
	}

	if mark != nil {
		position.Mark = mark
	}

	if position.price != nil {
		now := time.Now().UTC()
		position.UpdatedAt = &now

		// Virtual lots have no venue fills — seed a book-derived entry for mark.
		// Paper/real wait for the venue execution frame.
		if position.transport == nil && len(position.Fills) == 0 &&
			position.EntryOrder != nil && position.EntryOrder.Volume != "" {
			if parsed, err := decimal.NewFromString(position.EntryOrder.Volume); err == nil &&
				parsed.Sign() > 0 {
				position.seedEntry(&parsed, position.OpenedAt)
			}
		}

		var qty *decimal.Decimal

		for _, fill := range position.Fills {
			if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
				continue
			}

			side := strings.ToLower(fill.Side)

			if side == "buy" {
				if fill.CumQty != nil && fill.CumQty.Sign() > 0 {
					qty = fill.CumQty
					continue
				}

				if qty == nil {
					qty = fill.LastQty
					continue
				}

				sum := qty.Add(*fill.LastQty)
				qty = &sum
				continue
			}

			if side == "sell" && qty != nil {
				diff := qty.Sub(*fill.LastQty)
				qty = &diff

				if qty.Sign() <= 0 {
					zero := decimal.NewFromInt(0)
					qty = &zero
				}
			}
		}

		if qty == nil && position.EntryOrder != nil && position.EntryOrder.Volume != "" {
			if parsed, err := decimal.NewFromString(position.EntryOrder.Volume); err == nil && parsed.Sign() > 0 {
				qty = &parsed
			}
		}

		var entry *decimal.Decimal

		for _, fill := range position.Fills {
			if fill == nil || strings.ToLower(fill.Side) != "buy" {
				continue
			}

			if fill.AvgPrice != nil && fill.AvgPrice.Sign() > 0 {
				entry = fill.AvgPrice
				continue
			}

			if fill.LastPrice != nil && fill.LastPrice.Sign() > 0 {
				entry = fill.LastPrice
			}
		}

		fee := position.price.Fee(position.Symbol)
		if fee == nil {
			zero := decimal.Zero
			fee = &zero
		}

		position.ExitVWAP = position.price.ExitVWAP(position.Symbol, qty)
		position.ExitRoom = position.price.ExitRoom(position.Symbol, qty)

		newPnL, _ := position.price.roundTrip(
			position.Symbol, qty, entry, fee, fee,
		)

		newPnLPct := position.price.roundTripPct(
			position.Symbol, qty, entry, fee, fee,
		)

		if newPnL != nil {
			position.PnL = newPnL
			position.PnLPct = newPnLPct
		} else {
			// Bid walk can fail on thin books; chart still needs a Y from mark.
			position.markPnL(entry, qty)
		}

		if position.PnLPct != nil && position.Stoploss == nil && position.ClosedAt == nil {
			pnlPct, _ := position.PnLPct.Float64()
			position.Stoploss = NewStoploss(pnlPct)
		}
	}

	if position.ClosedAt != nil || position.Stoploss == nil || position.PnLPct == nil {
		return
	}

	position.Stoploss.rewire()

	pnlPct, _ := position.PnLPct.Float64()
	position.Stoploss.Update(pnlPct)

	if !position.Stoploss.Triggered() {
		return
	}

	position.ExitTrigger = "stoploss"
	position.OutcomeTriggers = []string{"stoploss"}

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitTick = data.Tick.Load()
	onSettled := position.onSettled
	position.mu.Unlock()

	if onSettled != nil {
		onSettled(position)
	}

	position.mu.Lock()
}

/*
Enter records the buy. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate sizes once from wallet. A nil wallet
uses the price desk balance.
*/
func (position *Position) Enter(path string, qty *decimal.Decimal, wallet *market.Balance) error {
	position.mu.Lock()
	defer position.mu.Unlock()

	if path == "" {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		)
	}

	if qty == nil || qty.Sign() <= 0 {
		var err error
		qty, err = position.price.Allocate(position.Symbol, wallet)

		if err != nil || qty == nil || qty.Sign() <= 0 {
			reason := "cannot size " + position.Symbol
			if err != nil {
				reason = err.Error()
			}

			// Thin books / venue floors that do not fit budget or exit depth
			// are routine skips — EntryDenial so the desk does not ERROR-log.
			return &EntryDenial{Gate: "size", Reason: reason}
		}
	}

	vol := qty.String()

	position.EntryOrder = &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    vol,
	}

	position.ExitOrder = &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    vol,
	}

	now := time.Now().UTC()
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path

	if position.transport == nil && len(position.Fills) == 0 {
		position.seedEntry(qty, now)
	}

	return nil
}

/*
Owns reports whether this execution belongs to the lot's entry or exit order.
*/
func (position *Position) Owns(execution kraken.ExecutionData) bool {
	if execution.ClientOrderID == "" {
		return false
	}

	if position.EntryOrder != nil &&
		position.EntryOrder.ClOrdId == execution.ClientOrderID {
		return true
	}

	if position.ExitOrder != nil &&
		position.ExitOrder.ClOrdId == execution.ClientOrderID {
		return true
	}

	return false
}

// markPnL sets close-now style PnL from mark/exit VWAP when the bid walk fails.
func (position *Position) markPnL(entry, qty *decimal.Decimal) {
	if entry == nil || entry.Sign() <= 0 {
		return
	}

	mark := position.Mark
	if mark == nil || mark.Sign() <= 0 {
		mark = position.ExitVWAP
	}

	if mark == nil || mark.Sign() <= 0 {
		return
	}

	diff := mark.Sub(*entry)
	pct := diff.Div(*entry).Mul(decimal.NewFromInt(100))
	position.PnLPct = &pct

	if qty != nil && qty.Sign() > 0 {
		pnl := diff.Mul(*qty)
		position.PnL = &pnl
	}
}

// seedEntry records a synthetic buy fill from EntryCost or the touch ask.
func (position *Position) seedEntry(qty *decimal.Decimal, at time.Time) {
	if position.price == nil || qty == nil || qty.Sign() <= 0 {
		return
	}

	var entry *decimal.Decimal

	if fill, err := position.price.EntryCost(position.Symbol, qty); err == nil &&
		fill != nil && fill.EntryPrice != nil && fill.EntryPrice.Sign() > 0 {
		entry = fill.EntryPrice
	}

	if entry == nil {
		entry = position.price.TouchAsk(position.Symbol)
	}

	if entry == nil || entry.Sign() <= 0 {
		return
	}

	position.Fills = append(position.Fills, &kraken.ExecutionData{
		Symbol:      position.Symbol,
		Side:        "buy",
		LastQty:     qty,
		CumQty:      qty,
		AvgPrice:    entry,
		LastPrice:   entry,
		ExecType:    "trade",
		OrderStatus: "filled",
		Timestamp:   at,
	})
}

/*
Exit records the sell and reprices from the bid book.
*/
func (position *Position) Exit() error {
	position.mu.Lock()

	if position.ExitTrigger == "" {
		position.ExitTrigger = "policy exit match " + position.ExitPath
		position.OutcomeTriggers = []string{position.ExitTrigger}
	}

	if position.ClosedAt != nil {
		position.mu.Unlock()
		return nil
	}

	now := time.Now().UTC()
	position.ClosedAt = &now

	position.ExitTick = data.Tick.Load()
	mark := position.Mark

	position.mu.Unlock()
	position.Update(mark)

	return nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()

	position.Fills = append(position.Fills, &execution)

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		position.mu.Unlock()
		return
	}

	mark := position.Mark

	if strings.ToLower(execution.Side) == "buy" && mark == nil {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			mark = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			mark = execution.LastPrice
		}
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}

	position.mu.Unlock()
	position.Update(mark)
}
