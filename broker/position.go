package broker

import (
	"context"
	"errors"
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
	"github.com/theapemachine/symm/broker/stoploss"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
)

type Position struct {
	*runtime.System
	mu                sync.Mutex
	exitInProgress    atomic.Uint32
	onShadowDone      func(*Position)
	Structural        *stoploss.Structural `json:"structural,omitempty"`
	Learning          *stoploss.Learning   `json:"learning,omitempty"`
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
	OpenPnL           *decimal.Decimal        `json:"open_pnl,omitempty"`
	LastPnL           *decimal.Decimal        `json:"last_pnl,omitempty"`
	PeakPnLPct        *decimal.Decimal        `json:"peak_pnl_pct,omitempty"`
	PeakStep          *decimal.Decimal        `json:"peak_step,omitempty"`
	Impulse           bool                    `json:"impulse,omitempty"`
	ExitPullbackPct   *decimal.Decimal        `json:"exit_pullback_pct,omitempty"`
	NurseryCutPct     *decimal.Decimal        `json:"nursery_cut_pct,omitempty"`
	MaxLifetimeNs     int64                   `json:"max_lifetime_ns,omitempty"`
	MaxLifetimeBars   int                     `json:"max_lifetime_bars,omitempty"`
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
	BarsHeld          int64      `json:"bars_held,omitempty"`
	RealizedPct       float64    `json:"realized_pct,omitempty"`
	OfficialExitAt    *time.Time `json:"official_exit_at,omitempty"`
	OfficialExitPct   float64    `json:"official_exit_pct,omitempty"`
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
		ctx, symbol, path, pathFound, ledger, price, transport, onSettled,
		stoploss.NewRails(),
	)
}

/*
NewPositionWithRails wires the desk's Rails into the learning shadow.
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
	rails *stoploss.Rails,
) *Position {
	if ledger == "" {
		ledger = LedgerPaper
	}

	return &Position{
		System:     runtime.NewSystem(ctx, "position"),
		price:      price,
		Structural: stoploss.NewStructural(),
		Learning:   stoploss.NewLearning(rails),
		transport:  transport,
		onSettled:  onSettled,
		ID:         uuid.NewString(),
		Fills:      make([]*kraken.ExecutionData, 0),
		Symbol:     symbol,
		EntryPath:  path,
		PathFound:  pathFound,
		Book:       ledger.String(),
		Virtual:    ledger.Virtual(),
	}
}

/*
Ledger returns the trading book this lot belongs to.
*/
func (position *Position) Ledger() Ledger {
	return ParseLedger(position.Book, position.Virtual)
}

/*
IsClosed reports whether the lot has finished exit bookkeeping.
*/
func (position *Position) IsClosed() bool {
	position.mu.Lock()
	defer position.mu.Unlock()
	return position.ClosedAt != nil
}

/*
stopExitPath is the latest hold cut when a stop fires. Empty means the
venue can still close; model.Exit waits for a distinct regime cut before
writing an exit leaf. Never invent "stop" or reuse entry_path.
*/
func stopExitPath(position *Position) string {
	if n := len(position.HoldPaths); n > 0 {
		path := position.HoldPaths[n-1]
		if path != "" && path != position.EntryPath {
			return path
		}
	}

	return ""
}

/*
Update refreshes the mark and reprices open PnL. Stops own their state;
Position only arms, updates, and exits when structural fires.
*/
func (position *Position) Update(mark *decimal.Decimal) {
	var path string

	position.mu.Lock()

	if position.ClosedAt == nil {
		position.Mark = mark
		position.repriceLocked()

		if position.PnLPct != nil {
			current := position.PnLPct.MustFloat64()
			position.armStopsLocked()

			if trigger, ok := position.Structural.Update(current); ok {
				position.ExitTrigger = trigger
				position.OutcomeTriggers = []string{trigger}
				path = stopExitPath(position)
			}

			if _, ok := position.Learning.Update(current); ok && position.OfficialExitAt != nil {
				position.Learning.Settle(position.OfficialExitPct)
			}
		}
	}

	fired := position.ExitTrigger != "" && position.ClosedAt == nil
	position.mu.Unlock()

	if !fired {
		return
	}

	if !position.exitInProgress.CompareAndSwap(0, 1) {
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
	if mark == nil {
		return false
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if !position.Learning.Live() {
		return false
	}

	if position.price == nil || position.EntryPrice == nil {
		return false
	}

	// Sell fills may zero Qty after the official settle; shadow still marks
	// the entry size so the learning episode can finish.
	qty := position.Qty
	if qty == nil || qty.Sign() <= 0 {
		qty = position.shadowQtyLocked()
	}

	if qty == nil || qty.Sign() <= 0 {
		return false
	}

	fee := position.price.feeForTrade(position.Symbol)
	pct := position.price.roundTripPct(
		position.Symbol, qty, position.EntryPrice, fee, fee,
	)

	if pct == nil {
		return false
	}

	current := pct.MustFloat64()
	position.Mark = mark
	position.PnLPct = pct
	position.armStopsLocked()

	_, ok := position.Learning.Update(current)
	if !ok {
		return false
	}

	if position.OfficialExitAt != nil {
		position.Learning.Settle(position.OfficialExitPct)
	}

	return true
}

/*
armStopsLocked freezes both stops from entry friction once.
*/
func (position *Position) armStopsLocked() {
	friction := 0.0

	if position.frictionPct != nil && position.frictionPct.Sign() > 0 {
		friction = position.frictionPct.MustFloat64()
	} else if position.price != nil {
		if hurdle, ok := position.price.Friction(position.Symbol); ok && hurdle != nil {
			friction = hurdle.MustFloat64()
			position.frictionPct = hurdle
		}
	}

	position.Structural.Arm(friction)
	position.Learning.Arm(friction)
}

/*
shadowQtyLocked recovers the entry size after a sell fill burned Qty.
*/
func (position *Position) shadowQtyLocked() *decimal.Decimal {
	if position.ExitOrder != nil && position.ExitOrder.Volume != "" {
		if qty, err := decimal.NewFromString(position.ExitOrder.Volume); err == nil && qty.Sign() > 0 {
			return qty
		}
	}

	if position.EntryOrder != nil && position.EntryOrder.Volume != "" {
		if qty, err := decimal.NewFromString(position.EntryOrder.Volume); err == nil && qty.Sign() > 0 {
			return qty
		}
	}

	return nil
}

/*
ShadowLive reports whether the learning ghost still wants marks.
*/
func (position *Position) ShadowLive() bool {
	position.mu.Lock()
	defer position.mu.Unlock()

	return position.Learning.Live()
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
			position.exitInProgress.Store(0)
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
	if position.Learning.ExitAt != nil {
		position.Learning.Settle(position.OfficialExitPct)
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

	// Stop-triggered exits may arrive with an empty cut; the model fills
	// ExitPath later from a live regime path distinct from entry.
	if path == "" && position.ExitTrigger == "" {
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

	if path != "" && path != position.EntryPath {
		position.ExitPath = path
	}

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
