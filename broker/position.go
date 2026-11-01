package broker

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
)

/*
positionTraceCap bounds one lot's net percent path. The path is the episode
the exit policy learns from, and a lot that runs for days would otherwise
grow it without end. The oldest marks are the ones a settled round trip no
longer needs.
*/
const positionTraceCap = 4096

type Position struct {
	*runtime.System
	mu                sync.Mutex
	exitInProgress    uint32
	structural   *StructuralStop // active settler
	learning     *LearningStop   // evaluated shadow; never settles
	onShadowDone func(*Position)
	transport         Transport
	price             *Price
	onSettled         func(*Position)
	ID                string                  `json:"id"`
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
	Fills             []*kraken.ExecutionData `json:"fills,omitempty"`
	Qty               *decimal.Decimal        `json:"qty,omitempty"`
	EntryPrice        *decimal.Decimal        `json:"entry_price,omitempty"`
	ExitPrice         *decimal.Decimal        `json:"exit_price,omitempty"`
	EntryFee          *decimal.Decimal        `json:"entry_fee,omitempty"`
	ExitFee           *decimal.Decimal        `json:"exit_fee,omitempty"`
	Mark              *decimal.Decimal        `json:"mark,omitempty"`
	ExitVWAP          *decimal.Decimal        `json:"exit_vwap,omitempty"`
	ExitRoom          *decimal.Decimal        `json:"exit_room,omitempty"`
	PnL               *decimal.Decimal        `json:"pnl,omitempty"`
	PnLPct            *decimal.Decimal        `json:"pnl_pct,omitempty"`
	OpenPnL           *decimal.Decimal        `json:"open_pnl,omitempty"`   // First mark-to-exit PnL after open
	LastPnL           *decimal.Decimal        `json:"last_pnl,omitempty"`   // Previous tick PnL for momentum checks
	PnLTrace          []float64               `json:"pnl_trace,omitempty"`  // Net percent, one entry per reprice
	StopState         StopState               `json:"stop_state,omitempty"` // Live proximity of each stop check
	PeakPnLPct        *decimal.Decimal        `json:"peak_pnl_pct,omitempty"`
	PeakStep          *decimal.Decimal        `json:"peak_step,omitempty"`         // Largest green up-step, in PnL percent
	Impulse           bool                    `json:"impulse,omitempty"`           // True explosive spike detected
	ExitPullbackPct   *decimal.Decimal        `json:"exit_pullback_pct,omitempty"` // Learned trailing giveback from peak
	NurseryCutPct     *decimal.Decimal        `json:"nursery_cut_pct,omitempty"`   // Learned invalidation cut
	MaxLifetimeNs     int64                   `json:"max_lifetime_ns,omitempty"`   // Legacy empirical duration limit
	MaxLifetimeBars   int                     `json:"max_lifetime_bars,omitempty"` // Empirical winning horizon in reprice bars
	WorstPct          *decimal.Decimal        `json:"worst_pct,omitempty"`
	paceKnown         bool
	spreadPct         *decimal.Decimal
	frictionPct       *decimal.Decimal
	stallTicks        int
	lastMark          *decimal.Decimal
	HoldNs            int64      `json:"hold_ns,omitempty"`
	Book              string     `json:"book,omitempty"`
	Virtual           bool       `json:"virtual"`
	PathFound         bool       `json:"path_found,omitempty"`
	OpenSurprise      float64    `json:"open_surprise,omitempty"`
	EnterR02          float64    `json:"enter_r02_brightness,omitempty"`
	EnterNetHold      float64    `json:"enter_net_hold,omitempty"`
	EnterGridMaturity float64    `json:"enter_grid_maturity,omitempty"`
	BarsHeld          int64      `json:"bars_held,omitempty"`    // Reprice events since entry (tape timebase)
	RealizedPct       float64    `json:"realized_pct,omitempty"` // Fee-net realized return captured at exit (not mark)
	ShadowActive      bool       `json:"shadow_active,omitempty"`
	ShadowPnLTrace    []float64  `json:"shadow_pnl_trace,omitempty"`
	ShadowExitTrigger string     `json:"shadow_exit_trigger,omitempty"`
	OfficialExitAt    *time.Time `json:"official_exit_at,omitempty"`
	ShadowExitAt      *time.Time `json:"shadow_exit_at,omitempty"`
	OfficialExitPct   float64    `json:"official_exit_pct,omitempty"`
	ShadowExitPct     float64    `json:"shadow_exit_pct,omitempty"`
	StopFloorPct      float64    `json:"stop_floor_pct,omitempty"`
	ShadowFloorPct    float64    `json:"shadow_floor_pct,omitempty"`
	StopFloorTrace    []float64  `json:"stop_floor_trace,omitempty"`
	ShadowFloorTrace  []float64  `json:"shadow_floor_trace,omitempty"`
	StopProximity     float64    `json:"stop_proximity,omitempty"`
	StopRoomPct       float64    `json:"stop_room_pct,omitempty"`
	ShadowProximity   float64    `json:"shadow_proximity,omitempty"`
	ShadowRoomPct     float64    `json:"shadow_room_pct,omitempty"`
}

