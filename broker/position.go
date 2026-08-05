package broker

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Position is one lot shell owned and event-routed by Desk. Order correlation uses
each decision's client order ID, then the exchange order ID returned by REST.
*/
type Position struct {
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	ui             chan []byte
	instrument     *Instrument
	price          *Price
	balance        *Balance
	recorder       *audit.Recorder
	pair           kraken.InstrumentPair
	seenExecutions map[string]struct{}
	entryCumQty    *decimal.Decimal
	exitCumQty     *decimal.Decimal
	entryTerminal  bool
	entryCancel    bool
	exitOrderID    string
	exitAttempt    uint64
	// exiting records that a sell has been handed to the venue for this lot.
	// One Status field cannot describe both order legs at once, and the two
	// disagree constantly: an entry fill arriving after an exit was submitted
	// would otherwise set the position back to OPEN. This is the sell leg's
	// own state, and it only ever goes one way.
	exiting bool
	// stopOrderID is the client order identifier the regulator's own exit is
	// submitted under, minted once so a retry cannot become a second sell.
	// It is distinct from the entry's identifier, which the exit leg used to
	// reuse — leaving both legs of one lot correlating to the same ID.
	stopOrderID string
	/*
		evidence is the latest reading the strategy has published about this
		symbol. It is an atomic handoff because the strategy runs on the
		analyzer's goroutine while everything else here runs on the desk's, and
		the alternative — letting the strategy reach in and set stop geometry
		directly — puts two goroutines on the same regulator.

		The desk-side reader takes whatever is currently latched. A tick that
		arrives between two thesis cuts uses the previous cut's reading, which
		is the correct behaviour: toxicity does not stop being true because no
		new measurement landed this millisecond.
	*/
	evidence atomic.Pointer[types.StopEvidence]
	/*
		snapshot is the mirror of evidence in the other direction: the desk
		publishes the regulator's geometry after every observation so the
		strategy can price a position against the boundaries it is actually
		being defended by, without reading the live regulator across goroutines.
	*/
	snapshot         atomic.Pointer[types.StopSnapshot]
	ID               string                `json:"id"`
	Status           types.Status          `json:"status"`
	EntryOrder       *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder        *spot.AddOrderRequest `json:"exit_order"`
	EntryOrderResult *spot.AddOrderResult  `json:"entry_order_result"`
	ExitOrderResult  *spot.AddOrderResult  `json:"exit_order_result"`
	Holding          *types.Holding        `json:"holding"`
}

/*
NewPosition constructs one desk-owned lot shell.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	recorder *audit.Recorder,
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	holding := types.NewHolding(
		ctx,
		pair.Symbol,
		decision,
	)

	position := &Position{
		ctx:            ctx,
		cancel:         cancel,
		Status:         types.INITIALIZING,
		api:            api,
		ui:             ui,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		recorder:       recorder,
		pair:           pair,
		seenExecutions: map[string]struct{}{},
		entryCumQty:    decimal.NewFromInt64(0),
		exitCumQty:     decimal.NewFromInt64(0),
		ID:             decision.ID,
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "buy",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		ExitOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "sell",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		Holding: holding,
	}

	position.Holding.Qty = decimal.NewFromInt64(0)
	position.Holding.SellableQty = decimal.NewFromInt64(0)
	position.publishSnapshot()
	position.Publish()

	return position
}

/*
Publish the position to the UI, which will automatically marshal the Holding
and its Stoploss into the JSON payload. For clarity, the balance is kept out
of this, as there must be a way to get that more accurate to reality, where
the exchange publishes the wallet state at the sensible moments. The paper
trading implementation we use is based on the kraken-cli, where under normal
use you would also not be manually managing the balances.
*/
func (position *Position) Publish() {
	out := datura.NewMap()
	out["positions"] = []*Position{position.Snapshot()}
	utils.Publish(position.ui, out)
}

