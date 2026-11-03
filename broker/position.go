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
)

type Position struct {
	*runtime.System
	mu              sync.Mutex
	transport       network.Client
	price           *Price
	tierKey         string
	onSettled       func(*Position)
	qty             *decimal.Decimal // folded in Apply; Update only reads
	entry           *decimal.Decimal // folded in Apply; Update only reads
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
	Fills           []*market.ExecutionData `json:"fills,omitempty"`
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
		Fills:     make([]*market.ExecutionData, 0),
		Symbol:    symbol,
		EntryPath: path,
	}
}

/*
Update is the mark path: set mark, reprice from folded qty/entry, eval stoploss.
Fill accounting belongs in Apply.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	position.mu.Lock()
	defer position.mu.Unlock()

	settled := position.ClosedAt != nil

	// Live ticks (nil mark) must not rewrite settled lots. Exit/Apply pass a
	// mark so the one-time final PnL pass can still run after ClosedAt.
	if settled && mark == nil {
		return
	}

	if mark != nil {
		position.Mark = mark
	}

	// Persisted lots carry Fills but not the unexported fold.
	if (position.qty == nil || position.entry == nil) && len(position.Fills) > 0 {
		position.foldFills()
	}

	// Order-only S3 skeletons (saved before fills landed) need a book fill
	// once before mark/PnL can run.
	if (position.qty == nil || position.entry == nil) &&
		len(position.Fills) == 0 &&
		position.ClosedAt == nil &&
		position.EntryOrder != nil &&
		position.EntryOrder.Volume != "" {
		if parsed, err := decimal.NewFromString(position.EntryOrder.Volume); err == nil &&
			parsed.Sign() > 0 {
			at := position.OpenedAt
			if at.IsZero() {
				at = time.Now().UTC()
			}
			position.seedEntry(&parsed, at)
		}
	}

	qty, entry := position.qty, position.entry

	if qty == nil || qty.Sign() <= 0 || entry == nil || entry.Sign() <= 0 {
		return
	}

	if position.price == nil {
		return
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now

	fee := position.price.Fee(position.Symbol)
	if fee == nil {
		zero := decimal.Zero
		fee = &zero
	}

	position.ExitVWAP = position.price.ExitVWAP(position.Symbol, qty)
	position.ExitRoom = position.price.ExitRoom(position.Symbol, qty)

	if position.Mark == nil && position.ExitVWAP != nil {
		position.Mark = position.ExitVWAP
	}

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
		position.markPnL(entry, qty)
	}

	if position.PnLPct != nil && position.Stoploss == nil && position.ClosedAt == nil {
		pnlPct, _ := position.PnLPct.Float64()
		position.Stoploss = NewStoploss(pnlPct, position.price, position.Symbol)
	}

	if position.ClosedAt != nil || position.Stoploss == nil || position.PnLPct == nil {
		return
	}

	position.Stoploss.rewire()
	position.Stoploss.Observe(position.price, position.Symbol)

	pnlPct, _ := position.PnLPct.Float64()
	position.Stoploss.Update(pnlPct)

	if !position.Stoploss.Triggered() {
		return
	}

	position.ExitTrigger = "stoploss"
	position.OutcomeTriggers = []string{"stoploss"}

	closed := time.Now().UTC()
	position.ClosedAt = &closed
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
func (position *Position) Owns(execution market.ExecutionData) bool {
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

// markPnL sets fee-net close-now PnL from mark/exit VWAP when the bid walk fails.
func (position *Position) markPnL(entry, qty *decimal.Decimal) {
	if entry == nil || entry.Sign() <= 0 || position.price == nil {
		return
	}

	if qty == nil || qty.Sign() <= 0 {
		return
	}

	fee := position.price.Fee(position.Symbol)
	if fee == nil {
		zero := decimal.Zero
		fee = &zero
	}

	if pnl, _ := position.price.roundTrip(position.Symbol, qty, entry, fee, fee); pnl != nil {
		position.PnL = pnl
	}

	if pct := position.price.roundTripPct(position.Symbol, qty, entry, fee, fee); pct != nil {
		position.PnLPct = pct
		return
	}

	exit := position.ExitVWAP
	if exit == nil || exit.Sign() <= 0 {
		exit = position.Mark
	}

	if exit == nil || exit.Sign() <= 0 {
		return
	}

	entryCost := entry.Mul(*qty)
	netEntry := position.price.afterFee(&entryCost, fee, BUY)
	exitGross := exit.Mul(*qty)
	netExit := position.price.afterFee(&exitGross, fee, SELL)

	if netEntry == nil || netExit == nil || netEntry.Sign() <= 0 {
		return
	}

	pnl := netExit.Sub(*netEntry)
	position.PnL = &pnl
	pct := pnl.Div(*netEntry).Mul(decimalHundred)
	position.PnLPct = &pct
}

// seedEntry records a book-derived buy fill for no-transport test lots.
func (position *Position) seedEntry(qty *decimal.Decimal, at time.Time) {
	if position.price == nil || qty == nil || qty.Sign() <= 0 {
		return
	}

	entry := position.price.FillAsk(position.Symbol, qty)

	if entry == nil || entry.Sign() <= 0 {
		return
	}

	position.applyFill(market.ExecutionData{
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
foldFills rebuilds qty/entry from Fills. Call only when fills change.
*/
func (position *Position) foldFills() {
	var qty *decimal.Decimal
	var entry *decimal.Decimal

	for _, fill := range position.Fills {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		side := strings.ToLower(fill.Side)

		if side == "buy" {
			if fill.CumQty != nil && fill.CumQty.Sign() > 0 {
				qty = fill.CumQty
			} else if qty == nil {
				qty = fill.LastQty
			} else {
				sum := qty.Add(*fill.LastQty)
				qty = &sum
			}

			if fill.AvgPrice != nil && fill.AvgPrice.Sign() > 0 {
				entry = fill.AvgPrice
			} else if fill.LastPrice != nil && fill.LastPrice.Sign() > 0 {
				entry = fill.LastPrice
			}

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

	position.qty = qty
	position.entry = entry
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

	mark := position.Mark
	position.mu.Unlock()

	// Final PnL while still open — Update ignores nil-mark ticks after close.
	position.Update(mark)

	position.mu.Lock()

	if position.ClosedAt == nil {
		now := time.Now().UTC()
		position.ClosedAt = &now
		position.ExitTick = data.Tick.Load()
	}

	position.mu.Unlock()
	return nil
}

/*
Apply records a venue fill, folds qty/entry, then marks through Update.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	position.mu.Lock()
	mark := position.applyFill(execution)
	position.mu.Unlock()

	if mark == nil {
		return
	}

	position.Update(mark)
}

/*
applyFill appends a fill and folds size under an already-held lock.
Returns the mark to feed Update, or nil when the fill carries no size.
*/
func (position *Position) applyFill(execution market.ExecutionData) *decimal.Decimal {
	position.Fills = append(position.Fills, &execution)
	position.foldFills()

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return nil
	}

	mark := position.Mark

	if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
		mark = execution.LastPrice
	} else if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		mark = execution.AvgPrice
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}

	return mark
}
