package broker

import (
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
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
	stopLoss        *StopLoss
	exiting         atomic.Bool
	settled         atomic.Bool
	transport       Transport
	price           *Price
	onSettled       func(*Position)
	ID              string                  `json:"id"`
	EntryPath       string                  `json:"entry_path"`
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
	OpenPnL         *decimal.Decimal        `json:"open_pnl,omitempty"`  // First mark-to-exit PnL after open
	LastPnL         *decimal.Decimal        `json:"last_pnl,omitempty"`  // Previous tick PnL for momentum checks
	PnLTrace        []float64               `json:"pnl_trace,omitempty"` // Net percent, one entry per reprice
	PeakPnLPct      *decimal.Decimal        `json:"peak_pnl_pct,omitempty"`
	PeakStep        *decimal.Decimal        `json:"peak_step,omitempty"`         // Largest green up-step, in PnL percent
	Impulse         bool                    `json:"impulse,omitempty"`           // True explosive spike detected
	ExitPullbackPct *decimal.Decimal        `json:"exit_pullback_pct,omitempty"` // Learned trailing giveback from peak
	NurseryCutPct   *decimal.Decimal        `json:"nursery_cut_pct,omitempty"`   // Learned invalidation cut
	MaxLifetimeNs   int64                   `json:"max_lifetime_ns,omitempty"`   // Empirical duration limit for unignited trades
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
}

func NewPosition(
	symbol string,
	path string,
	pathFound bool,
	virtual bool,
	price *Price,
	transport Transport,
	onSettled func(*Position),
) *Position {
	return &Position{
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

	if position.ClosedAt == nil && !position.exiting.Load() {
		position.Mark = mark
		position.repriceLocked()

		// StopLoss is the SINGLE SOURCE OF TRUTH for exits
		if position.stopLoss != nil {
			if trigger, ok := position.stopLoss.Evaluate(position); ok {
				position.recordExitTriggerLocked(trigger)
				errnie.Info("[exit] triggered by " + trigger + " for " + position.Symbol)
				if position.ExitPath != "" && position.claimExit() {
					path = position.ExitPath
				}
			}
		}
	}

	if path == "" {
		return
	}

	go position.finishExit(path)
}

func (position *Position) claimExit() bool {
	return position.exiting.CompareAndSwap(false, true)
}

func (position *Position) recordExitTriggerLocked(trigger string) {
	if trigger == "" {
		return
	}

	position.ExitTrigger = trigger
	position.OutcomeTriggers = []string{trigger}
}

func (position *Position) finishExit(path string) {
	err := position.exitClaimed(path)
	if err != nil && !errors.Is(err, ErrVenueFlat) {
		position.exiting.Store(false)
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

func (position *Position) settleOnce() bool {
	return position.settled.CompareAndSwap(false, true)
}

/*
Enter records the buy. qty must be a venue-valid size from Price.Allocate;
when nil or non-positive, Allocate is called once inside Enter.
*/
func (position *Position) Enter(path string, qty *decimal.Decimal) error {
	if path == "" {
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

			return errnie.Err(errnie.NotFound, msg, nil)
		}
	}

	vol := qty.String()

	entryOrder := &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "buy",
		Volume:    vol,
	}

	exitOrder := &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "market",
		Type:      "sell",
		Volume:    vol,
	}

	now := time.Now().UTC()
	position.Qty = qty
	position.EntryOrder = entryOrder
	position.ExitOrder = exitOrder
	position.OpenedAt = now
	position.PolicyEpoch = data.Epoch.Load()
	position.EntryTick = data.Tick.Load()
	position.EntryPath = path
	if position.ExitPath == "" {
		position.ExitPath = path
	}

	if position.Virtual {
		fill, err := position.price.EntryCost(position.Symbol, qty)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[position.Enter] no fill price for "+position.Symbol,
				err,
			))
		}

		position.Apply(kraken.ExecutionData{
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

	return position.Send(entryOrder)
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
	if path == "" {
		return errnie.Err(
			errnie.NotFound,
			"[position.Exit] missing path for "+position.Symbol,
			nil,
		)
	}

	if !position.claimExit() {
		return ErrExitInProgress
	}

	if position.ExitTrigger == "" {
		position.recordExitTriggerLocked("policy exit match " + path)
	}

	if err := position.exitClaimed(path); err != nil {
		if errors.Is(err, ErrVenueFlat) {
			return nil
		}

		position.exiting.Store(false)
		return err
	}

	return nil
}

func (position *Position) exitClaimed(path string) error {
	if position.ExitOrder == nil {
		return errnie.Err(
			errnie.NotFound, "[position.Exit] no exit order for "+position.Symbol, nil,
		)
	}

	if position.ClosedAt != nil {
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

	if err := position.Send(order); err != nil {
		if venueFlat(err) {
			return ErrVenueFlat
		}

		position.ClosedAt = nil
		return err
	}

	return nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
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