/*
Snapshot copies the externally visible position state while the desk owns the
live lot. Decimal values are immutable, and the mutable slices and order shells
are copied so later desk events cannot change a published/read snapshot.
*/
func (position *Position) Snapshot() *Position {
	if position == nil {
		return nil
	}

	position.mu.RLock()
	defer position.mu.RUnlock()

	snapshot := &Position{
		ID:     position.ID,
		Status: position.Status,
	}

	if position.EntryOrder != nil {
		entryOrder := *position.EntryOrder
		snapshot.EntryOrder = &entryOrder
	}

	if position.ExitOrder != nil {
		exitOrder := *position.ExitOrder
		snapshot.ExitOrder = &exitOrder
	}

	if position.EntryOrderResult != nil {
		entryResult := *position.EntryOrderResult
		entryResult.ID = slices.Clone(position.EntryOrderResult.ID)
		snapshot.EntryOrderResult = &entryResult
	}

	if position.ExitOrderResult != nil {
		exitResult := *position.ExitOrderResult
		exitResult.ID = slices.Clone(position.ExitOrderResult.ID)
		snapshot.ExitOrderResult = &exitResult
	}

	if position.Holding != nil {
		holding := *position.Holding

		if position.Holding.Stoploss != nil {
			stoploss := *position.Holding.Stoploss
			stoploss.Transitions = slices.Clone(position.Holding.Stoploss.Transitions)
			holding.Stoploss = &stoploss
		}

		snapshot.Holding = &holding
	}

	if latched := position.snapshot.Load(); latched != nil {
		stopSnapshot := *latched
		snapshot.snapshot.Store(&stopSnapshot)
	}

	return snapshot
}

/*
ApplyEvidence latches the strategy's latest reading of this symbol.

It only stores. Nothing is evaluated on the caller's goroutine, so a thesis cut
can never race the desk into the regulator, and the reading is applied on the
next tick alongside the book it belongs with.
*/
func (position *Position) ApplyEvidence(evidence types.StopEvidence) {
	if position == nil || !evidence.Present {
		return
	}

	position.evidence.Store(&evidence)
}

/*
observation assembles what the regulator judges: the price this lot's own size
could actually be sold at, plus whatever the strategy last said about whether
that price is real.
*/
func (position *Position) observation() types.StopEvidence {
	evidence := types.StopEvidence{Symbol: position.pair.Symbol}

	if latched := position.evidence.Load(); latched != nil {
		evidence = *latched
	}

	quantity := position.Holding.SellableQty

	if quantity == nil || quantity.Sign() <= 0 {
		quantity = position.Holding.Qty
	}

	evidence.ExecutableMark, evidence.DepthLimited = position.price.ExecutableMark(
		position.pair, quantity,
	)
	evidence.Present = evidence.ExecutableMark != nil

	if tick := position.price.Tick(position.pair.Symbol); tick != nil {
		evidence.SellCapacity = decimal.NewFromFloat64(tick.BidQty)
	}

	/*
		Spread and impact are left exactly as the strategy stated them. They are
		a matched pair — the impact estimate is a fraction of the spread it was
		measured against — and overwriting one of them here with a fresher tick
		would leave the regulator re-deriving its noise band from two readings
		of two different moments.
	*/
	return evidence
}

/*
publishSnapshot latches the regulator's current geometry for readers outside the
desk goroutine.
*/
func (position *Position) publishSnapshot() {
	if position == nil || position.Holding == nil {
		return
	}

	snapshot := position.Holding.Stoploss.Snapshot()
	position.snapshot.Store(&snapshot)
}

/*
StopSnapshot reports the regulator's geometry as of the last observation.

An absent snapshot means no tick has reached this lot yet, which is a real state
and not an error: a position submitted a moment ago has a regulator whose
boundaries exist but which has judged nothing.
*/
func (position *Position) StopSnapshot() types.StopSnapshot {
	if position == nil {
		return types.StopSnapshot{}
	}

	if latched := position.snapshot.Load(); latched != nil {
		return *latched
	}

	return types.StopSnapshot{}
}

