package broker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	disruptor "github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
guardianCapacity is the fixed, power-of-two slot count of one PositionGuardian's
priority LMAX Disruptor. It is bounded, preallocated, and never grows.
*/
const guardianCapacity uint32 = 1024

const guardianCapacityMask = int64(guardianCapacity) - 1

// positionWireBranchCount matches the six ranked branch rows in the journal.
const positionWireBranchCount = 6

/*
Position is one lot shell owned and event-routed by Desk. Order correlation uses
each decision's client order ID, then the exchange order ID returned by REST.
*/
type Position struct {
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	instrument     *Instrument
	price          *Price
	balance        *Balance
	recorder       *audit.Recorder
	store          *PositionStore
	checkpoint     func()
	onClose        func()
	pair           kraken.InstrumentPair
	seenExecutions map[string]struct{}
	passage        *passageTracker
	Status         atomic.Pointer[types.Status] `json:"-"`
	/*
		Decision is the arbitration that opened this lot, kept verbatim.

		A position outlives the round that produced it: the planner moves on, and
		by the time anyone asks why a lot is open its originating decision is no
		longer anywhere in the current decision batch. Carrying it here is what
		lets the terminal answer that question at all, and it survives a client
		reconnect because the desk republishes its positions on connect.
	*/
	Decision         types.Decision        `json:"decision"`
	EntryOrder       *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder        *spot.AddOrderRequest `json:"exit_order"`
	EntryOrderResult *spot.AddOrderResult  `json:"entry_order_result"`
	ExitOrderResult  *spot.AddOrderResult  `json:"exit_order_result"`
	Holding          *types.Holding        `json:"holding"`

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
	recorder *audit.Recorder,
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
		recorder:       recorder,
		store:          store,
		pair:           pair,
		seenExecutions: make(map[string]struct{}),
		Decision:       decision,
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
			Stoploss:      decision.Stoploss,
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

	sequence := position.disruptor.Reserve(1)
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
		Status: string(position.status()),
		Decision: types.DecisionWire(
			position.Decision,
			positionWireBranchCount,
			true,
		),
		Holding: types.HoldingWire(position.Holding),
	}
}

/*
onTicker refreshes the mark cache for this position's holding and lets the
bound stoploss regulator judge the price a sale would actually realise.
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

	if position.Holding.Stoploss == nil {
		position.Holding.Mark = ticker.Bid
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
		return
	}

	/*
		A fill whose RebindFill failed at execution time leaves the stop in ERROR
		with no geometry (Floor/Peak/ArmAt/LockFloor all nil) and would otherwise
		sit unprotected forever, since nothing else ever calls RebindFill again.
		Retry it on every tick using the live bid: the inputs that matter
		(TickSize, fee rates, RiskDistance, Plan) live on the Stoploss itself and
		don't depend on the tick that originally failed, so a later mark that
		clears the tick-rounded positive-floor requirement recovers the position
		into a normally armed stop.
	*/
	if position.Holding.Stoploss.Status == types.ERROR {
		if ticker.Bid == nil {
			return
		}

		entryAt := time.Time{}

		if position.Holding.EntryAt != nil {
			entryAt = *position.Holding.EntryAt
		}

		if err := position.Holding.Stoploss.RebindFill(
			position.Holding.EntryPrice,
			ticker.Bid,
			entryAt,
		); err != nil {
			position.Holding.Mark = ticker.Bid
			position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
			position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
			return
		}

		position.Holding.Status = types.OPEN

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding.Stoploss))
		}
	}

	previousStatus := position.Holding.Stoploss.Status
	previousLocked := position.Holding.Stoploss.Locked
	previousFloor := position.Holding.Stoploss.Floor
	previousPeak := position.Holding.Stoploss.Peak

	// One economic mark: once the authoritative L3 book has been read
	// (BookObserved), the executable-liquidation VWAP owns Holding.Mark and the
	// stoploss geometry. A later ticker must refresh the cache (price.Update
	// above) and advance the forecast-horizon clock, but must NOT overwrite the
	// mark with best-bid or re-run the stoploss against it — otherwise a
	// mid-run ticker undoes the L3 mark ("never mind, mark is best bid Y, now
	// run the stoploss again"). Before the book has produced a coherent frame,
	// best-bid is the only available mark and seeds the position.
	mark := ticker.Bid

	if position.Holding.Stoploss.BookObserved && position.Holding.Mark != nil {
		mark = position.Holding.Mark
	}

	stoploss := position.Holding.Stoploss
	stoploss.Update(mark)
	position.Holding.Mark = mark
	position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
	position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

	if position.passage != nil {
		position.passage.observe(position, position.Holding.Mark)
	}

	triggered := stoploss.Status == types.TRIGGERED

	/*
		Exit initiation happens first and independently of persistence, audit,
		checkpoint, UI, diagnostics, model feedback, and logging. Only the
		goroutine that atomically claims the exit may submit the initial order.
	*/
	if triggered && stoploss.Status == types.TRIGGERED {
		position.initiateProtectiveExit()
	}

	changed := previousStatus != stoploss.Status ||
		previousLocked != stoploss.Locked ||
		previousFloor.Cmp(stoploss.Floor) != 0 ||
		previousPeak.Cmp(stoploss.Peak) != 0

	if changed && position.store != nil {
		if err := position.store.Save(stoploss); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"position: failed to persist stoploss transition",
				err,
			))
		}
	}
}

