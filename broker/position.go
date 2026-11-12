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

/*
Position is the durable unit of risk: one symbol per tier cache key, with fills
as source of truth so wire cumulatives never overwrite multi-print lots.
*/
type Position struct {
	*runtime.System
	mu                sync.Mutex
	transport         network.Client
	price             *Price
	onSettled         func(*Position)
	Stoploss          *Stoploss               `json:"stoploss,omitempty"`
	ID                string                  `json:"id"`
	TierKey           string                  `json:"tier_key,omitempty"`
	EntryPath         string                  `json:"entry_path"`
	HoldPaths         []string                `json:"hold_paths,omitempty"`
	ExitPath          string                  `json:"exit_path"`
	ExitTrigger       string                  `json:"exit_trigger,omitempty"`
	OutcomeTriggers   []string                `json:"outcome_triggers,omitempty"`
	Symbol            string                  `json:"symbol"`
	OpenedAt          time.Time               `json:"opened_at"`
	ClosedAt          *time.Time              `json:"closed_at"`
	UpdatedAt         *time.Time              `json:"updated_at"`
	PolicyEpoch       uint64                  `json:"policy_epoch,omitempty"`
	EntryTick         uint64                  `json:"entry_tick,omitempty"`
	ExitTick          uint64                  `json:"exit_tick,omitempty"`
	EntryOrder        *spot.OrderRequest      `json:"entry_order,omitempty"`
	ExitOrder         *spot.OrderRequest      `json:"exit_order,omitempty"`
	EntryVenueOrderID string                  `json:"entry_venue_order_id,omitempty"`
	ExitVenueOrderID  string                  `json:"exit_venue_order_id,omitempty"`
	Fills             []*market.ExecutionData `json:"fills,omitempty"`
	Qty               *decimal.Decimal        `json:"qty,omitempty"`
	Entry             *decimal.Decimal        `json:"entry,omitempty"`
	Sold              *decimal.Decimal        `json:"sold,omitempty"`
	Mark              *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP          *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom          *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL               *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct            *decimal.Decimal        `json:"pnl_pct,omitempty"`
	Closing           bool                    `json:"closing,omitempty"`
	ExitWireStale     bool                    `json:"exit_wire_stale,omitempty"`
	ExitAttempts      int                     `json:"exit_attempts,omitempty"`
	TreeApplied       bool                    `json:"-"`
}

/*
NewPosition only assigns identity and collaborators — Qty and stoploss appear
after Apply so restored JSON cannot skip the fill fold path.
*/
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
		transport: transport,
		onSettled: onSettled,
		ID:        uuid.NewString(),
		TierKey:   tierKey,
		Fills:     make([]*market.ExecutionData, 0),
		Symbol:    symbol,
		EntryPath: path,
	}
}