/*
auditStops writes every geometry change the regulator has made since the last
call.

The rows are what makes the stop answerable after the fact. A position that
exits leaves a decision row saying a sell went out, and nothing about which
boundary sent it or where the other boundaries stood at that moment. Each row
here carries both floors, the profit line, the peak and the mark, so a later
pass can label the one question the calibrated model will need answered: from
this state, did the lot reach its protected profit before its hard loss.
*/
func (position *Position) auditStops() {
	if position.recorder == nil || position.Holding == nil ||
		position.Holding.Stoploss == nil {
		return
	}

	stoploss := position.Holding.Stoploss

	for _, transition := range stoploss.DrainTransitions() {
		event := audit.ExecutionLifecycle{
			PositionID: position.ID,
			Symbol:     position.pair.Symbol,
			Kind:       audit.ExecutionStopTransition,
			Transition: transition,
		}

		if transition.Status == types.TRIGGERED {
			event.StopOrderID = position.stopOrderID
			event.ExitAttempt = position.exitAttempt
		}

		errnie.Error(audit.Record(position.recorder, event))
	}
}

/*
onTicker refreshes the mark cache for this position's holding and lets the
bound stoploss regulator judge the price a sale would actually realise.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	position.mu.Lock()

	if position.Holding != nil {
		if err := position.Holding.Update(ticker); err != nil {
			errnie.Error(err)
		}

		// A lot adopted from the wallet was constructed before anything had
		// priced its symbol, so this is the first moment a boundary can be
		// drawn for it. Adopt is a no-op once the regulator has geometry.
		if position.Holding.Stoploss != nil {
			position.Holding.Stoploss.Adopt(position.price.RiskPlan(position.pair))
		}

		position.Holding.Observe(position.observation())
		position.publishSnapshot()
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
	}

	if !isTerminal(position.Status) && !position.exiting && position.Holding != nil &&
		position.Holding.SellableQty != nil &&
		position.Holding.SellableQty.Sign() > 0 &&
		position.Holding.Stoploss != nil &&
		position.Holding.Stoploss.Status == types.TRIGGERED {
		position.exitOnStop()
	}

	position.auditStops()
	position.mu.Unlock()

	position.Publish()
}

/*
exitOnStop submits the regulator's exit under an identifier minted for it.

The identifier is generated once and kept. The exit leg used to reuse the
entry's, which meant both legs of one lot correlated to the same client order ID
and an execution row could not say which order it belonged to.

A submission failure is recorded rather than swallowed. The regulator stays
triggered, so the next tick tries again — which is the behaviour that matters
when a stop has fired and the venue did not answer — but the failure is on the
record instead of vanishing into an ignored return value.
*/
func (position *Position) exitOnStop() {
	if !position.entryTerminal && !position.entryCancel {
		if err := position.cancelEntry(); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"position: could not cancel the unfilled entry remainder for "+position.pair.Symbol,
				err,
			))
		}
	}

	if position.stopOrderID == "" {
		position.stopOrderID = uuid.NewString()
		position.exitAttempt++
	}

	if err := position.submitStopExit(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"position: stop exit for "+position.pair.Symbol+" was not accepted",
			err,
		))
	}
}

func (position *Position) cancelEntry() error {
	request := &spot.CancelOrderRequest{ClOrdID: position.EntryOrder.ClOrdId}

	if position.EntryOrderResult != nil && len(position.EntryOrderResult.ID) > 0 {
		request.TxID = position.EntryOrderResult.ID[0]
		request.ClOrdID = ""
	}

	result, err := position.api.CancelOrder(request)

	if err != nil {
		return errnie.Error(err)
	}

	if result.Count <= 0 && !result.Pending {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position: entry order was not found for cancellation",
			nil,
		))
	}

	position.entryCancel = true
	return nil
}

func (position *Position) adoptExit(orderID string, order spot.Order) {
	position.stopOrderID = order.ClOrdID

	if position.stopOrderID == "" {
		position.stopOrderID = orderID
	}

	position.exitOrderID = orderID
	position.ExitOrder.ClOrdId = order.ClOrdID
	position.ExitOrderResult = &spot.AddOrderResult{
		OrderPlacementSingle: spot.OrderPlacementSingle{ID: []string{orderID}},
	}
	position.exitCumQty = decimal.NewFromInt64(0)

	if order.VolumeExecuted != nil {
		position.exitCumQty = order.VolumeExecuted
	}

	position.exitAttempt = 1
	position.exiting = true
	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
	position.Holding.Stoploss.BindRecoveredExit()
}

