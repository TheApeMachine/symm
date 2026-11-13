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
	entry := decimal.NewFromInt(0)

	for _, fill := range position.Fills {
		if fill != nil && fill.Side == "buy" {
			entry = *fill.LastPrice
		}
	}

	return &entry
}

/*
Update sets PnL / PnLPct from the mark, feeds the stoploss, and may arm exit.
*/
func (position *Position) Update(price *decimal.Decimal) error {
	if price == nil {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] price is nil",
			nil,
		), "symbol", position.Symbol)
	}

	position.mu.Lock()

	if position.ClosedAt != nil || position.Closing {
		position.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"[position.Update] closed position",
			nil,
		), "symbol", position.Symbol)
	}

	position.mu.Unlock()

	position.Mark = price
	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)

	friction := position.price.ExhaustionReading(position.Symbol)

	if room := position.price.ExitRoom(position.Symbol, position.Qty); room != nil {
		if depth, _ := room.Float64(); depth > 0 && depth < 2.0 {
			friction = math.Max(friction, 0.6)
		}
	}

	position.mu.Lock()

	pnlpct, _ := position.PnLPct.Float64()

	if position.Stoploss == nil {
		position.Stoploss = NewStoploss(pnlpct)
	}

	position.Stoploss.Update(pnlpct, friction)

	if !position.Stoploss.Triggered() {
		position.mu.Unlock()
		return nil
	}

	position.Closing = true
	position.mu.Unlock()

	if position.onSettled != nil {
		position.onSettled(position)
	}

	return nil
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

	now := time.Now().UTC()
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path

	return nil
}

/*
Exit arms Closing for a policy exit. ClosedAt waits for exit fills in Apply.
*/
func (position *Position) exit() error {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ClosedAt != nil || position.Closing {
		return nil
	}

	position.ExitTrigger = "policy exit match " + position.ExitPath
	position.Closing = true

	return nil
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
			closedAt := execution.Timestamp

			if closedAt.IsZero() {
				closedAt = time.Now().UTC()
			}

			position.ClosedAt = &closedAt
		}

		return
	}

	if position.price == nil {
		return
	}

	position.PnL = position.price.PnL(position)
	position.PnLPct = position.price.PnLPct(position)
}
