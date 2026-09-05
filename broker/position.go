package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
guardianCapacity is the fixed, power-of-two slot count of one PositionGuardian's
priority LMAX Disruptor. It is bounded, preallocated, and never grows.
*/
const guardianCapacity uint32 = 8192

const guardianCapacityMask = int64(guardianCapacity) - 1

/*
Position is one lot shell owned and event-routed by Desk. Entry correlation uses
the decision UUID; each exit derives its own stable Kraken-valid UUID so both
open orders remain independently identifiable through retries and recovery.
*/
type Position struct {
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	instrument     *Instrument
	price          *Price
	balance        *Balance
	store          *PositionStore
	onClose        func()
	recordFill     func(kind string, execution kraken.ExecutionData)
	pair           kraken.InstrumentPair
	seenExecutions map[string]struct{}
	Status         atomic.Pointer[types.Status] `json:"-"`
	/*
		Decision is the arbitration that opened this lot, kept verbatim.

		A position outlives the round that produced it: the planner moves on, and
		by the time anyone asks why a lot is open its originating decision is no
		longer anywhere in the current decision batch. Carrying it here is what
		lets the terminal answer that question at all, and it survives a client
		reconnect because the desk republishes its positions on connect.
	*/
	Decision types.Decision `json:"decision"`
	// decisionWire is constructed once because Decision is immutable. Position
	// telemetry is emitted on every public ticker, so rebuilding and sorting the
	// same decision evidence on that hot path would allocate needlessly.
	decisionWire     *wire.DecisionT
	EntryOrder       *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder        *spot.AddOrderRequest `json:"exit_order"`
	EntryOrderResult *spot.AddOrderResult  `json:"entry_order_result"`
	ExitOrderResult  *spot.AddOrderResult  `json:"exit_order_result"`

	/*
		ReduceOrder is a sell that shrinks the lot without closing it. It is
		tracked apart from ExitOrder because a reduction is not an exit: the
		position stays open, keeps its entry basis, and may be reduced again.
		reduceSequence makes each reduction's client order id distinct, so a
		retry of one cannot be mistaken for another.
	*/
	ReduceOrder    *spot.AddOrderRequest `json:"reduce_order,omitempty"`
	reduceSequence uint64

	Holding *types.Holding `json:"holding"`

	/*
		Priority transport: one dedicated LMAX Disruptor plus its contiguous
		slot storage and a single guardian consumer handler. The guardian
		bypasses Workspace analytics scheduling entirely.
	*/
	disruptor    disruptor.Disruptor
	guardian     *guardianHandler
	guardianSlot [guardianCapacity]guardianEvent

	// exitSequence protects the OPEN → EXIT_REQUESTED → EXITING → CLOSED
	// transition so that only one goroutine may claim and submit the exit.
	exitMu    sync.Mutex
	exitClaim atomic.Bool

	// DegradedRecovery is true when the position was reconstructed at boot
	// without its stored row, so its basis was rebuilt from trade history
	// rather than read back. It is an explicit, inspectable marker that the
	// recovered basis is a reconstruction.
	DegradedRecovery bool `json:"degraded_recovery,omitempty"`

	// guardianWatermark counts the priority events the guardian has fully
	// processed. It is the guardian's causal processing cursor: an observer can
	// read it atomically and, once it has advanced past an event's ordinal, read
	// the position's derived state with a happens-before edge to that event.
	guardianWatermark atomic.Uint64
}

/*
guardianEvent is the payload a guardian priority ring slot carries: one typed
market or control message. Sequence is its LMAX publication sequence.
*/
type guardianEvent struct {
	sequence int64
	value    any
}

/*
guardianHandler is the single consumer handler of a PositionGuardian. It runs on
the dedicated guardian goroutine driven by disruptor.Listen and never acquires
an analytical semaphore.
*/
type guardianHandler struct {
	position *Position
}

func (handler *guardianHandler) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		event := &handler.position.guardianSlot[sequence&guardianCapacityMask]
		handler.position.handleGuardian(event.value)
	}
}