/*
Update marks an open lot, feeds the stoploss, and on trigger arms Closing.
ClosedAt waits for exit fills in Apply. Calling Update on a lot that is not
a live open fill is a bug — it errors, it does not soft-skip.
*/
func (position *Position) Update(price *decimal.Decimal) error {
	position.mu.Lock()

	if price == nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	if position.ClosedAt != nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] closed lot",
			nil,
		), "symbol", position.Symbol)
	}

	if position.Qty == nil || position.Qty.Sign() <= 0 ||
		position.Entry == nil || position.Entry.Sign() <= 0 {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.Update] lot not filled",
			nil,
		), "symbol", position.Symbol)
	}

	// Snapshot under the position lock, then walk the book unlocked so marks
	// do not hold Position.mu across BookManager.RLock.
	symbol := position.Symbol
	qty := *position.Qty
	entry := *position.Entry
	pricer := position.price
	position.mu.Unlock()

	var vwap, room, bookPnL, bookPnLPct *decimal.Decimal

	if pricer != nil {
		vwap, room, bookPnL, bookPnLPct = pricer.MarkClose(symbol, &qty, &entry)
	}

	position.mu.Lock()

	if position.ClosedAt != nil || position.Qty == nil || position.Entry == nil {
		position.mu.Unlock()
		return nil
	}

	if position.Qty.Cmp(qty) != 0 || position.Entry.Cmp(entry) != 0 {
		qty = *position.Qty
		entry = *position.Entry

		if pricer != nil {
			vwap, room, bookPnL, bookPnLPct = pricer.MarkClose(symbol, &qty, &entry)
		}
	}

	if vwap == nil || bookPnL == nil || bookPnLPct == nil {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[position.Update] book mark failed for "+position.Symbol,
			nil,
		))
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now
	position.Mark = price

	hasSellFills := position.Sold != nil && position.Sold.Sign() > 0

	if hasSellFills {
		position.ExitRoom = room
	} else {
		position.ExitVWAP = vwap
		position.ExitRoom = room
		position.PnL = bookPnL
		position.PnLPct = bookPnLPct
	}

	if position.Closing {
		position.mu.Unlock()
		return nil
	}

	if position.EntryPath == "exchange" {
		position.mu.Unlock()
		return nil
	}

	pnlPct, _ := position.PnLPct.Float64()

	if position.Stoploss == nil {
		position.Stoploss = NewStoploss(pnlPct)
	}

	position.Stoploss.Update(pnlPct)
	position.dischargeStoploss()

	return nil
}

/*
dischargeStoploss arms Closing and calls onSettled when the stop fires.
Caller holds position.mu.
*/
func (position *Position) dischargeStoploss() {
	if position.Closing || position.ClosedAt != nil {
		position.mu.Unlock()
		return
	}

	if position.Stoploss == nil || !position.Stoploss.Triggered() {
		position.mu.Unlock()
		return
	}

	position.armClosing("stoploss")
	onSettled := position.onSettled
	position.mu.Unlock()

	if onSettled != nil {
		onSettled(position)
	}
}

/*
Enter records the buy order. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate sizes once from wallet. Fills arrive only
through Apply.
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
			if err != nil {
				return errnie.Error(err)
			}

			return errnie.Err(
				errnie.UnprocessableContent,
				"[position.Enter] cannot size "+position.Symbol,
				nil,
			)
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

	return nil
}

/*
Owns reports whether this execution belongs to the lot's entry or exit order.
*/
func (position *Position) Owns(execution market.ExecutionData) bool {
	if execution.ClientOrderID != "" {
		if position.EntryOrder != nil &&
			position.EntryOrder.ClOrdId == execution.ClientOrderID {
			return true
		}

		if position.ExitOrder != nil &&
			position.ExitOrder.ClOrdId == execution.ClientOrderID {
			return true
		}
	}

	if execution.OrderID != "" {
		if position.EntryVenueOrderID == execution.OrderID {
			return true
		}

		if position.ExitVenueOrderID == execution.OrderID {
			return true
		}
	}

	if execution.OrderID == "" {
		return false
	}

	for _, fill := range position.Fills {
		if fill != nil && fill.OrderID == execution.OrderID {
			return true
		}
	}

	return false
}

/*
Flat is the Apply gate for ClosedAt — until qty nets to zero we must not stamp
settle or run Remove even when Closing is already armed.
*/
func (position *Position) Flat() bool {
	return position.Sold != nil && position.Sold.Sign() > 0 &&
		(position.Qty == nil || position.Qty.Sign() <= 0)
}

/*
Exit arms Closing for a policy exit. ClosedAt waits for exit fills in Apply.
*/
func (position *Position) Exit() error {
	position.mu.Lock()

	if position.ClosedAt != nil || position.Closing {
		position.mu.Unlock()
		return nil
	}

	trigger := "policy exit match " + position.ExitPath

	if position.ExitTrigger == "" {
		position.armClosing(trigger)
	} else {
		position.armClosing(position.ExitTrigger)
	}

	position.mu.Unlock()
	return nil
}