func NewPosition(
	ctx context.Context,
	symbol string,
	path string,
	pathFound bool,
	ledger Ledger,
	price *Price,
	transport Transport,
	onSettled func(*Position),
) *Position {
	return NewPositionWithRails(
		ctx, symbol, path, pathFound, ledger, price, transport, onSettled, nil,
	)
}

/*
NewPositionWithRails wires the desk's AdaptiveRails into the learning shadow.
*/
func NewPositionWithRails(
	ctx context.Context,
	symbol string,
	path string,
	pathFound bool,
	ledger Ledger,
	price *Price,
	transport Transport,
	onSettled func(*Position),
	rails *AdaptiveRails,
) *Position {
	if ledger == "" {
		ledger = LedgerPaper
	}

	position := &Position{
		System:       runtime.NewSystem(ctx, "position"),
		price:        price,
		structural:   NewStructuralStop(),
		learning:     NewLearningStop(rails),
		transport:    transport,
		onSettled:    onSettled,
		ID:           uuid.NewString(),
		Fills:        make([]*kraken.ExecutionData, 0),
		Symbol:       symbol,
		EntryPath:    path,
		PathFound:    pathFound,
		Book:         ledger.String(),
		Virtual:      ledger.Virtual(),
		ShadowActive: true,
	}

	return position
}

/*
Ledger returns the trading book this lot belongs to.
*/
func (position *Position) Ledger() Ledger {
	if position == nil {
		return LedgerPaper
	}

	return ParseLedger(position.Book, position.Virtual)
}

/*
IsClosed reports whether the lot has finished exit bookkeeping.
*/
func (position *Position) IsClosed() bool {
	if position == nil {
		return false
	}

	position.mu.Lock()
	defer position.mu.Unlock()
	return position.ClosedAt != nil
}

/*
stopExitPath is the path argument passed into Exit when a stop fires.
exit_path on the lot is still empty until Exit runs. Never use entry_path —
only the latest hold spell, or a fixed stop tag.
*/
func stopExitPath(position *Position) string {
	if position == nil {
		return "stop"
	}

	if n := len(position.HoldPaths); n > 0 {
		return position.HoldPaths[n-1]
	}

	return "stop"
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

		if position.PnLPct != nil {
			current := position.PnLPct.MustFloat64()
			position.armLossBudgetLocked()

			// Active settler: structural only.
			if position.structural != nil && !position.structural.Done() {
				if trigger, ok := position.structural.Update(current); ok {
					position.ExitTrigger = trigger
					position.OutcomeTriggers = []string{trigger}
					path = stopExitPath(position)
				}

				position.StopFloorPct = position.structural.Floor()
				position.StopRoomPct = current - position.StopFloorPct
				position.StopProximity = shadowProximity(current, position.StopFloorPct)
				position.StopState = StopState{
					CurrentPct: current,
					PeakPct:    position.structural.Peak(),
					FloorPct:   position.StopFloorPct,
					RoomPct:    position.StopRoomPct,
					Proximity:  position.StopProximity,
				}
				position.recordStopFloorTraceLocked(position.StopFloorPct)
			}

			// Evaluated shadow: learning stop never settles.
			if position.learning != nil && position.ShadowActive && !position.learning.Done() {
				position.recordShadowTraceLocked(current)

				if trigger, ok := position.learning.Update(current); ok {
					position.finishShadowLocked(trigger, current)
				}

				position.ShadowFloorPct = position.learning.ActiveFloor(current)
				position.ShadowRoomPct = current - position.ShadowFloorPct
				position.ShadowProximity = position.learning.Proximity(current)
				position.recordShadowFloorTraceLocked(position.ShadowFloorPct)
			}
		}
	}

	position.mu.Unlock()

	if path == "" {
		return
	}

	if !atomic.CompareAndSwapUint32(&position.exitInProgress, 0, 1) {
		return
	}

	errnie.Info("[exit] triggered by " + position.ExitTrigger + " for " + position.Symbol)
	go position.finishExit(path)
}