/*
NewPosition constructs one desk-owned lot shell.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	store *PositionStore,
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	position := &Position{
		ctx:            ctx,
		cancel:         cancel,
		api:            api,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		store:          store,
		pair:           pair,
		seenExecutions: make(map[string]struct{}),
		Decision:       decision,
		decisionWire:   types.DecisionWire(&decision),
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "buy",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		Holding: &types.Holding{
			Symbol:        pair.Symbol,
			Qty:           decimal.NewFromInt64(0),
			SellableQty:   decimal.NewFromInt64(0),
			Asset:         pair.Base,
			EntryPrice:    decision.EntryPrice,
			Mark:          decision.Mark,
			IsOpportunity: decision.Opportunity,
		},
	}
	guardian := &guardianHandler{position: position}
	disruptorInstance, err := disruptor.New(
		disruptor.Options.BufferCapacity(guardianCapacity),
		disruptor.Options.NewHandlerGroup(guardian),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"position: failed to construct guardian disruptor for "+pair.Symbol,
			err,
		))
	} else {
		position.guardian = guardian
		position.disruptor = disruptorInstance
		go disruptorInstance.Listen()
	}

	position.setStatus(types.INITIALIZING)
	return position
}

/*
publishGuardian routes one priority message into this PositionGuardian's
dedicated LMAX Disruptor using the library's native Reserve. Priority events are
never silently lost: if the guardian has fallen catastrophically behind, the
failure is surfaced to the caller, who must invoke the risk failure path rather
than queue the mark elsewhere.
*/
func (position *Position) publishGuardian(value any) error {
	if position == nil || position.disruptor == nil {
		return errnie.Err(
			errnie.NotFound,
			"position: guardian transport unavailable",
			nil,
		)
	}

	var sequence int64 = -1

	for range 4 {
		sequence = position.disruptor.Reserve(1)

		if sequence >= 0 {
			break
		}

		runtime.Gosched()
	}

	if sequence < 0 {
		return errnie.Err(
			errnie.NotAcceptable,
			"position: guardian priority ring saturated",
			nil,
		)
	}

	position.guardianSlot[sequence&guardianCapacityMask] = guardianEvent{
		sequence: sequence,
		value:    value,
	}
	position.disruptor.Commit(sequence, sequence)

	return nil
}

/*
handleGuardian dispatches one priority event on the dedicated guardian goroutine.
It is the sole consumer of the guardian ring and runs without any analytical
semaphore, so position protection can never be delayed by bulk analytics.
*/
func (position *Position) handleGuardian(value any) {
	defer position.guardianWatermark.Add(1)

	switch payload := value.(type) {
	case kraken.TickerData:
		position.onTicker(payload)
	case kraken.ExecutionData:
		position.onExecution(kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data:    []kraken.ExecutionData{payload},
		})
	case kraken.Level3Data:
		position.onLevel3(payload)
	case string: // e.g. "manual_exit"
		if payload == "manual_exit" {
			if err := position.executeManualExit(); err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"position: manual exit did not execute for "+position.pair.Symbol,
					err,
				))
			}
		}
	}
}

func (position *Position) status() types.Status {
	status := position.Status.Load()

	if status == nil {
		return types.UNKNOWN
	}

	return *status
}

func (position *Position) setStatus(status types.Status) {
	position.Status.Store(&status)
}

func (position *Position) MarshalJSON() ([]byte, error) {
	type positionJSON Position

	return json.Marshal(struct {
		Status types.Status `json:"status"`
		*positionJSON
	}{
		Status:       position.status(),
		positionJSON: (*positionJSON)(position)})
}

func (position *Position) Wire() *wire.PositionT {
	return &wire.PositionT{
		Status:   string(position.status()),
		Decision: position.decisionWire,
		Holding:  types.HoldingWire(position.Holding),
	}
}