/*
onLevel3 derives the current executable-liquidation surface for this position's
actual SellableQty from the authoritative post-frame book and feeds it to the
bound stoploss through ObserveExecutable. It is the L3 market-state clock: the
mark, peak, floor, and any execution-regime trigger are driven by the complete
book, never by ticker.Bid, and never once per intermediate order mutation.

It only runs for positions with positive sellable inventory, so a closed or
unfilled lot performs no L3 book work.
*/
func (position *Position) onLevel3(level3 kraken.Level3Data) {
	position.evaluateExecutable(level3.Symbol, level3.Timestamp)
}

/*
evaluateExecutable derives the current executable-liquidation surface for this
position's actual SellableQty from the authoritative book and feeds it to the
bound stoploss through ObserveExecutable. It is the single evaluation path for
the L3 market-state clock, used by both live L3 frames and recovery. It only
runs for positions with positive sellable inventory, so a closed or unfilled
lot performs no L3 book work.

The mark, peak, floor, and any execution-regime trigger are driven by the
complete book, never by ticker.Bid, and never once per intermediate order
mutation.
*/
func (position *Position) evaluateExecutable(symbol string, at time.Time) {
	if position.Holding == nil {
		return
	}

	if position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		return
	}

	stoploss := position.Holding.Stoploss

	if stoploss == nil || stoploss.Status == types.TRIGGERED {
		return
	}

	surface := position.price.ExecutableSurface(
		symbol,
		position.Holding.SellableQty,
		stoploss.Floor,
		at,
	)

	previousStatus := stoploss.Status
	previousLocked := stoploss.Locked
	previousFloor := stoploss.Floor
	previousPeak := stoploss.Peak

	stoploss.ObserveExecutable(surface)

	if surface.ExecutableVWAP != nil && surface.ExecutableVWAP.Sign() > 0 {
		position.Holding.Mark = surface.ExecutableVWAP
	}

	position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
	position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

	if position.passage != nil && surface.ExecutableVWAP != nil {
		position.passage.observe(position, surface.ExecutableVWAP)
	}

	triggered := stoploss.Status == types.TRIGGERED

	if triggered {
		position.initiateProtectiveExit()
	}

	changed := previousStatus != stoploss.Status ||
		previousLocked != stoploss.Locked ||
		previousFloor.Cmp(stoploss.Floor) != 0 ||
		previousPeak.Cmp(stoploss.Peak) != 0

	if changed && position.store != nil {
		if err := position.store.Save(stoploss); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"position: failed to persist stoploss transition",
				err,
			))
		}
	}
}

/*
initiateProtectiveExit checkpoints the trigger and submits the initial exit
order through Exit, which claims the exit atomically. It is idempotent: repeated
triggering ticks before the exchange acknowledges cannot re-submit, and ordering
with persistence is preserved by submitting the exit before any checkpoint or
audit work.
*/
func (position *Position) initiateProtectiveExit() {
	if position.checkpoint != nil {
		position.checkpoint()
	}

	if _, err := position.Exit(); err != nil {
		errnie.Error(err)
	}
}

