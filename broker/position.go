package broker

import (
	"context"
	"math"
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
as source of truth so wire cumulatives never overwrite multi-print positions.
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
	Exit              *decimal.Decimal        `json:"exit,omitempty"`
	Sold              *decimal.Decimal        `json:"sold,omitempty"`
	Mark              *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP          *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom          *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL               *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct            *decimal.Decimal        `json:"pnl_pct,omitempty"`
	ExitWireStale     bool                    `json:"exit_wire_stale,omitempty"`
	EntryDesk         map[string]float64      `json:"entry_desk,omitempty"`
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
		System:    runtime.NewSystem(ctx, tierKey+":"+symbol),
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
Update stamps Mark from the bid hint for the chart, then exact book PnL/PnLPct.
WAITING opens (restored or just filled) arm to READY only once the book can
mark — restore must not READY/PnL before Level3 exists. Incomplete books leave
PnL nil and stay WAITING. Stoploss arms above risk.stoploss_arm_pnl_pct.
*/
func (position *Position) Update(hint *decimal.Decimal) (bool, error) {
	if hint == nil {
		return false, errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	stage := position.Status()

	if stage != runtime.READY && stage != runtime.WAITING {
		return false, nil
	}

	position.mu.Lock()

	stage = position.Status()

	if (stage != runtime.READY && stage != runtime.WAITING) ||
		position.Qty == nil || position.Qty.Sign() <= 0 || position.price == nil {
		position.mu.Unlock()
		return false, nil
	}

	stale := position.Mark != nil && hint.Equal(*position.Mark)

	position.Mark = hint
	position.PnL, position.PnLPct = position.price.markPnLLocked(position)

	if position.PnLPct == nil {
		position.mu.Unlock()
		return false, nil
	}

	if stage == runtime.WAITING {
		position.Transition(runtime.READY)
	}

	// Unchanged bid mark (ask-only L3, etc.): do not feed zero-delta ticks into
	// stoploss confirms / ExplosiveTTL — trail_confirm would trip on flat books.
	if stale && position.Stoploss != nil {
		position.mu.Unlock()
		return false, nil
	}

	qty := *position.Qty
	friction := position.price.ExhaustionReading(position.Symbol)

	if room := position.price.ExitRoom(position.Symbol, &qty); room != nil {
		if r, _ := room.Float64(); r > 0 {
			friction = math.Max(friction, 1/(1+r))
		}
	}

	pnlpct, _ := position.PnLPct.Float64()
	if position.Stoploss == nil {
		position.Stoploss = NewStoploss(pnlpct)
		if q, _ := position.Qty.Float64(); q > 0 {
			position.Stoploss.BasisQty = q
		}
	}

	position.Stoploss.Update(pnlpct, friction)

	if !position.Stoploss.WantsExit() {
		position.mu.Unlock()
		return !stale, nil
	}

	position.Transition(runtime.BUSY)
	position.mu.Unlock()

	if position.onSettled != nil {
		position.onSettled(position)
	}

	return true, nil
}

/*
Enter records the buy order. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate sizes once from wallet. Fills arrive only
through Apply.
*/
func (position *Position) Enter(
	path string, qty *decimal.Decimal, wallet *market.Balance,
) error {
	if path == "" {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		))
	}

	vol := qty.String()

	position.mu.Lock()
	defer position.mu.Unlock()

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
	}

	position.OpenedAt = time.Now().UTC()
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path
	position.Transition(runtime.WAITING)

	return nil
}