/*
onTicker refreshes this position's mark and realized-return view from the live
quote. It decides nothing: the agent that opened the lot re-evaluates it on
every book update and commands its own exit, so there is no protective
geometry here to advance and no path by which a ticker closes a position.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	if position.Holding == nil {
		return
	}

	position.price.Update(&ticker)

	if position.Holding.Qty == nil || position.Holding.Qty.Sign() <= 0 {
		if ticker.Bid != nil {
			position.Holding.Mark = ticker.Bid
		}

		return
	}

	// Executable L3 state owns the mark once it can price the whole lot,
	// because that is what a sale would actually realise. Best bid seeds the
	// position until then, and a later ticker must not overwrite a
	// liquidation mark with a top-of-book one.
	if ticker.Bid != nil && position.Holding.Mark == nil {
		position.Holding.Mark = ticker.Bid
	}

	position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
	position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
}

/*
onLevel3 derives the current executable-liquidation surface for this position's
actual SellableQty from the continuously-resident execution state and feeds it
to the bound stoploss through ObserveExecutable. It is the L3 market-state
clock: the mark, peak, floor, and any execution-regime trigger are driven by
the complete executable state, never by ticker.Bid, and never once per
intermediate order mutation.

It only runs for positions with positive sellable inventory, so a closed or
unfilled lot performs no L3 execution work.
*/
func (position *Position) onLevel3(level3 kraken.Level3Data) {
	if position.Holding == nil ||
		position.Holding.SellableQty == nil ||
		position.Holding.SellableQty.Sign() <= 0 {
		return
	}

	position.evaluateExecutable(level3.Symbol, level3.Timestamp)
}

/*
evaluateExecutable marks this position at the price its actual sellable
inventory would realise against the resident executable state, rather than at
top of book. It is the single evaluation path for the L3 market-state clock,
used by both live frames and recovery.

This is a valuation, not a decision. A partial-depth surface prices a sale that
cannot complete, so it is not accepted as a mark and the previous one stands.
*/
func (position *Position) evaluateExecutable(symbol string, at time.Time) {
	if position.Holding == nil || position.price == nil {
		return
	}

	if position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		return
	}

	fee := position.price.FeeIfAvailable(symbol)

	if fee == nil {
		return
	}

	surface := position.price.Surface(symbol, position.Holding.SellableQty, nil, fee, at)

	if surface == nil || !surface.BookComplete || !surface.FullyExecutable ||
		surface.ExecutableVWAP == nil || surface.ExecutableVWAP.Sign() <= 0 {
		return
	}

	position.Holding.Mark = surface.ExecutableVWAP
	position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
	position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
}

func (position *Position) onExecution(message kraken.Execution) bool {
	if position.status() == types.CLOSED {
		return true
	}

	for _, execution := range message.Data {
		if execution.ExecID != "" {
			if position.seenExecutions == nil {
				position.seenExecutions = make(map[string]struct{})
			}

			if _, seen := position.seenExecutions[execution.ExecID]; seen {
				continue
			}

			position.seenExecutions[execution.ExecID] = struct{}{}
		}

		if position.ExitOrder != nil &&
			execution.ClientOrderID == position.ExitOrder.ClOrdId {
			status, err := types.StatusFromMarket(execution.OrderStatus)

			if err != nil {
				position.setStatus(types.ERROR)
				position.Holding.Status = types.ERROR
				errnie.Error(err)
				return false
			}

			if status == types.FILLED {
				if err = position.closeFill(execution); err != nil {
					position.setStatus(types.ERROR)
					position.Holding.Status = types.ERROR
					errnie.Error(err)
					return false
				}

				return true
			}

			if status != types.CANCELED && status != types.REJECTED &&
				status != types.EXPIRED {
				continue
			}

			if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
				position.setStatus(types.ERROR)
				position.Holding.Status = types.ERROR
				errnie.Error(errnie.Err(
					errnie.Conflict,
					"position: terminal exit retained partial inventory",
					nil,
				))
				return false
			}

			position.ExitOrder = nil
			position.ExitOrderResult = nil
			position.exitClaim.Store(false)
			position.setStatus(types.OPEN)
			position.Holding.Status = types.OPEN
			continue
		}

		if position.EntryOrder == nil ||
			execution.ClientOrderID != position.EntryOrder.ClOrdId {
			continue
		}

		status, err := types.StatusFromMarket(execution.OrderStatus)

		if err != nil {
			position.setStatus(types.ERROR)
			position.Holding.Status = types.ERROR
			errnie.Error(err)
			return false
		}

		terminal := status == types.CANCELED || status == types.REJECTED ||
			status == types.EXPIRED
		filled := execution.CumQty != nil && execution.CumQty.Sign() > 0 &&
			execution.CumCost != nil && execution.CumCost.Sign() > 0 &&
			execution.FeeUsdEquiv != nil && execution.FeeUsdEquiv.Sign() >= 0
		hasInventory := position.Holding.Qty != nil && position.Holding.Qty.Sign() > 0

		if terminal && !filled && hasInventory {
			position.setStatus(types.OPEN)
			position.Holding.Status = types.OPEN

			if position.store != nil {
				errnie.Error(position.store.SaveTrade(position))
			}

			continue
		}

		if terminal && !filled {
			position.setStatus(status)
			position.Holding.Status = status

			if position.store != nil {
				errnie.Error(position.store.SaveTrade(position))
			}

			if position.cancel != nil {
				position.cancel()
			}

			return true
		}

		if !filled {
			continue
		}

		if terminal {
			status = types.OPEN
		}

		position.setStatus(status)
		position.Holding.Status = position.status()
		position.Holding.EntryAt = &execution.Timestamp
		position.Holding.EntryPrice = executionVWAP(execution)
		position.Holding.EntryFee = execution.FeeUsdEquiv
		position.Holding.Qty = execution.CumQty
		position.Holding.SellableQty = execution.CumQty

		// Authoritative entry economics for the persisted trade record: whole-order
		// realized VWAP (AvgPrice preferred), total filled quantity, and the
		// exchange's reported fee.
		position.Holding.EntryVWAP = executionVWAP(execution)
		position.Holding.EntryQty = decimal.NewFromInt64(0).Add(execution.CumQty)
		position.Holding.EntryFees = decimal.NewFromInt64(0).Add(execution.FeeUsdEquiv)

		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

		if position.recordFill != nil {
			position.recordFill("entry_fill", execution)
		}

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding))
			errnie.Error(position.store.SaveTrade(position))
		}
	}

	return false
}

