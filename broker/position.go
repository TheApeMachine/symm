package broker

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
)

/*
positionTraceCap bounds one lot's net percent path. The path is the episode
the exit policy learns from, and a lot that runs for days would otherwise
grow it without end. The oldest marks are the ones a settled round trip no
longer needs.
*/
const positionTraceCap = 4096

type Position struct {
	mu              sync.Mutex
	*runtime.System
	stopLoss        *StopLoss
	transport       Transport
	price           *Price
	onSettled       func(*Position)
	ID              string                  `json:"id"`
	EntryPath       string                  `json:"entry_path"`
	HoldPath        string                  `json:"hold_path,omitempty"`
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
	Qty             *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice      *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice       *decimal.Decimal        `json:"exit_price,omitempty"`
	EntryFee        *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee         *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark            *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP        *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom        *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL             *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct          *decimal.Decimal        `json:"pnl_pct,omitempty"`
	OpenPnL         *decimal.Decimal        `json:"open_pnl,omitempty"`   // First mark-to-exit PnL after open
	LastPnL         *decimal.Decimal        `json:"last_pnl,omitempty"`   // Previous tick PnL for momentum checks
	PnLTrace        []float64               `json:"pnl_trace,omitempty"`  // Net percent, one entry per reprice
	StopState       StopState               `json:"stop_state,omitempty"` // Live proximity of each stop check
	PeakPnLPct      *decimal.Decimal        `json:"peak_pnl_pct,omitempty"`
	PeakStep        *decimal.Decimal        `json:"peak_step,omitempty"`         // Largest green up-step, in PnL percent
	Impulse         bool                    `json:"impulse,omitempty"`           // True explosive spike detected
	ExitPullbackPct *decimal.Decimal        `json:"exit_pullback_pct,omitempty"` // Learned trailing giveback from peak
	NurseryCutPct   *decimal.Decimal        `json:"nursery_cut_pct,omitempty"`   // Learned invalidation cut
	MaxLifetimeNs   int64                   `json:"max_lifetime_ns,omitempty"`   // Legacy empirical duration limit
	MaxLifetimeBars int                     `json:"max_lifetime_bars,omitempty"` // Empirical winning horizon in reprice bars
	WorstPct        *decimal.Decimal        `json:"worst_pct,omitempty"`
	paceKnown       bool
	spreadPct       *decimal.Decimal
	frictionPct     *decimal.Decimal
	stallTicks      int
	lastMark        *decimal.Decimal
	HoldNs          int64   `json:"hold_ns,omitempty"`
	Virtual         bool    `json:"virtual"`
	PathFound       bool    `json:"path_found,omitempty"`
	OpenSurprise    float64 `json:"open_surprise,omitempty"`
	BarsHeld        int64   `json:"bars_held,omitempty"`    // Repress events since entry (tape timebase)
	RealizedPct     float64 `json:"realized_pct,omitempty"` // Fee-net realized return captured at exit (not mark)
}

func NewPosition(
	ctx context.Context,
	symbol string,
	path string,
	pathFound bool,
	virtual bool,
	price *Price,
	transport Transport,
	onSettled func(*Position),
) *Position {
	return &Position{
		System:    runtime.NewSystem(ctx, "position"),
		price:     price,
		stopLoss:  NewStopLoss(),
		transport: transport,
		onSettled: onSettled,
		ID:        uuid.NewString(),
		Fills:     make([]*kraken.ExecutionData, 0),
		Symbol:    symbol,
		EntryPath: path,
		PathFound: pathFound,
		Virtual:   virtual,
	}
}

/*
Update refreshes the mark and reprices open PnL. A fired exit starts on
its own goroutine so the trade ingress can publish the next tick.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	var path string

	position.mu.Lock()

	if position.ClosedAt == nil {
		position.Mark = mark
		position.repriceLocked()

		if position.stopLoss != nil {
			if trigger, ok := position.stopLoss.Evaluate(position); ok {
				position.ExitTrigger = trigger
				position.OutcomeTriggers = []string{trigger}
				errnie.Info("[exit] triggered by " + trigger + " for " + position.Symbol)

				if position.ExitPath != "" {
					path = position.ExitPath
				}
			}
		}
	}

	position.mu.Unlock()

	if path == "" {
		return
	}

	go position.finishExit(path)
}

/*
ShouldReprice reports whether a lot needs a mark refresh on this ingress
path. Desk.Update uses it under the lot lock so skip logic does not race
Refresh or serialization.
*/
func (position *Position) ShouldReprice(mark *decimal.Decimal, isTraded bool) bool {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ClosedAt != nil || mark == nil {
		return false
	}

	markUnchanged := position.Mark != nil && position.Mark.Cmp(mark) == 0

	watchProfit := false
	if position.stopLoss != nil {
		watchProfit = position.stopLoss.ProfitLocked(position)
	}

	if position.PnL != nil && position.PnL.Sign() > 0 {
		watchProfit = true
	}

	return isTraded || !markUnchanged || watchProfit
}

