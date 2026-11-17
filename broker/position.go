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
	"github.com/theapemachine/symm/venue/kraken"
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
Stoploss arms above risk.stoploss_arm_pnl_pct. Incomplete books leave PnL nil
and skip the stop. Other stages are a silent no-op.
*/
func (position *Position) Update(hint *decimal.Decimal) (bool, error) {
	if hint == nil {
		return false, errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	if position.Status() != runtime.READY {
		return false, nil
	}

	position.mu.Lock()

	if position.Status() != runtime.READY ||
		position.Qty == nil || position.Qty.Sign() <= 0 || position.price == nil {
		position.mu.Unlock()
		return false, nil
	}

	stale := position.Mark != nil && hint.Equal(*position.Mark)

	position.Mark = hint
	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)

	if position.PnLPct == nil {
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
	}

	position.Stoploss.Update(pnlpct, friction)

	if !position.Stoploss.Triggered() {
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

	wire := *position.Qty

	var pair kraken.InstrumentPair

	if position.price != nil && position.price.Instrument != nil &&
		position.price.normalizer != nil {
		pair = position.price.Instrument.Pair(
			position.price.normalizer.Name(position.Symbol),
		)

		if pair.QtyIncrement != nil && pair.QtyIncrement.Sign() > 0 {
			inc := *pair.QtyIncrement
			wire = wire.Div(inc).Floor().Mul(inc)
		}
	}

	if wire.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[position.exit] wire qty rounds to zero",
			nil,
		))
	}

	vol := wire.String()

	if pair.QtyPrecision > 0 {
		vol = wire.StringFixed(int32(pair.QtyPrecision))
	}

	if position.ExitOrder == nil {
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   uuid.NewString(),
			OrderType: "market",
			Type:      "sell",
			Volume:    vol,
		}
	} else if vol != "" {
		position.ExitOrder.Volume = vol
	}

	position.ExitTrigger = trigger
	position.Transition(runtime.BUSY)

	return nil
}

/*
Validate checks the fields a live position needs to mark and exit.
EntryPath is strategy attribution — trade recover often has none; do not require it.
*/
func (position *Position) Validate() error {
	return errnie.Require(map[string]any{
		"id":                   position.ID,
		"symbol":               position.Symbol,
		"tier_key":             position.TierKey,
		"qty":                  position.Qty,
		"entry":                position.Entry,
		"entry_order":          position.EntryOrder,
		"entry_venue_order_id": position.EntryVenueOrderID,
		"fills":                position.Fills,
		"price":                position.price,
		"on_settled":           position.onSettled,
	})
}

/*
Apply is the only fill path. Entry fills refresh Qty/Entry/PnL; exit fills only
stamp ClosedAt once buys and sells net to zero — PnL stays the last mark.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	if (execution.ExecType != "" && execution.ExecType != "trade") ||
		execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	for _, fill := range position.Fills {
		if fill != nil && fill.ExecID == execution.ExecID && execution.ExecID != "" {
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
			position.Transition(runtime.READY)
		}

		closed = false
	}

	position.Fills = append(position.Fills, &execution)

	var net, bought, notional decimal.Decimal

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

	position.Qty = &net

	if bought.Sign() > 0 {
		vwap := notional.Div(bought)
		position.Entry = &vwap
	}

	if execution.Side == "sell" {
		if position.ClosedAt == nil && position.Qty.Sign() <= 0 {
			position.ClosedAt = &execution.Timestamp
			position.Transition(runtime.DONE)
		}

		return
	}

	if position.Qty != nil && position.Qty.Sign() > 0 {
		if position.Status() != runtime.READY {
			position.Transition(runtime.READY)
		}
	}

	if position.price == nil {
		return
	}

	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)
}