/*
executionVWAP resolves the authoritative whole-order realized VWAP for a
Kraken ExecutionData. It prefers the explicit AvgPrice field (Kraken's own
whole-order average) and falls back to the cumulative equivalent
CumCost/CumQty only when AvgPrice is absent. It never uses the individual
LastPrice, which is a single fill and not the whole-order economics a closed
position's exit must be marked by.
*/
func executionVWAP(execution kraken.ExecutionData) *decimal.Decimal {
	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		return decimal.NewFromInt64(0).Add(execution.AvgPrice)
	}

	if execution.CumCost == nil || execution.CumCost.Sign() <= 0 ||
		execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
		return nil
	}

	return decimal.NewFromInt64(0).Add(execution.CumCost).Div(execution.CumQty)
}

/*
closeFill records the exchange's realized exit economics before the lot leaves
the desk and publishes the terminal state so retained UI positions can remove
it by identity.

The exit price is the whole-order realized VWAP (AvgPrice preferred, else
CumCost/CumQty), never the final individual fill's LastPrice. Entry and exit
fees are the exchange's authoritative FeeUsdEquiv totals. Realized PnL and
RealizedReturn derive from those same figures, so the journal reconciles
exit proceeds − entry basis − entry fees − exit fees exactly.
*/
func (position *Position) closeFill(execution kraken.ExecutionData) error {
	if position.status() == types.CLOSED {
		return nil
	}

	if position.Holding == nil || position.Holding.Qty == nil ||
		position.Holding.Qty.Sign() <= 0 || position.Holding.EntryPrice == nil ||
		position.Holding.EntryPrice.Sign() <= 0 || position.Holding.EntryFee == nil ||
		position.Holding.EntryFee.Sign() < 0 || execution.CumQty == nil ||
		execution.CumQty.Sign() <= 0 || execution.CumCost == nil ||
		execution.CumCost.Sign() <= 0 || execution.FeeUsdEquiv == nil ||
		execution.FeeUsdEquiv.Sign() < 0 || execution.Timestamp.IsZero() {
		return errnie.Err(
			errnie.Validation,
			"position: complete exit fill economics required",
			nil,
		)
	}

	sellable := position.Holding.SellableQty

	if sellable == nil || sellable.Sign() <= 0 {
		sellable = position.Holding.Qty
	}

	if execution.CumQty.Cmp(sellable) != 0 {
		return errnie.Err(
			errnie.Conflict,
			"position: filled exit quantity does not match sellable inventory",
			nil,
		)
	}

	entryGross := decimal.NewFromInt64(0).Add(
		position.Holding.EntryPrice,
	).Mul(position.Holding.Qty)
	entryValue := entryGross.Add(position.Holding.EntryFee)
	exitVWAP := executionVWAP(execution)

	if exitVWAP == nil {
		return errnie.Err(
			errnie.Validation,
			"position: exit execution has no resolvable realized VWAP",
			nil,
		)
	}

	// Authoritative exit proceeds are CumCost (gross): realized PnL is
	// proceeds − entry basis − entry fees − exit fees, all exchange-reported.
	exitValue := decimal.NewFromInt64(0).Add(execution.CumCost).Sub(
		execution.FeeUsdEquiv,
	)
	position.Holding.ExitAt = &execution.Timestamp
	position.Holding.ExitPrice = exitVWAP
	position.Holding.ExitFee = execution.FeeUsdEquiv
	position.Holding.PnL = exitValue.Sub(entryValue)
	position.Holding.ReturnPct = decimal.NewFromInt64(0).Add(
		position.Holding.PnL,
	).Div(entryValue).Mul(decimal.NewFromInt64(100)).Float64()

	// Separated realized economics for the persisted trade record.
	position.Holding.ExitVWAP = exitVWAP
	position.Holding.ExitQty = decimal.NewFromInt64(0).Add(execution.CumQty)
	position.Holding.ExitFees = decimal.NewFromInt64(0).Add(execution.FeeUsdEquiv)
	position.Holding.RealizedPnL = decimal.NewFromInt64(0).Add(position.Holding.PnL)
	position.Holding.RealizedReturn = decimal.NewFromInt64(0).Add(
		position.Holding.PnL,
	).Div(entryValue)
	position.Holding.SellableQty = decimal.NewFromInt64(0)

	if position.recordFill != nil {
		position.recordFill("exit_fill", execution)
	}

	if position.store != nil {
		if err := position.store.Delete(position.pair.Symbol); err != nil {
			return err
		}
	}

	if err := position.Close(); err != nil {
		return err
	}

	if position.store != nil {
		errnie.Error(position.store.SaveTrade(position))
	}

	return nil
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.setStatus(types.ERROR)
		position.Holding.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.EntryOrderResult = &result

	if position.status() != types.OPEN && position.status() != types.CLOSED {
		position.setStatus(types.PENDING)
		position.Holding.Status = types.PENDING
	}

	return position, nil
}