/*
UpdateShadow continues the learning observation after the official lot settled.
Returns true when the shadow episode ends on this mark.
*/
func (position *Position) UpdateShadow(mark *decimal.Decimal) bool {
	if position == nil || mark == nil {
		return false
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if !position.ShadowActive || position.learning == nil || position.learning.Done() {
		return false
	}

	if position.price == nil || position.EntryPrice == nil || position.Qty == nil {
		return false
	}

	fee := position.price.feeForTrade(position.Symbol)
	pct := position.price.roundTripPct(
		position.Symbol, position.Qty, position.EntryPrice, fee, fee,
	)

	if pct == nil {
		return false
	}

	current := pct.MustFloat64()
	position.Mark = mark
	position.PnLPct = pct
	position.armLossBudgetLocked()
	position.recordShadowTraceLocked(current)

	trigger, ok := position.learning.Update(current)
	position.ShadowFloorPct = position.learning.ActiveFloor(current)
	position.ShadowRoomPct = current - position.ShadowFloorPct
	position.ShadowProximity = position.learning.Proximity(current)
	position.recordShadowFloorTraceLocked(position.ShadowFloorPct)
	if !ok {
		return false
	}

	position.finishShadowLocked(trigger, current)
	return true
}

func (position *Position) finishShadowLocked(trigger string, current float64) {
	now := time.Now().UTC()
	position.ShadowActive = false
	position.ShadowExitTrigger = trigger
	position.ShadowExitAt = &now
	position.ShadowExitPct = current

	if position.learning != nil {
		position.learning.Finish(trigger, current)

		if position.OfficialExitAt != nil {
			position.learning.Settle(position.OfficialExitPct)
		}
	}
}

func (position *Position) recordShadowTraceLocked(value float64) {
	position.recordPctPathLocked(&position.ShadowPnLTrace, value)
}

func (position *Position) recordStopFloorTraceLocked(value float64) {
	position.recordPctPathLocked(&position.StopFloorTrace, value)
}

func (position *Position) recordShadowFloorTraceLocked(value float64) {
	position.recordPctPathLocked(&position.ShadowFloorTrace, value)
}

func (position *Position) recordPctPathLocked(path *[]float64, value float64) {
	if path == nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}

	if len(*path) >= positionTraceCap {
		copy(*path, (*path)[1:])
		(*path)[len(*path)-1] = value
		return
	}

	*path = append(*path, value)
}

/*
armLossBudgetLocked freezes structural zone-0 from entry friction once, and
arms the learning shadow from AdaptiveRails on the same friction print.
*/
func (position *Position) armLossBudgetLocked() {
	if position == nil {
		return
	}

	if position.structural != nil && position.structural.budget != nil &&
		position.structural.budget.Armed() {
		return
	}

	friction := 0.0
	if position.frictionPct != nil && position.frictionPct.Sign() > 0 {
		friction = position.frictionPct.MustFloat64()
	} else if position.price != nil {
		if hurdle, ok := position.price.Friction(position.Symbol); ok && hurdle != nil {
			friction = hurdle.MustFloat64()
			position.frictionPct = hurdle
		}
	}

	budget := NewLossBudget().Arm(friction)

	if position.structural != nil {
		position.structural.BindBudget(budget)
	}

	if position.learning != nil {
		position.learning.Arm(friction)
	}
}

/*
shadowProximity maps room above the structural floor onto 0..1 using a 2pp
span so the meter reads like the official invalidation budget.
*/
func shadowProximity(current, floor float64) float64 {
	room := current - floor
	if room <= 0 {
		return 1
	}

	const span = 2.0
	if room >= span {
		return 0
	}

	return 1 - room/span
}