/*
exit arms a market sell and moves READY → BUSY. force stamps trigger "manual"
so real Send keeps the market flatten instead of the break-even IOC limit.
*/
func (position *Position) exit(force bool) error {
	trigger := "policy exit match"

	if force {
		trigger = "manual"
	} else if position.ExitPath != "" {
		trigger = "policy exit match " + position.ExitPath
	} else if position.Stoploss != nil && position.Stoploss.WantsExit() &&
		position.Stoploss.TriggerReason != "" {
		trigger = position.Stoploss.TriggerReason
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.Qty == nil || position.Qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.exit] no fill-derived qty for "+position.Symbol,
			nil,
		))
	}

	wire := position.exitWireQty(force)
	vol := wire.String()

	if position.price != nil && position.price.Instrument != nil &&
		position.price.normalizer != nil {
		pair := position.price.Instrument.Pair(
			position.price.normalizer.Name(position.Symbol),
		)

		if pair.QtyPrecision > 0 {
			vol = wire.StringFixed(int32(pair.QtyPrecision))
		}
	}

	if position.ExitOrder == nil {
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   uuid.NewString(),
			OrderType: "market",
			Type:      "sell",
			Volume:    vol,
		}
	} else {
		position.ExitOrder.ClOrdId = uuid.NewString()
		position.ExitOrder.Volume = vol
	}

	position.ExitTrigger = trigger
	position.Transition(runtime.BUSY)

	return nil
}

/*
exitWireQty sizes a ladder exit. Mid stages take PendingExitPct of BasisQty.
If that equal slice would leave a remainder below QtyMin/CostMin, the dust is
kept for the final stage: this mid sell shrinks so the stub stays venue-legal.
If the mid slice itself is below min (or a legal mid+final split is impossible),
this exit becomes the final stage and sells the full open.
*/
func (position *Position) exitWireQty(force bool) decimal.Decimal {
	qty := *position.Qty

	if force || position.Stoploss == nil || position.Stoploss.Triggered() {
		return qty
	}

	frac := position.Stoploss.PendingExitPct
	if frac <= stepEps || frac >= 1-stepEps {
		return qty
	}

	basis := decimal.NewFromFloat(position.Stoploss.BasisQty)
	if basis.Sign() <= 0 {
		basis = qty
	}

	want := basis.Mul(decimal.NewFromFloat(frac))
	if want.Cmp(qty) > 0 {
		want = qty
	}

	if position.belowVenueMin(want) {
		return qty
	}

	rem := qty.Sub(want)
	if rem.Sign() <= 0 || !position.belowVenueMin(rem) {
		return want
	}

	// Equal slice would leave dust — park at least venue-min for the final stage.
	leave := position.venueMinSize()
	if leave == nil {
		return qty
	}

	mid := qty.Sub(*leave)
	if mid.Sign() > 0 && !position.belowVenueMin(mid) {
		return mid
	}

	return qty
}

/*
belowVenueMin is true when size cannot clear the pair's QtyMin/CostMin.
Missing instrument metadata returns false (cannot judge).
*/
func (position *Position) belowVenueMin(size decimal.Decimal) bool {
	if size.Sign() <= 0 {
		return true
	}

	min := position.venueMinSize()
	return min != nil && size.Cmp(*min) < 0
}

/*
venueMinSize is the smallest venue-legal base size at the current mark
(QtyMin, and enough base to clear CostMin).
*/
func (position *Position) venueMinSize() *decimal.Decimal {
	if position.price == nil || position.price.Instrument == nil ||
		position.price.normalizer == nil {
		return nil
	}

	symbol := position.price.normalizer.Name(position.Symbol)
	pair := position.price.Instrument.Pair(symbol)

	var min decimal.Decimal
	have := false

	if pair.QtyMin != nil && pair.QtyMin.Sign() > 0 {
		min = *pair.QtyMin
		have = true
	}

	if pair.CostMin != nil && pair.CostMin.Sign() > 0 {
		if mark := position.price.Mark(symbol); mark != nil && mark.Sign() > 0 {
			byCost := pair.CostMin.Div(*mark)
			if !have || byCost.Cmp(min) > 0 {
				min = byCost
				have = true
			}
		}
	}

	if !have {
		return nil
	}

	return &min
}

/*
Validate checks the fields a live position needs to mark and exit.
EntryPath is strategy attribution — trade recover often has none; do not require it.
*/
func (position *Position) Validate() error {
	if position == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.Validate] position is nil",
			nil,
		))
	}

	position.mu.Lock()
	defer position.mu.Unlock()
	return position.validateLocked()
}