/*
Reduce sells part of an open lot without closing it. It is how a policy that
sizes its own exposure gives some of it back: the position stays open, keeps
its entry basis, and can be reduced again or exited later.

A reduction never claims the exit. Claiming it would latch the lot as closing
and block the real exit that follows, and the two orders answer different
questions — "hold less of this" is not "stop holding this".
*/
func (position *Position) Reduce(volume *decimal.Decimal) error {
	position.exitMu.Lock()
	defer position.exitMu.Unlock()

	if position.status() == types.CLOSED || position.Holding == nil {
		return errnie.Err(
			errnie.NotAcceptable,
			"position: an open holding is required to reduce",
			nil,
		)
	}

	// A lot already selling everything it has cannot also sell part of it.
	if position.ExitOrder != nil || position.ReduceOrder != nil {
		return nil
	}

	sellable := position.Holding.SellableQty

	if sellable == nil || sellable.Sign() <= 0 {
		return errnie.Err(
			errnie.NotAcceptable,
			"position: no sellable inventory to reduce for "+position.pair.Symbol,
			nil,
		)
	}

	if volume == nil || volume.Sign() <= 0 || volume.Cmp(sellable) >= 0 {
		return errnie.Err(
			errnie.Validation,
			"position: a reduction must be positive and smaller than the sellable lot",
			nil,
		)
	}

	position.reduceSequence++
	order := &spot.AddOrderRequest{
		ClOrdId: uuid.NewSHA1(
			uuid.NameSpaceOID,
			fmt.Appendf(nil, "symm:reduce:%s:%d", position.EntryOrder.ClOrdId, position.reduceSequence),
		).String(),
		Type:      "sell",
		OrderType: "market",
		Volume:    volume.String(),
		Pair:      position.pair.Symbol,
	}

	if _, err := position.api.AddOrder(order); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market reduce order",
			err,
		))
	}

	position.ReduceOrder = order

	return nil
}

