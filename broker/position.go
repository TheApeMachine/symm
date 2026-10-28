package broker

import (
	"errors"
	"strings"
	"sync"
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

type Position struct {
	mu              sync.RWMutex
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
	OpenPnL         *decimal.Decimal        `json:"open_pnl,omitempty"` // First mark-to-exit PnL after open
	LastPnL         *decimal.Decimal        `json:"last_pnl,omitempty"` // Previous tick PnL for momentum checks
	PeakPnLPct      *decimal.Decimal        `json:"peak_pnl_pct,omitempty"`
	LowestPnLPct    *decimal.Decimal        `json:"lowest_pnl_pct,omitempty"` // Worst observed net PnL (MAE)
	PeakStep        *decimal.Decimal        `json:"peak_step,omitempty"` // Largest green up-step, in PnL percent
	Impulse         bool                    `json:"impulse,omitempty"`   // True explosive spike detected
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

	position.mu.Lock()

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

	position.mu.Unlock()

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
	position.mu.Lock()
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
	position.mu.Unlock()

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

	position.mu.Lock()
	if position.ExitTrigger == "" {
		position.recordExitTriggerLocked("policy exit match " + path)
	}
	position.mu.Unlock()

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
	position.mu.Lock()

	if position.ExitOrder == nil {
		position.mu.Unlock()
		return errnie.Err(
			errnie.NotFound, "[position.Exit] no exit order for "+position.Symbol, nil,
		)
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
		if venueFlat(err) {
			return ErrVenueFlat
		}

		position.mu.Lock()
		position.ClosedAt = nil
		position.mu.Unlock()
		return err
	}

	return nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.mu.Lock()
	defer position.mu.Unlock()

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
		// Excursions are measured from the entry baseline, even if the first
		// executable mark arrives already green or already red.
		if position.PeakPnLPct == nil {
			position.PeakPnLPct = decimal.NewFromInt64(0)
		}
		if position.LowestPnLPct == nil {
			position.LowestPnLPct = decimal.NewFromInt64(0)
		}
		if position.PnLPct.Cmp(position.PeakPnLPct) > 0 {
			position.PeakPnLPct = position.PnLPct
		}
		if position.PnLPct.Cmp(position.LowestPnLPct) < 0 {
			position.LowestPnLPct = position.PnLPct
		}
	}

	if position.ClosedAt == nil && position.LastPnL == nil {
		position.LastPnL = position.PnL
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