func (position *Position) finishExit(path string) {
	err := position.Exit(path)

	if err != nil && !errors.Is(err, ErrVenueFlat) {
		errnie.Error(err)
		return
	}

	if errors.Is(err, ErrVenueFlat) {
		errnie.Warn("[position] " + position.Symbol + " is already flat at the venue; closing the lot")
	}

	if position.onSettled != nil {
		position.onSettled(position)
	}
}

/*
Enter records the buy. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate is called once inside Enter.
*/
func (position *Position) Enter(path string, qty *decimal.Decimal) error {
	position.mu.Lock()

	if path == "" {
		position.mu.Unlock()
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] missing path for "+position.Symbol,
			nil,
		)
	}

	if qty == nil || qty.Sign() <= 0 {
		var why string
		qty, why = position.price.Allocate(position.Symbol)

		if qty == nil || qty.Sign() <= 0 {
			msg := "[position.Enter] cannot size " + position.Symbol
			if why != "" {
				msg += ": " + why
			}

			position.mu.Unlock()
			return errnie.Err(errnie.NotFound, msg, nil)
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
	position.Qty = qty
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path

	if position.Virtual {
		fill, err := position.price.EntryCost(position.Symbol, qty)

		if err != nil {
			position.mu.Unlock()
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[position.Enter] no fill price for "+position.Symbol,
				err,
			))
		}

		position.applyLocked(kraken.ExecutionData{
			Symbol:      position.Symbol,
			Side:        "buy",
			ExecType:    "Fill",
			LastQty:     qty,
			CumQty:      qty,
			LastPrice:   fill.EntryPrice,
			AvgPrice:    fill.EntryPrice,
			FeeUsdEquiv: fill.EntryFee,
			Timestamp:   time.Now().UTC(),
		})
	}

	order := position.EntryOrder
	position.mu.Unlock()

	return position.Send(order)
}

// ErrExitInProgress means another caller already owns the sell.
var ErrExitInProgress = errors.New("exit already in progress")

// ErrVenueFlat means the paper wallet no longer holds the asset, so the sell cannot be sent again.
var ErrVenueFlat = errors.New("paper venue is already flat")

func venueFlat(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "insufficient")
}

/*
Exit records the sell and reprices from the bid book.
The venue send happens after the position lock is released, so the
fill can be applied while this call waits on the paper CLI.
*/

func (position *Position) Exit(path string) error {
	position.mu.Lock()

	if path == "" {
		position.mu.Unlock()
		return errnie.Err(
			errnie.NotFound,
			"[position.Exit] missing path for "+position.Symbol,
			nil,
		)
	}

	if position.ExitTrigger == "" {
		position.ExitTrigger = "policy exit match " + path
		position.OutcomeTriggers = []string{position.ExitTrigger}
	}

	if position.ClosedAt != nil {
		position.mu.Unlock()
		return nil
	}

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitPath = path

	if position.price != nil {
		position.ExitVWAP = position.price.ExitVWAP(position.Symbol, position.Qty)
		position.ExitPrice = position.ExitVWAP
	}

	if !position.OpenedAt.IsZero() {
		position.HoldNs = int64(now.Sub(position.OpenedAt))
	}

	position.ExitTick = data.Tick.Load()
	position.repriceLocked()
	order := position.ExitOrder

	position.mu.Unlock()

	if err := position.Send(order); err != nil {
		position.mu.Lock()
		defer position.mu.Unlock()

		if venueFlat(err) {
			return ErrVenueFlat
		}

		position.ClosedAt = nil
		return err
	}

	return nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()
	position.applyLocked(execution)
}

func (position *Position) applyLocked(execution kraken.ExecutionData) {
	position.Fills = append(position.Fills, &execution)

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.Qty = execution.CumQty
		} else if position.Qty == nil {
			position.Qty = execution.LastQty
		} else if position.Qty != execution.LastQty {
			position.Qty = position.Qty.Add(execution.LastQty)
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.EntryPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.EntryPrice = execution.LastPrice
		}

		if position.Mark == nil {
			position.Mark = position.EntryPrice
		}

		position.repriceLocked()
		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.ExitPrice = execution.AvgPrice
		} else if execution.LastPrice != nil && execution.LastPrice.Sign() > 0 {
			position.ExitPrice = execution.LastPrice
		}

		if position.Qty != nil {
			position.Qty = position.Qty.Sub(execution.LastQty)

			if position.Qty.Sign() <= 0 {
				position.Qty = decimal.NewFromFloat64(0)
			}
		}

		position.repriceLocked()
	}

	if position.ClosedAt != nil && position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}
}