func (position *Position) onExecution(execution *kraken.Execution) {
	if execution == nil {
		return
	}

	publish := false
	position.mu.Lock()

	for _, row := range execution.Data {
		if position.matchesEntry(row) {
			position.onEntryExecution(row)
			publish = true
			continue
		}

		if position.matchesExit(row) {
			position.onExitExecution(row)
			publish = true
		}
	}

	position.mu.Unlock()

	if publish {
		position.Publish()
	}
}

func (position *Position) matchesEntry(row kraken.ExecutionData) bool {
	if row.Symbol != position.pair.Symbol || row.Side != "buy" {
		return false
	}

	if row.ClientOrderID == position.EntryOrder.ClOrdId {
		return true
	}

	return position.EntryOrderResult != nil && len(position.EntryOrderResult.ID) > 0 &&
		row.OrderID == position.EntryOrderResult.ID[0]
}

func (position *Position) matchesExit(row kraken.ExecutionData) bool {
	if row.Symbol != position.pair.Symbol || row.Side != "sell" {
		return false
	}

	return row.ClientOrderID == position.stopOrderID ||
		(position.exitOrderID != "" && row.OrderID == position.exitOrderID)
}

func (position *Position) unseenTrade(row kraken.ExecutionData) bool {
	if row.ExecType != "trade" || row.CumQty == nil || row.CumQty.Sign() <= 0 {
		return false
	}

	if row.ExecID == "" {
		return true
	}

	if _, seen := position.seenExecutions[row.ExecID]; seen {
		return false
	}

	position.seenExecutions[row.ExecID] = struct{}{}
	return true
}

func (position *Position) onEntryExecution(row kraken.ExecutionData) {
	if position.unseenTrade(row) {
		position.applyEntryFill(row)
	}

	if !terminalOrderStatus(row.OrderStatus) {
		return
	}

	position.entryTerminal = true

	if position.Holding.SellableQty.Sign() > 0 {
		return
	}

	if position.Holding.Stoploss.Status == types.TRIGGERED {
		position.closeInventory(row.Timestamp)
		return
	}

	position.Status = terminalPositionStatus(row.OrderStatus)
	position.Holding.Status = position.Status
}

func (position *Position) applyEntryFill(row kraken.ExecutionData) {
	if row.CumQty.Cmp(position.entryCumQty) <= 0 {
		return
	}

	firstFill := position.entryCumQty.Sign() == 0
	delta := row.CumQty.Sub(position.entryCumQty)
	position.entryCumQty = row.CumQty

	if row.AvgPrice != nil {
		position.Holding.EntryPrice = row.AvgPrice
		position.Holding.ExitPrice = row.AvgPrice
		position.Holding.Mark = row.AvgPrice
	}

	if position.Holding.EntryAt == nil {
		entryAt := row.Timestamp
		position.Holding.EntryAt = &entryAt
	}

	if row.FeeUsdEquiv != nil && firstFill {
		position.Holding.EntryFee = row.FeeUsdEquiv
	}

	if row.FeeUsdEquiv != nil && !firstFill {
		position.Holding.EntryFee = position.Holding.EntryFee.Add(row.FeeUsdEquiv)
	}

	position.Holding.Qty = row.CumQty
	wasEmpty := position.Holding.SellableQty.Sign() == 0

	if wasEmpty {
		position.Holding.SellableQty = delta
	}

	if !wasEmpty {
		position.Holding.SellableQty = position.Holding.SellableQty.Add(delta)
	}

	position.Holding.Stoploss.RebindFill(types.Fill{
		EntryPrice: position.Holding.EntryPrice,
		EntryFee:   position.Holding.EntryFee,
		Qty:        position.Holding.Qty,
	})

	position.publishSnapshot()
	position.auditStops()

	if position.Holding.Stoploss.Status == types.TRIGGERED || position.exiting {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
		return
	}

	position.Status = types.OPEN
	position.Holding.Status = types.OPEN
}