/*
fillPrice prefers LastPrice but accepts AvgPrice because Kraken status frames
sometimes omit last on partials we still need to fold.
*/
func fillPrice(execution *market.ExecutionData) *decimal.Decimal {
	if execution == nil {
		return nil
	}

	px := execution.LastPrice

	if px == nil || px.Sign() <= 0 {
		px = execution.AvgPrice
	}

	if px == nil || px.Sign() <= 0 {
		return nil
	}

	return px
}

/*
fillHasPrice guards Apply — zero-price rows are status noise and must not move
entry VWAP or arm mistaken stoploss moves.
*/
func fillHasPrice(execution market.ExecutionData) bool {
	return fillPrice(&execution) != nil
}

/*
fillWireDuplicate prevents double-folding the same print when Kraken replays
executions without exec_id after a reconnect or burst of status frames.
*/
func fillWireDuplicate(stored *market.ExecutionData, incoming *market.ExecutionData) bool {
	if stored == nil || incoming == nil || stored.ExecID != "" || incoming.ExecID != "" {
		return false
	}

	if !strings.EqualFold(stored.Side, incoming.Side) {
		return false
	}

	if stored.OrderID != incoming.OrderID ||
		stored.ClientOrderID != incoming.ClientOrderID {
		return false
	}

	if stored.LastQty == nil || incoming.LastQty == nil ||
		stored.LastQty.Cmp(*incoming.LastQty) != 0 {
		return false
	}

	spx := fillPrice(stored)
	ipx := fillPrice(incoming)

	if spx == nil || ipx == nil {
		return spx == ipx
	}

	return spx.Cmp(*ipx) == 0
}

/*
sanitizeFillForReplay prevents Restore from treating stored CumQty as lot qty —
each replayed row must look like a single print again.
*/
func sanitizeFillForReplay(fill *market.ExecutionData) {
	if fill == nil {
		return
	}

	fill.CumQty = nil
	fill.CumCost = nil
	fill.AvgPrice = nil
}

/*
Apply is the only fill path. Fills are the ledger; Qty/Entry/Sold are folded
from them. ClosedAt stamps once the lot is flat.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	position.mu.Lock()

	if position.ClosedAt != nil {
		position.mu.Unlock()
		return
	}

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		position.mu.Unlock()
		return
	}

	if !fillHasPrice(execution) {
		position.mu.Unlock()
		return
	}

	for _, fill := range position.Fills {
		if fill == nil {
			continue
		}

		if execution.ExecID != "" {
			if fill.ExecID == execution.ExecID {
				position.mu.Unlock()
				return
			}

			continue
		}

		if fillWireDuplicate(fill, &execution) {
			position.mu.Unlock()
			return
		}
	}

	// Nothing left to sell — ignore late/duplicate exit prints.
	if strings.EqualFold(execution.Side, "sell") &&
		(position.Qty == nil || position.Qty.Sign() <= 0) {
		position.mu.Unlock()
		return
	}

	copied := execution
	position.Fills = append(position.Fills, &copied)
	position.foldFills()

	if position.Closing &&
		strings.EqualFold(copied.Side, "sell") &&
		position.Qty != nil && position.Qty.Sign() > 0 {
		position.ExitWireStale = true
	}

	if position.Closing && position.Qty != nil && position.Qty.Sign() > 0 {
		position.ensureExitOrder()
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now

	closed := false

	if position.Flat() && position.ClosedAt == nil {
		if position.price != nil && position.price.Instrument != nil {
			position.PnL, position.PnLPct = position.price.FeeNetRoundTrip(
				position.Symbol, position.Entry, position.Sold, position.ExitVWAP,
			)
		}

		if position.PnL == nil || position.PnLPct == nil {
			position.settleGrossLocked()
		}

		if (position.PnL == nil || position.PnLPct == nil) &&
			position.EntryPath != "exchange" {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[position.Apply] settle failed for "+position.Symbol,
				nil,
			))
		}

		position.Closing = true
		position.ClosedAt = &now

		if position.ExitTick == 0 {
			position.ExitTick = data.Tick.Load()
		}

		closed = true
	}

	onSettled := position.onSettled
	position.mu.Unlock()

	if closed && onSettled != nil {
		onSettled(position)
	}
}

/*
settleGrossLocked is the fallback when FeeNetRoundTrip cannot run (missing fee
table) so closed lots still get a PnL sign for Unlocked stats.
*/
func (position *Position) settleGrossLocked() {
	if position.Entry == nil || position.Sold == nil || position.ExitVWAP == nil ||
		position.Sold.Sign() <= 0 || position.ExitVWAP.Sign() <= 0 ||
		position.Entry.Sign() <= 0 {
		return
	}

	grossEntry := position.Entry.Mul(*position.Sold)
	grossExit := position.ExitVWAP.Mul(*position.Sold)
	pnl := grossExit.Sub(grossEntry)

	if grossEntry.Sign() <= 0 {
		return
	}

	hundred := decimal.NewFromInt(100)
	pct := pnl.Div(grossEntry).Mul(hundred)
	position.PnL = &pnl
	position.PnLPct = &pct
}

