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

func (position *Position) qty() *decimal.Decimal {
	qty := decimal.NewFromInt(0)

	for _, fill := range position.Fills {
		if fill == nil || fill.LastQty == nil {
			continue
		}

		switch fill.Side {
		case "buy":
			qty = qty.Add(*fill.LastQty)
		case "sell":
			qty = qty.Sub(*fill.LastQty)
		}
	}

	return &qty
}

func (position *Position) entry() *decimal.Decimal {
	var cost, qty decimal.Decimal

	for _, fill := range position.Fills {
		if fill == nil || fill.Side != "buy" ||
			fill.LastPrice == nil || fill.LastQty == nil {
			continue
		}

		cost = cost.Add(fill.LastPrice.Mul(*fill.LastQty))
		qty = qty.Add(*fill.LastQty)
	}

	if qty.Sign() <= 0 {
		zero := decimal.Zero
		return &zero
	}

	vwap := cost.Div(qty)
	return &vwap
}

/*
Update marks a READY position. PnL always uses the best available mark so the
hub chart gets a y on every Hello. Stoploss only arms/triggers when the visible
book can absorb the full Qty (Walk VWAP). Other stages are a silent no-op.
*/
func (position *Position) Update(hint *decimal.Decimal) error {
	if hint == nil {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	if position.Status() != runtime.READY {
		return nil
	}

	if position.Qty == nil || position.Qty.Sign() <= 0 || position.price == nil {
		return nil
	}

	vwap, fully := position.price.Liquidation(position.Symbol, position.Qty)

	if fully && vwap != nil {
		position.Mark = vwap
	} else {
		position.Mark = hint
	}

	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)

	if position.PnLPct == nil || !fully {
		return nil
	}

	friction := position.price.ExhaustionReading(position.Symbol)

	if room := position.price.ExitRoom(position.Symbol, position.Qty); room != nil {
		if r, _ := room.Float64(); r > 0 {
			friction = math.Max(friction, 1/(1+r))
		}
	}

	position.mu.Lock()

	pnlpct, _ := position.PnLPct.Float64()
	arm := position.price.StoplossArmPctFor(position.Symbol, position.Qty)

	if position.Stoploss == nil {
		position.Stoploss = NewStoploss(pnlpct, arm)
	} else if arm > position.Stoploss.ArmPnlPct {
		position.Stoploss.ArmPnlPct = arm
	}

	position.Stoploss.Update(pnlpct, friction)

	if !position.Stoploss.Triggered() {
		position.mu.Unlock()
		return nil
	}

	position.Transition(runtime.BUSY)
	position.mu.Unlock()

	if position.onSettled != nil {
		position.onSettled(position)
	}

	return nil
}

/*
AbortExit cancels a pending sell after the pre-send guard refuses a loss exit.
BUSY → READY so marks resume; Stoploss re-seeds Healthy.
*/
func (position *Position) AbortExit() {
	if position == nil {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.Stoploss != nil {
		position.Stoploss.Reset()
	}

	position.ExitPath = ""
	position.ExitTrigger = ""
	position.ExitVenueOrderID = ""

	if position.Status() == runtime.BUSY {
		position.Transition(runtime.READY)
	}
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
		Volume:    vol,
	}

	position.OpenedAt = time.Now().UTC()
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path
	position.Transition(runtime.WAITING)

	return nil
}

/*
Exit arms Closing and moves READY → BUSY. ClosedAt / DONE wait for exit fills.
*/
func (position *Position) exit() error {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ExitOrder == nil {
		position.ExitOrder = &spot.OrderRequest{
			ClOrdId:   uuid.NewString(),
			OrderType: "market",
			Type:      "sell",
			Volume:    position.Qty.String(),
		}
	}

	position.ExitTrigger = "policy exit match " + position.ExitPath

	if position.ExitOrder != nil && position.Qty != nil {
		position.ExitOrder.Volume = position.Qty.String()
	}

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
Bind attaches a restored wire order onto the matching entry or exit slot.
*/
func (position *Position) Bind(txid, side string, order *spot.OrderRequest) {
	if position == nil || txid == "" || order == nil {
		return
	}

	switch side {
	case "buy":
		if position.EntryOrder == nil {
			position.EntryOrder = order
		}

		if position.EntryVenueOrderID == "" {
			position.EntryVenueOrderID = txid
		}
	case "sell":
		if position.ExitOrder == nil {
			position.ExitOrder = order
		}

		if position.ExitVenueOrderID == "" {
			position.ExitVenueOrderID = txid
		}
	}
}

/*
owns is true when this fill belongs to the position. No order id means refuse —
symbol-only matching flats real holdings on unrelated account prints.
*/
func (position *Position) owns(data *market.ExecutionData) bool {
	if data == nil {
		return false
	}

	if data.ClientOrderID != "" {
		if position.EntryOrder != nil && position.EntryOrder.ClOrdId == data.ClientOrderID {
			return true
		}

		if position.ExitOrder != nil && position.ExitOrder.ClOrdId == data.ClientOrderID {
			return true
		}

		return false
	}

	if data.OrderID != "" {
		if data.OrderID == position.EntryVenueOrderID ||
			data.OrderID == position.ExitVenueOrderID {
			return true
		}

		for _, fill := range position.Fills {
			if fill != nil && fill.OrderID == data.OrderID {
				return true
			}
		}
	}

	return false
}

/*
Apply is the only fill path. Entry fills refresh Qty/Entry/PnL; exit fills only
stamp ClosedAt once buys and sells net to zero — PnL stays the last mark.
*/
func (position *Position) Apply(execution market.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

	for _, fill := range position.Fills {
		if fill != nil && fill.ExecID == execution.ExecID && execution.ExecID != "" {
			return
		}
	}

	position.Fills = append(position.Fills, &execution)
	position.Qty = position.qty()
	position.Entry = position.entry()

	if execution.Side == "sell" {
		if position.ClosedAt == nil && position.Qty.Sign() == 0 {
			position.ClosedAt = &execution.Timestamp
			position.Transition(runtime.DONE)
		}

		return
	}

	if position.Qty != nil && position.Qty.Sign() > 0 {
		position.Transition(runtime.READY)
	}

	if position.price == nil {
		return
	}

	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)
}