/*
Apply is the only fill path for one cycle. Entry fills refresh Qty/Entry and
leave the row WAITING — Update arms READY once price/books can mark. Exit fills
stamp ClosedAt once buys and sells net to zero. Ledger Fold restores the same
way: closed cycles end DONE without ever READY; opens wait for the book.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	if position == nil ||
		(execution.ExecType != "" && execution.ExecType != "trade") ||
		execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	for _, fill := range position.Fills {
		if fill == nil {
			continue
		}

		if fill.ExecID != "" && fill.ExecID == execution.ExecID {
			return
		}

		if execution.ExecID == "" &&
			fill.OrderID == execution.OrderID &&
			fill.Side == execution.Side &&
			fill.Timestamp.Equal(execution.Timestamp) &&
			fill.LastQty != nil && fill.LastQty.Equal(*execution.LastQty) {
			return
		}
	}

	closed := position.ClosedAt != nil || position.Status() == runtime.DONE

	if closed && execution.Side == "buy" {
		if position.ClosedAt != nil &&
			!execution.Timestamp.After(*position.ClosedAt) {
			return
		}

		position.Fills = make([]*market.ExecutionData, 0)
		position.ClosedAt = nil

		if position.Status() == runtime.DONE {
			position.Transition(runtime.WAITING)
		}

		closed = false
	}

	position.Fills = append(position.Fills, &execution)
	net, bought, notional := position.fillsNet()
	position.Qty = &net

	if bought.Sign() > 0 {
		vwap := notional.Div(bought)
		position.Entry = &vwap
	}

	if execution.Side == "sell" {
		if position.ClosedAt == nil && position.Qty.Sign() <= 0 {
			position.ClosedAt = &execution.Timestamp
			position.Transition(runtime.DONE)
			// Mark PnL needs open Qty; once flat, stamp settle PnL from fills
			// so closed/ S3 rows (and the Forward list) never ship blank.
			position.stampFillRealizedLocked()
		}

		return
	}

	if position.Qty != nil && position.Qty.Sign() > 0 {
		switch position.Status() {
		case runtime.READY, runtime.WAITING:
		default:
			position.Transition(runtime.WAITING)
		}
	}
}

func (position *Position) fillsNet() (net, bought, notional decimal.Decimal) {
	for _, fill := range position.Fills {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		switch fill.Side {
		case "buy":
			px := decimal.Zero

			if fill.LastPrice != nil {
				px = *fill.LastPrice
			}

			bought = bought.Add(*fill.LastQty)
			notional = notional.Add(px.Mul(*fill.LastQty))
			net = net.Add(*fill.LastQty)
		case "sell":
			net = net.Sub(*fill.LastQty)
		}
	}

	return net, bought, notional
}

/*
stampFillRealizedLocked writes fee-agnostic settle PnL from fill notionals.
Caller holds position.mu. Used when the book can no longer mark a flat Qty.
*/
func (position *Position) stampFillRealizedLocked() {
	var buyQty, buyNotional, sellQty, sellNotional decimal.Decimal

	for _, fill := range position.Fills {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		px := decimal.Zero

		if fill.LastPrice != nil {
			px = *fill.LastPrice
		}

		switch fill.Side {
		case "buy":
			buyQty = buyQty.Add(*fill.LastQty)
			buyNotional = buyNotional.Add(px.Mul(*fill.LastQty))
		case "sell":
			sellQty = sellQty.Add(*fill.LastQty)
			sellNotional = sellNotional.Add(px.Mul(*fill.LastQty))
		}
	}

	if buyQty.Sign() <= 0 || buyNotional.Sign() <= 0 || sellQty.Sign() <= 0 {
		return
	}

	pnl := sellNotional.Sub(buyNotional)
	pct := pnl.Div(buyNotional).Mul(decimalHundred)
	position.PnL = &pnl
	position.PnLPct = &pct
	sold := sellQty
	position.Sold = &sold
	vwap := sellNotional.Div(sellQty)
	position.ExitVWAP = &vwap
}