/*
foldFills rebuilds Qty/Entry/Sold/ExitVWAP/Mark from Fills.
Each fill contributes LastQty; entry is the volume-weighted buy average.
CumQty/AvgPrice on a wire row describe that order slice, not the open lot.
*/
func (position *Position) foldFills() {
	var qty, entry, sold, exitVWAP, mark *decimal.Decimal
	var soldCost decimal.Decimal

	for _, fill := range position.Fills {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		px := fill.LastPrice

		if px == nil || px.Sign() <= 0 {
			px = fill.AvgPrice
		}

		if px == nil || px.Sign() <= 0 {
			continue
		}

		mark = px
		last := fill.LastQty

		switch strings.ToLower(fill.Side) {
		case "buy":
			if qty == nil {
				q := *last
				qty = &q
			} else {
				q := qty.Add(*last)
				qty = &q
			}

			if entry == nil {
				entry = px
			} else {
				prev := qty.Sub(*last)
				e := entry.Mul(prev).Add(px.Mul(*last)).Div(*qty)
				entry = &e
			}

		case "sell":
			if qty == nil || qty.Sign() <= 0 {
				continue
			}

			take := decimal.Min(*last, *qty)
			open := qty.Sub(take)
			qty = &open

			soldCost = soldCost.Add(px.Mul(take))

			if sold == nil {
				sold = &take
			} else {
				s := sold.Add(take)
				sold = &s
			}

			vwap := soldCost.Div(*sold)
			exitVWAP = &vwap
		}
	}

	position.Qty = qty
	position.Entry = entry
	position.Sold = sold
	position.ExitVWAP = exitVWAP
	position.Mark = mark
}

/*
armClosing centralizes stoploss and policy exits so kickExit always sees ExitOrder
volume matching post-partial Qty before Send.
*/
func (position *Position) armClosing(trigger string) {
	position.Closing = true
	position.ExitTrigger = trigger
	position.OutcomeTriggers = []string{trigger}
	position.ExitTick = data.Tick.Load()
	position.ensureExitOrder()
}

/*
ensureExitOrder sets the sell to the filled open Qty. Caller holds mu.
A blank ClOrdId (after a venue reject) remints so the next Send is not a
duplicate client id.
*/
func (position *Position) ensureExitOrder() {
	if position.Qty == nil || position.Qty.Sign() <= 0 {
		return
	}

	vol := position.Qty.String()

	if position.ExitOrder == nil {
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   uuid.NewString(),
			OrderType: "market",
			Type:      "sell",
			Volume:    vol,
		}
		return
	}

	if position.ExitOrder.ClOrdId == "" {
		position.ExitOrder.ClOrdId = uuid.NewString()
	}

	position.ExitOrder.OrderType = "market"
	position.ExitOrder.Type = "sell"
	position.ExitOrder.Volume = vol
}