/*
MarkFeedback snapshots the live position geometry after one ticker has updated
the holding and its stop. Distances are dimensionless so the global regulator
can compare instruments without confusing quote-currency price scales.
*/
func (position *Position) MarkFeedback(at time.Time) types.MarkFeedback {
	feedback := types.MarkFeedback{}

	if position == nil || position.Holding == nil {
		return feedback
	}

	holding := position.Holding
	feedback.PositionID = position.Decision.ID
	feedback.Symbol = holding.Symbol
	feedback.At = at.UTC()

	if feedback.At.IsZero() {
		feedback.At = time.Now().UTC()
	}

	feedback.ReturnPct = holding.ReturnPct
	feedback.Exposed = holding.Qty != nil && holding.Qty.Sign() > 0

	if holding.Mark != nil {
		feedback.Mark = holding.Mark.Float64()
	}

	if holding.PnL != nil {
		feedback.PnL = holding.PnL.Float64()
	}

	stoploss := holding.Stoploss

	if stoploss == nil {
		return feedback
	}

	feedback.StopStatus = stoploss.Status
	feedback.TriggerReason = stoploss.TriggerReason
	feedback.SurgeArmed = stoploss.SurgeArmed

	if feedback.Mark > 0 && stoploss.Floor != nil {
		feedback.FloorDistance = (feedback.Mark - stoploss.Floor.Float64()) / feedback.Mark
	}

	if feedback.Mark > 0 && stoploss.Peak != nil && stoploss.Peak.Sign() > 0 {
		feedback.PeakDrawdown = math.Min(0, math.Log(feedback.Mark/stoploss.Peak.Float64()))
	}

	return feedback
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

		if status == types.CANCELED || status == types.REJECTED ||
			status == types.EXPIRED {
			position.setStatus(status)
			position.Holding.Status = status

			if position.store != nil {
				_ = position.store.SaveTrade(position)
			}

			if position.cancel != nil {
				position.cancel()
			}

			return true
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 ||
			execution.CumCost == nil || execution.CumCost.Sign() <= 0 ||
			execution.FeeUsdEquiv == nil || execution.FeeUsdEquiv.Sign() < 0 {
			continue
		}

		position.setStatus(status)
		position.Holding.Status = position.status()
		position.Holding.EntryAt = &execution.Timestamp
		position.Holding.EntryPrice = executionVWAP(execution)
		position.Holding.EntryFee = execution.FeeUsdEquiv
		position.Holding.Qty = execution.CumQty
		position.Holding.SellableQty = execution.CumQty

		// Authoritative entry economics for the audit journal: whole-order
		// realized VWAP (AvgPrice preferred), total filled quantity, and the
		// exchange's reported fee.
		position.Holding.EntryVWAP = executionVWAP(execution)
		position.Holding.EntryQty = decimal.NewFromInt64(0).Add(execution.CumQty)
		position.Holding.EntryFees = decimal.NewFromInt64(0).Add(execution.FeeUsdEquiv)

		if err := position.Holding.Stoploss.RebindFill(
			position.Holding.EntryPrice,
			position.Holding.Mark,
			execution.Timestamp,
		); err != nil {
			position.setStatus(types.ERROR)
			position.Holding.Status = types.ERROR
			position.Holding.Stoploss.Status = types.ERROR
			errnie.Error(err)
			continue
		}

		position.Holding.Stoploss.ArmClock()
		position.Holding.Stoploss.Update(position.Holding.Mark)
		position.passage = newPassageTracker(
			position,
			position.Holding.EntryPrice,
			max(1, position.Decision.ForecastHorizon),
		)
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding.Stoploss))
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

	// Separated realized economics for the audit journal.
	position.Holding.ExitVWAP = exitVWAP
	position.Holding.ExitQty = decimal.NewFromInt64(0).Add(execution.CumQty)
	position.Holding.ExitFees = decimal.NewFromInt64(0).Add(execution.FeeUsdEquiv)
	position.Holding.RealizedPnL = decimal.NewFromInt64(0).Add(position.Holding.PnL)
	position.Holding.RealizedReturn = decimal.NewFromInt64(0).Add(
		position.Holding.PnL,
	).Div(entryValue)
	position.Holding.SellableQty = decimal.NewFromInt64(0)

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
		position.Holding.Stoploss.Status = types.ERROR

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

	if position.Holding.Stoploss == nil {
		return errnie.Err(
			errnie.NotAcceptable,
			"position: regulator is required for a manual exit",
			nil,
		)
	}

	if err := position.Holding.Stoploss.TriggerManualOverride(); err != nil {
		return err
	}

	if position.store != nil {
		if err := position.store.Save(position.Holding.Stoploss); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"position: persist manual exit transition",
				err,
			))
		}
	}

	if position.checkpoint != nil {
		position.checkpoint()
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

	if position.Holding == nil || position.Holding.Stoploss == nil ||
		position.Holding.Stoploss.Status != types.TRIGGERED {
		return position, errnie.Err(
			errnie.NotAcceptable,
			"position: triggered stoploss required to submit an exit",
			nil,
		)
	}

	volume := position.Holding.Qty

	if position.Holding.SellableQty != nil && position.Holding.SellableQty.Sign() > 0 {
		volume = position.Holding.SellableQty
	}

	exitOrder := &spot.AddOrderRequest{
		ClOrdId:   position.EntryOrder.ClOrdId + "-exit",
		Type:      "sell",
		OrderType: "market",
		Volume:    volume.String(),
		Pair:      position.pair.Symbol,
	}

	result, err := position.api.AddOrder(exitOrder)

	if err != nil {
		// No order reached the exchange, so the claim must release or a
		// transient AddOrder failure would permanently strand a triggered
		// stop with no exit ever in flight and no path to retry it.
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