/*
applyReduceFill settles one reduction against the lot it shrank. Sold inventory
leaves Qty and SellableQty; the entry basis is untouched, because reducing does
not change what the remaining inventory cost. A terminal status with nothing
filled simply releases the order so the lot can be reduced again.
*/
func (position *Position) applyReduceFill(execution kraken.ExecutionData) {
	status, err := types.StatusFromMarket(execution.OrderStatus)

	if err != nil {
		errnie.Error(err)
		return
	}

	if status != types.FILLED && status != types.CANCELED &&
		status != types.REJECTED && status != types.EXPIRED {
		return
	}

	if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
		sold := execution.CumQty

		if position.Holding.Qty != nil {
			position.Holding.Qty = decimal.NewFromInt64(0).Add(position.Holding.Qty).Sub(sold)
		}

		if position.Holding.SellableQty != nil {
			position.Holding.SellableQty = decimal.NewFromInt64(0).Add(position.Holding.SellableQty).Sub(sold)
		}

		if position.recordFill != nil {
			position.recordFill("reduce_fill", execution)
		}

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding))
		}
	}

	position.ReduceOrder = nil
}

/*
ManualExit is the operator override for one filled lot. It pushes a command to the
guardian ring to guarantee order with market events.
*/
func (position *Position) ManualExit() error {
	return position.publishGuardian("manual_exit")
}

func (position *Position) executeManualExit() error {
	if position == nil || position.Holding == nil ||
		position.Holding.Qty == nil || position.Holding.Qty.Sign() <= 0 {
		return errnie.Err(
			errnie.NotAcceptable,
			"position: filled inventory is required for a manual exit",
			nil,
		)
	}

	if position.status() == types.CLOSED {
		return errnie.Err(
			errnie.NotFound,
			"position: lot is already closed",
			nil,
		)
	}

	if position.ExitOrder != nil {
		return nil
	}

	_, err := position.Exit()
	return err
}

/*
Exit is the single sell-order boundary for an open lot.
*/
func (position *Position) Exit() (*Position, error) {
	if position.status() == types.CLOSED {
		return position, nil
	}

	position.exitMu.Lock()
	defer position.exitMu.Unlock()

	if position.exitClaim.Swap(true) {
		return position, nil
	}

	if position.ExitOrder != nil {
		return position, nil
	}

	// The exit is commanded, not triggered. Whoever opened the lot decides
	// when it closes, and the only precondition is that there is inventory to
	// sell and no sell order already in flight.
	if position.Holding == nil {
		return position, errnie.Err(
			errnie.NotAcceptable,
			"position: an open holding is required to submit an exit",
			nil,
		)
	}

	volume := position.Holding.Qty

	if position.Holding.SellableQty != nil && position.Holding.SellableQty.Sign() > 0 {
		volume = position.Holding.SellableQty
	}

	exitOrder := &spot.AddOrderRequest{
		ClOrdId: uuid.NewSHA1(
			uuid.NameSpaceOID,
			[]byte("symm:exit:"+position.EntryOrder.ClOrdId),
		).String(),
		Type:      "sell",
		OrderType: "market",
		Volume:    volume.String(),
		Pair:      position.pair.Symbol,
	}

	result, err := position.api.AddOrder(exitOrder)

	if err != nil {
		// Release the local claim so a transient failure can retry. The retry
		// derives the same client order UUID, so losing a successful venue
		// response cannot create a distinct second sell order.
		position.exitClaim.Store(false)

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market exit order",
			err,
		))
	}

	position.ExitOrder = exitOrder
	position.ExitOrderResult = &result
	position.setStatus(types.PENDING)
	position.Holding.Status = types.PENDING

	return position, nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	if position.status() == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.disruptor != nil {
		_ = position.disruptor.Close()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.setStatus(types.CLOSED)

	if position.onClose != nil {
		position.onClose()
	}

	return errnie.Error(err)
}