func (position *Position) repriceLocked() {
	// The mark is the round trip, so it goes into the lot's own net percent
	// path before anything else can bail out of this reprice. A lot restored
	// without a price book, or one whose book cannot fill its size, still
	// settled marks, and those marks are the episode the exit policy learns
	// from.
	if position.PnLPct != nil {
		position.recordTraceLocked(position.PnLPct.Float64())
	}

	if position.price == nil {
		return
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now

	fee := position.price.Fee(position.Symbol)

	position.ExitVWAP = position.price.ExitVWAP(position.Symbol, position.Qty)
	position.ExitRoom = position.price.ExitRoom(position.Symbol, position.Qty)

	newPnL := position.price.PnL(position.Symbol, position.Qty, position.EntryPrice, fee)
	newPnLPct := position.price.PnLPct(position.Symbol, position.Qty, position.EntryPrice, fee)

	if newPnL == nil {
		return
	}

	if friction, ok := position.price.Friction(position.Symbol); ok && friction != nil {
		position.frictionPct = friction
	}

	if position.OpenPnL == nil && position.ClosedAt == nil {
		position.OpenPnL = newPnL
	}

	if position.PnL != nil {
		position.LastPnL = position.PnL
	}

	position.PnL = newPnL
	position.PnLPct = newPnLPct

	if position.PnLPct != nil {
		if position.PeakPnLPct == nil || position.PnLPct.Cmp(position.PeakPnLPct) > 0 {
			position.PeakPnLPct = position.PnLPct
		}
	}

	if position.ClosedAt == nil && position.LastPnL == nil {
		position.LastPnL = position.PnL
	}

	// BarsHeld counts reprice events since entry: the tape timebase the rest of
	// the stop uses. A wall-clock horizon would expire the same 8-bar move at 3x
	// the speed on a fast tape and under-use it on a slow one, so the expiry
	// lives in bars, not seconds.
	position.BarsHeld++

	// Refresh the live stop-check proximity for the dashboard frame. This is
	// what lets the operator see, per mark, how close each of the four checks
	// is to firing, so a held lot is never a black box.
	if position.stopLoss != nil {
		position.StopState = position.stopLoss.State(position)
	}

	// Capture the fee-net realized return the moment the exit is priced
	// (ExitVWAP/ExitPrice set above, then repriceLocked ran). PnLPct is
	// mark-to-market and is overwritten on every later reprice; the realized
	// label the tree and gate train from must be the one the exit actually made.
	if position.RealizedPct == 0 && position.EntryPrice != nil && position.Qty != nil &&
		position.ExitVWAP != nil && position.EntryPrice.Sign() > 0 && position.Qty.Sign() > 0 {
		entryGross := position.EntryPrice.Mul(position.Qty)
		cost := entryGross.OffsetPercent(decimalPercent.Mul(fee))

		if cost != nil && cost.Sign() > 0 {
			netExit := position.ExitVWAP.Mul(position.Qty).OffsetPercent(decimalPercent.Mul(fee).Mul(decimalNegativeOne))
			position.RealizedPct = netExit.Sub(cost).Div(cost).Float64()
		}
	}
}

/*
recordTraceLocked appends one mark to the lot's own net percent path. The
trace is the episode: it is what the exit policy learns from once the lot
settles, so it is written on every reprice, virtual and paper alike.

The path is bounded. A lot that runs for hours would otherwise grow the
record without end, and the oldest marks are the ones a settled episode no
longer needs: the recent path is what says how this market gives back.
*/
func (position *Position) recordTraceLocked(value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	position.PnLTrace = append(position.PnLTrace, value)

	if len(position.PnLTrace) > positionTraceCap {
		position.PnLTrace = append(
			[]float64(nil),
			position.PnLTrace[len(position.PnLTrace)-positionTraceCap:]...,
		)
	}
}

/*
send an order to the exchange.
*/
func (position *Position) Send(order *spot.OrderRequest) error {
	if position.Virtual {
		return nil
	}

	if order == nil || position.Symbol == "" {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Send] paper order is missing its symbol",
			nil,
		)
	}

	buf, err := sonic.Marshal(kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    position.Symbol,
		Type:    order.Type,
		OrdType: order.OrderType,
		Volume:  order.Volume,
		Price:   order.Price,
		ClOrdId: order.ClOrdId,
	}))

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[desk.Send] failed to marshal order",
			err,
		)
	}

	return position.transport.Write(buf)
}