func (position *Position) onExitExecution(row kraken.ExecutionData) {
	if position.unseenTrade(row) {
		position.applyExitFill(row)
	}

	if !terminalOrderStatus(row.OrderStatus) {
		return
	}

	position.exiting = false
	position.stopOrderID = ""
	position.exitOrderID = ""
	position.exitCumQty = decimal.NewFromInt64(0)

	if position.entryTerminal && position.Holding.SellableQty.Sign() == 0 {
		position.closeInventory(row.Timestamp)
		return
	}

	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
}

func (position *Position) applyExitFill(row kraken.ExecutionData) {
	if row.CumQty.Cmp(position.exitCumQty) <= 0 {
		return
	}

	delta := row.CumQty.Sub(position.exitCumQty)
	remaining := position.Holding.SellableQty

	if delta.Cmp(remaining) > 0 {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"position: exit fill exceeds remaining inventory for "+position.pair.Symbol,
			nil,
		))
		return
	}

	position.exitCumQty = row.CumQty
	position.Holding.SellableQty = remaining.Sub(delta)

	if row.AvgPrice != nil {
		position.Holding.ExitPrice = row.AvgPrice
	}

	if row.FeeUsdEquiv != nil && position.Holding.ExitFee == nil {
		position.Holding.ExitFee = row.FeeUsdEquiv
		return
	}

	if row.FeeUsdEquiv != nil {
		position.Holding.ExitFee = position.Holding.ExitFee.Add(row.FeeUsdEquiv)
	}
}

func (position *Position) closeInventory(at time.Time) {
	position.Holding.SellableQty = decimal.NewFromInt64(0)
	position.Holding.ExitAt = &at
	position.Status = types.CLOSED
	position.Holding.Status = types.CLOSED
}

func terminalOrderStatus(status string) bool {
	switch status {
	case "filled", "canceled", "cancelled", "rejected", "expired":
		return true
	default:
		return false
	}
}

func terminalPositionStatus(status string) types.Status {
	switch status {
	case "rejected":
		return types.REJECTED
	case "expired":
		return types.EXPIRED
	default:
		return types.CANCELED
	}
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	position.mu.Lock()
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR
		position.mu.Unlock()

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.EntryOrderResult = &result

	if position.Status != types.OPEN && position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}

	position.mu.Unlock()
	position.Publish()
	return position, nil
}

/*
submitStopExit submits a market sell for the sellable ledger quantity.

This method is deliberately private and accepts no caller-supplied authority.
The only call site is exitOnStop, after onTicker observed the regulator in the
triggered state. Keeping the submission behind that state transition makes a
strategy decision incapable of reaching the sell transport directly.
*/
func (position *Position) submitStopExit() error {
	if position.Holding == nil || position.Holding.Stoploss == nil ||
		position.Holding.Stoploss.Status != types.TRIGGERED {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"position: sell submission requires a triggered stoploss",
			nil,
		))
	}

	if position.stopOrderID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position: triggered stoploss is missing its exit order identifier",
			nil,
		))
	}

	if position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"position: no sellable quantity available",
			nil,
		))
	}

	position.ExitOrder.Volume = position.Holding.SellableQty.String()
	position.ExitOrder.ClOrdId = position.stopOrderID

	result, err := position.api.AddOrder(position.ExitOrder)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	if len(result.ID) == 0 || result.ID[0] == "" {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"position: stop exit acknowledgement is missing its venue order ID",
			nil,
		))
	}

	position.ExitOrderResult = &result
	position.exitOrderID = result.ID[0]
	position.exitCumQty = decimal.NewFromInt64(0)

	// The venue has the sell. From here the lot belongs to the exit leg, and a
	// late entry fill must not hand it back to the entry leg.
	position.exiting = true

	if position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}

	return nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.Status == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.Status = types.CLOSED
	return errnie.Error(err)
}