/*
ShadowLive reports whether the learning ghost still wants marks.
*/
func (position *Position) ShadowLive() bool {
	if position == nil {
		return false
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	return position.ShadowActive && position.learning != nil && !position.learning.Done()
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

	return isTraded || !markUnchanged
}

func (position *Position) finishExit(path string) {
	defer func() {
		position.mu.Lock()
		closed := position.ClosedAt != nil
		position.mu.Unlock()
		if !closed {
			atomic.StoreUint32(&position.exitInProgress, 0)
		}
	}()

	err := position.Exit(path)

	if err != nil && !errors.Is(err, ErrVenueFlat) {
		errnie.Error(err)
		return
	}

	if errors.Is(err, ErrVenueFlat) {
		errnie.Warn("[position] " + position.Symbol + " is already flat at the venue; closing the lot")
	}

	position.mu.Lock()
	now := time.Now().UTC()
	position.OfficialExitAt = &now
	if position.PnLPct != nil {
		position.OfficialExitPct = position.PnLPct.MustFloat64()
	} else if position.RealizedPct != 0 {
		position.OfficialExitPct = position.RealizedPct * 100
	}

	// Learning already finished during the open → score regret now.
	if position.learning != nil && position.learning.Done() && !position.ShadowActive {
		position.learning.Settle(position.OfficialExitPct)
	}

	position.mu.Unlock()

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
		if err := position.fillTakerEntryLocked(); err != nil {
			position.mu.Unlock()
			return err
		}

		position.mu.Unlock()
		return nil
	}

	order := position.EntryOrder
	position.mu.Unlock()

	return position.Send(order)
}

func (position *Position) Unfilled() bool {
	if position == nil {
		return false
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ClosedAt != nil {
		return false
	}

	if position.EntryPrice != nil && position.EntryPrice.Sign() > 0 {
		return false
	}

	return position.Qty != nil && position.Qty.Sign() > 0
}

func (position *Position) fillTakerEntryLocked() error {
	if position.price == nil || position.Qty == nil || position.Qty.Sign() <= 0 {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] cannot fill virtual entry for "+position.Symbol,
			nil,
		)
	}

	fill, err := position.price.EntryCost(position.Symbol, position.Qty)
	if err != nil || fill == nil || fill.EntryPrice == nil || fill.EntryPrice.Sign() <= 0 {
		msg := "[position.Enter] cannot fill virtual entry for " + position.Symbol
		if err != nil {
			msg += ": " + err.Error()
		}

		return errnie.Err(errnie.NotFound, msg, err)
	}

	position.applyLocked(kraken.ExecutionData{
		Symbol:       position.Symbol,
		Side:         "buy",
		ExecType:     "trade",
		LiquidityInd: "t",
		LastQty:      position.Qty,
		CumQty:       position.Qty,
		LastPrice:    fill.EntryPrice,
		AvgPrice:     fill.EntryPrice,
		FeeUsdEquiv:  fill.EntryFee,
	})

	if position.EntryPrice == nil {
		return errnie.Err(
			errnie.NotFound,
			"[position.Enter] virtual taker fill did not set entry for "+position.Symbol,
			nil,
		)
	}

	return nil
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
				position.Qty = dFloat(0)
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
		position.recordTraceLocked(position.PnLPct.MustFloat64())
	}

	if position.price == nil {
		return
	}

	now := time.Now().UTC()
	position.UpdatedAt = &now

	fee := position.price.feeForTrade(position.Symbol)

	position.ExitVWAP = position.price.ExitVWAP(position.Symbol, position.Qty)
	position.ExitRoom = position.price.ExitRoom(position.Symbol, position.Qty)

	newPnL, _ := position.price.roundTrip(
		position.Symbol, position.Qty, position.EntryPrice, fee, fee,
	)
	newPnLPct := position.price.roundTripPct(
		position.Symbol, position.Qty, position.EntryPrice, fee, fee,
	)

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

	// BarsHeld counts reprices since entry. It is a trace statistic. It does
	// not close the lot.
	position.BarsHeld++

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
			position.RealizedPct = netExit.Sub(cost).Div(cost).MustFloat64()
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
	position.recordPctPathLocked(&position.PnLTrace, value)
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

	request := &kraken.AddOrderRequest{
		Pair:    position.Symbol,
		Type:    order.Type,
		OrdType: order.OrderType,
		Volume:  order.Volume,
		Price:   order.Price,
		ClOrdId: order.ClOrdId,
	}

	if _, live := position.transport.(*network.WebsocketClient); live {
		if position.price == nil || position.price.Instrument == nil {
			return errnie.Err(
				errnie.Validation,
				"[position.Send] live order has no instrument for a websocket token",
				nil,
			)
		}

		token, err := position.price.Instrument.WebSocketToken(position.Context())

		if err != nil {
			return err
		}

		request.Token = token
	}

	buf, err := sonic.Marshal(kraken.NewAddOrderMessage(request.Token, request))

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[desk.Send] failed to marshal order",
			err,
		)
	}

	return position.transport.Write(buf)
}
